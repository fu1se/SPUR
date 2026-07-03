package dev.spur.app

import android.content.Context
import android.net.Uri
import androidx.documentfile.provider.DocumentFile
import java.io.FileNotFoundException
import java.io.IOException
import org.json.JSONArray
import org.json.JSONObject
import spurmobile.FileSink
import spurmobile.FileSource
import spurmobile.MobileReader
import spurmobile.MobileWriter

private const val MIME_OCTET_STREAM = "application/octet-stream"

// findDocument walks segments of relPath (forward-slash separated, as
// spur's wire protocol always uses — see file_transfer_wire.go) starting
// from root, returning null if any segment along the way doesn't exist.
private fun findDocument(root: DocumentFile, relPath: String): DocumentFile? {
    var current = root
    for (segment in relPath.split("/")) {
        current = current.findFile(segment) ?: return null
    }
    return current
}

// ensureParentDirs creates (if missing) every directory segment of
// relPath except the last (the file itself), returning the immediate
// parent DocumentFile.
private fun ensureParentDirs(root: DocumentFile, relPath: String): DocumentFile {
    val segments = relPath.split("/")
    var dir = root
    for (i in 0 until segments.size - 1) {
        val name = segments[i]
        dir = dir.findFile(name) ?: dir.createDirectory(name)
            ?: throw IOException("spur: cannot create directory $name")
    }
    return dir
}

// SafFileSource is spurmobile.FileSource backed by the Storage Access
// Framework: spur's Go core (android/spurmobile) can't touch content://
// URIs directly (Android gives apps no arbitrary filesystem path access
// the way desktop Go's os/filepath assumes — see filetransfer.go's doc
// comment in android/spurmobile), so every actual file operation happens
// here in Kotlin and is only relayed to Go as bytes/JSON.
class SafFileSource(private val context: Context, treeUri: Uri) : FileSource {
    private val root: DocumentFile = DocumentFile.fromTreeUri(context, treeUri)
        ?: throw IOException("spur: cannot open picked folder")

    override fun listJSON(): String {
        val out = JSONArray()
        fun walk(dir: DocumentFile, prefix: String) {
            for (child in dir.listFiles()) {
                val name = child.name ?: continue
                val relPath = if (prefix.isEmpty()) name else "$prefix/$name"
                if (child.isDirectory) {
                    walk(child, relPath)
                } else {
                    out.put(JSONObject().put("RelPath", relPath).put("Size", child.length()))
                }
            }
        }
        walk(root, "")
        return out.toString()
    }

    override fun open(relPath: String, skip: Long): MobileReader {
        val doc = findDocument(root, relPath) ?: throw FileNotFoundException(relPath)
        val stream = context.contentResolver.openInputStream(doc.uri)
            ?: throw IOException("spur: cannot open $relPath")
        // content:// InputStreams generally aren't seekable, so resuming
        // mid-file means reading and discarding the skipped prefix —
        // same cost as re-sending it, but at least not re-transmitting
        // it over the tunnel (skip is set only when the receiver already
        // has that many bytes — see usecase's resume plan).
        var remaining = skip
        val discard = ByteArray(64 * 1024)
        while (remaining > 0) {
            val n = stream.read(discard, 0, minOf(discard.size.toLong(), remaining).toInt())
            if (n < 0) break
            remaining -= n
        }
        return object : MobileReader {
            // Returns a freshly allocated array, not a count of bytes
            // written into a caller-supplied one — see
            // spurmobile.MobileReader's doc comment for why: gomobile
            // does not copy a Java-side write into a []byte parameter
            // back to the Go slice it came from, confirmed by a real,
            // silent all-zero-bytes transfer before this was changed.
            override fun read(n: Long): ByteArray {
                val buf = ByteArray(n.toInt())
                val read = stream.read(buf)
                return if (read <= 0) ByteArray(0) else buf.copyOf(read)
            }

            override fun close() = stream.close()
        }
    }
}

// SafFileSink is spurmobile.FileSink backed by the Storage Access
// Framework — see SafFileSource's doc comment for why this lives here
// rather than in Go.
class SafFileSink(private val context: Context, treeUri: Uri) : FileSink {
    private val root: DocumentFile = DocumentFile.fromTreeUri(context, treeUri)
        ?: throw IOException("spur: cannot open picked folder")

    override fun existingSize(relPath: String): Long {
        val doc = findDocument(root, relPath) ?: return 0
        return doc.length()
    }

    override fun create(relPath: String, size: Long, offset: Long): MobileWriter {
        val parent = ensureParentDirs(root, relPath)
        val name = relPath.substringAfterLast("/")
        val existing = parent.findFile(name)
        val doc = if (offset > 0) {
            // Resuming: the file must already exist with exactly offset
            // bytes — usecase.ReceiveFiles only ever passes an offset it
            // got from a prior ExistingSize call on this same sink, but
            // don't trust that blindly (mirrors localfs.Sink.Create's
            // same re-check): a mismatch here means something changed
            // the file between the two calls.
            val doc = existing ?: throw IOException("spur: resume target $relPath vanished")
            if (doc.length() != offset) {
                throw IOException("spur: resume offset mismatch for $relPath: have ${doc.length()}, want $offset")
            }
            doc
        } else {
            existing?.delete()
            parent.createFile(MIME_OCTET_STREAM, name)
                ?: throw IOException("spur: cannot create $relPath")
        }

        val mode = if (offset > 0) "wa" else "w"
        val stream = context.contentResolver.openOutputStream(doc.uri, mode)
            ?: throw IOException("spur: cannot open $relPath for writing")
        return object : MobileWriter {
            override fun write(buf: ByteArray): Long {
                stream.write(buf)
                return buf.size.toLong()
            }

            override fun close() = stream.close()
        }
    }
}
