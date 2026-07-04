package dev.spur.app

import android.content.Intent
import android.net.Uri
import android.net.VpnService
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import spurmobile.Client
import spurmobile.CodeCallback
import spurmobile.PortForward
import spurmobile.ProgressCallback
import spurmobile.Spurmobile
import spurmobile.Transfer

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        // filesDir is app-private storage, always writable without any
        // permission — mirrors the desktop CLI's ~/.config/spur, see
        // spurmobile.Client's doc comment for why os.UserConfigDir()
        // can't be used here instead.
        val client = Client(filesDir.absolutePath)

        setContent {
            MaterialTheme {
                Surface(modifier = Modifier.fillMaxWidth()) {
                    SkeletonScreen(coreVersion = Spurmobile.version(), client = client)
                }
            }
        }
    }
}

@Composable
fun SkeletonScreen(coreVersion: String, client: Client) {
    var serverAddr by remember { mutableStateOf("") }
    var stunAddr by remember { mutableStateOf("") }
    var status by remember { mutableStateOf("не зарегистрирован") }
    val scope = rememberCoroutineScope()

    var toOrRoom by remember { mutableStateOf("") }
    var isRoom by remember { mutableStateOf(false) }
    var port by remember { mutableStateOf("") }
    var pfStatus by remember { mutableStateOf("остановлено") }
    var pairingCode by remember { mutableStateOf("") }
    var activeForward by remember { mutableStateOf<PortForward?>(null) }

    Column(
        modifier = Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(24.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Text("spur")
        Text("Go core version: $coreVersion")
        Text("peer id: ${client.selfID()}")
        OutlinedTextField(
            value = serverAddr,
            onValueChange = { serverAddr = it },
            label = { Text("server address") },
        )
        Button(onClick = {
            scope.launch {
                status = "подключение..."
                status = try {
                    withContext(Dispatchers.IO) { client.register(serverAddr) }
                    "зарегистрирован"
                } catch (e: Exception) {
                    "ошибка: ${e.message}"
                }
            }
        }) {
            Text("Register")
        }
        Text(status)

        Text("Port forward")
        OutlinedTextField(
            value = stunAddr,
            onValueChange = { stunAddr = it },
            label = { Text("stun address") },
        )
        OutlinedTextField(
            value = toOrRoom,
            onValueChange = { toOrRoom = it },
            label = { Text(if (isRoom) "room" else "to (peer-id/код, пусто — режим хоста)") },
        )
        OutlinedTextField(
            value = port,
            onValueChange = { port = it },
            label = { Text("порт") },
        )
        Button(onClick = { isRoom = !isRoom }) {
            Text(if (isRoom) "режим: room (нажмите для to/код)" else "режим: to/код (нажмите для room)")
        }

        val to = if (isRoom) "" else toOrRoom
        val room = if (isRoom) toOrRoom else ""
        val onCode = CodeCallback { code -> pairingCode = code }

        Button(onClick = {
            scope.launch {
                pfStatus = "устанавливаем канал..."
                pairingCode = ""
                try {
                    val portNum = port.toLong()
                    val fwd = withContext(Dispatchers.IO) {
                        client.startConnect(serverAddr, stunAddr, to, room, portNum, onCode)
                    }
                    activeForward = fwd
                    pfStatus = "connect: локальный порт $port проброшен"
                    scope.launch(Dispatchers.IO) {
                        val err = runCatching { fwd.await() }.exceptionOrNull()
                        pfStatus = if (err != null) "connect: завершилось с ошибкой: ${err.message}" else "connect: остановлено"
                    }
                } catch (e: Exception) {
                    pfStatus = "ошибка: ${e.message}"
                }
            }
        }) {
            Text("Start connect")
        }
        Button(onClick = {
            scope.launch {
                pfStatus = "устанавливаем канал..."
                pairingCode = ""
                try {
                    val portNum = port.toLong()
                    val fwd = withContext(Dispatchers.IO) {
                        client.startExpose(serverAddr, stunAddr, to, room, portNum, onCode)
                    }
                    activeForward = fwd
                    pfStatus = "expose: локальный сервис на порту $port открыт"
                    scope.launch(Dispatchers.IO) {
                        val err = runCatching { fwd.await() }.exceptionOrNull()
                        pfStatus = if (err != null) "expose: завершилось с ошибкой: ${err.message}" else "expose: остановлено"
                    }
                } catch (e: Exception) {
                    pfStatus = "ошибка: ${e.message}"
                }
            }
        }) {
            Text("Start expose")
        }
        Button(onClick = { activeForward?.stop() }) {
            Text("Stop")
        }
        if (pairingCode.isNotEmpty()) {
            Text("код для подключения: $pairingCode")
        }
        Text(pfStatus)

        Text("Send / receive")
        var ftToOrRoom by remember { mutableStateOf("") }
        var ftIsRoom by remember { mutableStateOf(false) }
        var sourceUri by remember { mutableStateOf<Uri?>(null) }
        var destUri by remember { mutableStateOf<Uri?>(null) }
        var ftStatus by remember { mutableStateOf("остановлено") }
        var ftCode by remember { mutableStateOf("") }
        var activeTransfer by remember { mutableStateOf<Transfer?>(null) }
        val context = LocalContext.current

        val pickSource = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocumentTree()) { uri ->
            if (uri != null) {
                context.contentResolver.takePersistableUriPermission(
                    uri,
                    android.content.Intent.FLAG_GRANT_READ_URI_PERMISSION,
                )
                sourceUri = uri
            }
        }
        val pickDest = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocumentTree()) { uri ->
            if (uri != null) {
                context.contentResolver.takePersistableUriPermission(
                    uri,
                    android.content.Intent.FLAG_GRANT_READ_URI_PERMISSION or android.content.Intent.FLAG_GRANT_WRITE_URI_PERMISSION,
                )
                destUri = uri
            }
        }

        OutlinedTextField(
            value = ftToOrRoom,
            onValueChange = { ftToOrRoom = it },
            label = { Text(if (ftIsRoom) "room" else "to (peer-id/код, пусто — режим хоста)") },
        )
        Button(onClick = { ftIsRoom = !ftIsRoom }) {
            Text(if (ftIsRoom) "режим: room (нажмите для to/код)" else "режим: to/код (нажмите для room)")
        }
        val ftTo = if (ftIsRoom) "" else ftToOrRoom
        val ftRoom = if (ftIsRoom) ftToOrRoom else ""
        val ftOnCode = CodeCallback { code -> ftCode = code }
        val onProgress = ProgressCallback { relPath, fileDone, fileTotal, overallDone, overallTotal ->
            ftStatus = "$relPath: $fileDone/$fileTotal (всего $overallDone/$overallTotal)"
        }

        Button(onClick = { pickSource.launch(null) }) {
            Text(if (sourceUri != null) "источник выбран" else "выбрать папку-источник")
        }
        Button(onClick = {
            scope.launch {
                val uri = sourceUri
                if (uri == null) {
                    ftStatus = "сначала выберите папку-источник"
                    return@launch
                }
                ftStatus = "устанавливаем канал..."
                ftCode = ""
                try {
                    val source = SafFileSource(context, uri)
                    val tr = withContext(Dispatchers.IO) {
                        client.startSend(serverAddr, stunAddr, ftTo, ftRoom, source, ftOnCode, onProgress)
                    }
                    activeTransfer = tr
                    scope.launch(Dispatchers.IO) {
                        val err = runCatching { tr.await() }.exceptionOrNull()
                        ftStatus = if (err != null) "send: ошибка: ${err.message}" else "send: завершено"
                    }
                } catch (e: Exception) {
                    ftStatus = "ошибка: ${e.message}"
                }
            }
        }) {
            Text("Start send")
        }

        Button(onClick = { pickDest.launch(null) }) {
            Text(if (destUri != null) "папка назначения выбрана" else "выбрать папку назначения")
        }
        Button(onClick = {
            scope.launch {
                val uri = destUri
                if (uri == null) {
                    ftStatus = "сначала выберите папку назначения"
                    return@launch
                }
                ftStatus = "устанавливаем канал..."
                ftCode = ""
                try {
                    val sink = SafFileSink(context, uri)
                    val tr = withContext(Dispatchers.IO) {
                        client.startReceive(serverAddr, stunAddr, ftTo, ftRoom, sink, ftOnCode, onProgress, null)
                    }
                    activeTransfer = tr
                    scope.launch(Dispatchers.IO) {
                        val err = runCatching { tr.await() }.exceptionOrNull()
                        ftStatus = if (err != null) "receive: ошибка: ${err.message}" else "receive: завершено"
                    }
                } catch (e: Exception) {
                    ftStatus = "ошибка: ${e.message}"
                }
            }
        }) {
            Text("Start receive")
        }
        Button(onClick = { activeTransfer?.stop() }) {
            Text("Stop transfer")
        }
        if (ftCode.isNotEmpty()) {
            Text("код для подключения: $ftCode")
        }
        Text(ftStatus)

        Text("Rooms")
        var roomName by remember { mutableStateOf("") }
        var roomStatus by remember { mutableStateOf("") }
        OutlinedTextField(
            value = roomName,
            onValueChange = { roomName = it },
            label = { Text("имя комнаты") },
        )
        Button(onClick = {
            scope.launch {
                roomStatus = "создаём..."
                roomStatus = try {
                    val token = withContext(Dispatchers.IO) { client.createRoom(serverAddr, roomName) }
                    "инвайт-токен: $token"
                } catch (e: Exception) {
                    "ошибка: ${e.message}"
                }
            }
        }) {
            Text("Create room")
        }
        var inviteToken by remember { mutableStateOf("") }
        OutlinedTextField(
            value = inviteToken,
            onValueChange = { inviteToken = it },
            label = { Text("инвайт-токен") },
        )
        Button(onClick = {
            scope.launch {
                roomStatus = "присоединяемся..."
                roomStatus = try {
                    withContext(Dispatchers.IO) { client.joinRoom(serverAddr, roomName, inviteToken) }
                    "вы в комнате \"$roomName\" — используйте её в поле room выше"
                } catch (e: Exception) {
                    "ошибка: ${e.message}"
                }
            }
        }) {
            Text("Join room")
        }
        Text(roomStatus)

        Text("Mesh VPN")
        var meshNetwork by remember { mutableStateOf("") }
        var meshInvite by remember { mutableStateOf("") }
        var meshStatus by remember { mutableStateOf("остановлено") }
        val vpnPermissionLauncher = rememberLauncherForActivityResult(
            ActivityResultContracts.StartActivityForResult(),
        ) { result ->
            if (result.resultCode == android.app.Activity.RESULT_OK) {
                startMeshService(context, serverAddr, stunAddr, meshNetwork, meshInvite)
                meshStatus = "запускается..."
            } else {
                meshStatus = "отказано в VPN-разрешении"
            }
        }
        OutlinedTextField(
            value = meshNetwork,
            onValueChange = { meshNetwork = it },
            label = { Text("имя mesh-сети") },
        )
        OutlinedTextField(
            value = meshInvite,
            onValueChange = { meshInvite = it },
            label = { Text("инвайт-токен (не нужен при повторном join)") },
        )
        Button(onClick = {
            // VpnService.prepare returns a consent-screen Intent the
            // first time (or after being revoked), null once already
            // granted — same one-time-then-remembered flow as any other
            // Android runtime permission.
            val prepareIntent = VpnService.prepare(context)
            if (prepareIntent != null) {
                vpnPermissionLauncher.launch(prepareIntent)
            } else {
                startMeshService(context, serverAddr, stunAddr, meshNetwork, meshInvite)
            }
            meshStatus = "запускается..."
        }) {
            Text("Join mesh")
        }
        Button(onClick = {
            context.stopService(Intent(context, SpurVpnService::class.java))
            meshStatus = "остановлено"
        }) {
            Text("Stop mesh")
        }
        Text(meshStatus)
    }
}

private fun startMeshService(context: android.content.Context, serverAddr: String, stunAddr: String, networkName: String, inviteToken: String) {
    val intent = Intent(context, SpurVpnService::class.java)
        .putExtra(SpurVpnService.EXTRA_SERVER, serverAddr)
        .putExtra(SpurVpnService.EXTRA_STUN, stunAddr)
        .putExtra(SpurVpnService.EXTRA_NETWORK, networkName)
        .putExtra(SpurVpnService.EXTRA_INVITE, inviteToken)
    context.startForegroundService(intent)
}
