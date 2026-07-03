package dev.spur.app

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
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
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import spurmobile.Client
import spurmobile.CodeCallback
import spurmobile.PortForward
import spurmobile.Spurmobile

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
    }
}
