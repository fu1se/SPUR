package dev.spur.app

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
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
    var status by remember { mutableStateOf("не зарегистрирован") }
    val scope = rememberCoroutineScope()

    Column(
        modifier = Modifier.fillMaxWidth().padding(24.dp),
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
    }
}
