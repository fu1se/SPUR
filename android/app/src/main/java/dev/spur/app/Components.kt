package dev.spur.app

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.ClipboardManager
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp

// SectionCard is every feature area's outer shell (Register, Port forward,
// Send/receive, Rooms, Mesh VPN) — a titled card instead of the flat
// un-delimited wall of fields/buttons the screen used to be, so five
// unrelated features stacked in one scrolling column read as five
// distinct panels rather than one long form.
@Composable
fun SectionCard(
    title: String,
    icon: ImageVector,
    content: @Composable ColumnScope.() -> Unit,
) {
    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
    ) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                Icon(icon, contentDescription = null, tint = MaterialTheme.colorScheme.primary)
                Text(title, style = MaterialTheme.typography.titleMedium)
            }
            content()
        }
    }
}

// StatusText color-codes every section's one-line status by sniffing its
// text for the same handful of substrings every RunE-style callback in
// this app already produces ("ошибка"/"error", "остановлено", "..."
// mid-operation) — one shared rule instead of every section re-deciding
// its own success/error/idle colors, and the same rule applying
// automatically to friendly-error strings from cli.Explain (Russian) or
// English text alike.
@Composable
fun StatusText(text: String) {
    if (text.isBlank()) return
    val color = when {
        text.contains("ошибка", ignoreCase = true) || text.contains("error", ignoreCase = true) -> StatusColors.Error
        text.contains("отказано", ignoreCase = true) -> StatusColors.Error
        text.contains("остановлено", ignoreCase = true) ||
            text.contains("не зарегистрирован", ignoreCase = true) ||
            text == "stopped" -> StatusColors.Idle
        text.endsWith("...") -> StatusColors.InProgress
        else -> StatusColors.Success
    }
    Text(text, color = color, style = MaterialTheme.typography.bodyMedium)
}

// CopyableValue is for anything the user has to relay to a second person
// verbatim (peer-id, a pairing code, a room invite token) — reading a
// 32-character hex string aloud or retyping it by hand is exactly the
// kind of friction that caused real mistakes during this project's own
// live testing sessions (mistyped/truncated peer-ids). A monospace font
// makes look-alike characters (0/O, 1/l) distinguishable, and the copy
// button sidesteps retyping entirely.
@Composable
fun CopyableValue(label: String, value: String) {
    val clipboard: ClipboardManager = LocalClipboardManager.current
    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Column(modifier = Modifier.weight(1f)) {
            Text(label, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            Text(value, style = MaterialTheme.typography.bodyMedium, fontFamily = FontFamily.Monospace)
        }
        IconButton(onClick = { clipboard.setText(AnnotatedString(value)) }) {
            Icon(Icons.Default.ContentCopy, contentDescription = "copy $label")
        }
    }
}

// ModeToggle is the shared to/peer-id-or-code vs. room switch used by
// both Port forward and Send/receive — previously a single Button whose
// label just changed text on tap, giving no visual sense that this is a
// two-way choice rather than an action. A segmented control reads as
// exactly what it is: pick one of two mutually exclusive modes.
@Composable
fun ModeToggle(isRoom: Boolean, onChange: (Boolean) -> Unit, toLabel: String) {
    SingleChoiceSegmentedButtonRow(modifier = Modifier.fillMaxWidth()) {
        SegmentedButton(
            selected = !isRoom,
            onClick = { onChange(false) },
            shape = SegmentedButtonDefaults.itemShape(index = 0, count = 2),
        ) {
            Text(toLabel)
        }
        SegmentedButton(
            selected = isRoom,
            onClick = { onChange(true) },
            shape = SegmentedButtonDefaults.itemShape(index = 1, count = 2),
        ) {
            Text("room")
        }
    }
}
