package dev.spur.app

import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.Intent
import android.net.VpnService
import android.os.Build
import androidx.core.app.NotificationCompat
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.launch
import spurmobile.Client
import spurmobile.MeshSession

// SpurVpnService is "spur join" for Android: the only piece of the app
// that needs android.permission.BIND_VPN_SERVICE, since it's the only
// feature that touches the device's default route. Port-forward and
// send/receive need no special permission at all — they're plain TCP
// traffic the app originates itself, not a system-wide tunnel.
//
// The interface itself (address, routes) is created and configured by
// Builder.establish() here, in Kotlin — a regular app process cannot
// create a TUN device the way the desktop CLI's wgmesh.NewDevice does
// (that needs CAP_NET_ADMIN). Only the resulting file descriptor crosses
// into Go, via spurmobile.Client.JoinMesh — see wgmesh.NewDeviceFromFD's
// doc comment.
class SpurVpnService : VpnService() {
    private var meshSession: MeshSession? = null
    private var job: Job? = null
    private val scope = CoroutineScope(Dispatchers.IO)

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val serverAddr = intent?.getStringExtra(EXTRA_SERVER)
        val stunAddr = intent?.getStringExtra(EXTRA_STUN)
        val networkName = intent?.getStringExtra(EXTRA_NETWORK)
        val inviteToken = intent?.getStringExtra(EXTRA_INVITE) ?: ""
        if (serverAddr == null || stunAddr == null || networkName == null) {
            stopSelf()
            return START_NOT_STICKY
        }

        startForeground(NOTIFICATION_ID, buildNotification())

        job = scope.launch {
            try {
                val client = Client(filesDir.absolutePath)
                // Builder.addAddress/addRoute must be called before
                // establish() returns a fd, but the mesh address only
                // becomes known by actually joining the network on the
                // server — hence the two-step Resolve-then-Join split
                // (see MeshNetworkInfo's doc comment). Joining twice is
                // safe: the second JoinMesh call is the same idempotent
                // RPC under the hood.
                val info = client.resolveMeshNetwork(serverAddr, networkName, inviteToken)
                val prefixBits = info.getCIDRBits().toInt()
                val pfd = Builder()
                    .setSession("spur")
                    .addAddress(info.getSelfMeshIP(), prefixBits)
                    .addRoute(networkAddress(info.getSelfMeshIP(), prefixBits), prefixBits)
                    .setMtu(1280)
                    .establish() ?: throw IllegalStateException("VpnService.Builder.establish() returned null")
                // detachFd(), not just .fd: ownership of the underlying
                // fd moves to the Go side (wgmesh.NewDeviceFromFD wraps
                // it in an os.File and closes it via MeshSession.Stop) —
                // leaving ParcelFileDescriptor believing it still owns
                // the fd would double-close it.
                val fd = pfd.detachFd()
                meshSession = client.joinMesh(serverAddr, stunAddr, networkName, inviteToken, fd.toLong())
            } catch (e: Exception) {
                android.util.Log.e("SpurVpnService", "mesh join failed", e)
                stopSelf()
            }
        }
        return START_STICKY
    }

    override fun onRevoke() {
        stopMesh()
        super.onRevoke()
    }

    override fun onDestroy() {
        stopMesh()
        super.onDestroy()
    }

    private fun stopMesh() {
        job?.cancel()
        meshSession?.stop()
        meshSession = null
    }

    private fun buildNotification(): android.app.Notification {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channel = NotificationChannel(CHANNEL_ID, "spur mesh", NotificationManager.IMPORTANCE_LOW)
            getSystemService(NotificationManager::class.java).createNotificationChannel(channel)
        }
        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle("spur")
            .setContentText("подключено к mesh-сети")
            .setSmallIcon(android.R.drawable.stat_sys_download_done)
            .setOngoing(true)
            .build()
    }

    companion object {
        const val EXTRA_SERVER = "server"
        const val EXTRA_STUN = "stun"
        const val EXTRA_NETWORK = "network"
        const val EXTRA_INVITE = "invite"
        private const val CHANNEL_ID = "spur_mesh"
        private const val NOTIFICATION_ID = 1

        // networkAddress computes the network address for addRoute from
        // an assigned host address and prefix length (e.g.
        // "100.64.0.3"/10 -> "100.64.0.0") — VpnService.Builder.addRoute
        // wants the route's network address, not a specific host's.
        fun networkAddress(hostAddr: String, prefixBits: Int): String {
            val octets = hostAddr.split(".").map { it.toInt() }
            val mask = if (prefixBits == 0) 0 else (-1 shl (32 - prefixBits))
            var addr = 0
            for (o in octets) addr = (addr shl 8) or o
            val netAddr = addr and mask
            return listOf(24, 16, 8, 0).joinToString(".") { shift -> ((netAddr ushr shift) and 0xFF).toString() }
        }
    }
}
