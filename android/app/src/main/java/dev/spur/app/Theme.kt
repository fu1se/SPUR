package dev.spur.app

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color

// Colors lifted straight from logo.jpg (the product's actual logo, not a
// palette invented separately from it): dark navy background, teal
// compass-needle foreground. Dark is the default/primary theme — it's
// literally what the logo was designed against — light is a plain
// derivative for users who have the system light-mode preference, not a
// separately designed palette.
private val BrandTeal = Color(0xFF24BDA8)
private val BrandTealDark = Color(0xFF178C7C)
private val BrandNavyBackground = Color(0xFF18191E)
private val BrandNavySurface = Color(0xFF212228)
private val BrandError = Color(0xFFE0555A)

private val SpurDarkColors = darkColorScheme(
    primary = BrandTeal,
    onPrimary = Color(0xFF00201C),
    secondary = BrandTealDark,
    background = BrandNavyBackground,
    onBackground = Color(0xFFE3E4E6),
    surface = BrandNavySurface,
    onSurface = Color(0xFFE3E4E6),
    surfaceVariant = Color(0xFF2A2C33),
    onSurfaceVariant = Color(0xFFB8BAC0),
    error = BrandError,
)

private val SpurLightColors = lightColorScheme(
    primary = BrandTealDark,
    onPrimary = Color.White,
    secondary = BrandTeal,
    background = Color(0xFFF7F8F9),
    surface = Color.White,
    error = BrandError,
)

@Composable
fun SpurTheme(content: @Composable () -> Unit) {
    val colors = if (isSystemInDarkTheme()) SpurDarkColors else SpurLightColors
    MaterialTheme(colorScheme = colors, content = content)
}

// Semantic status colors, used across every section's status line (see
// StatusText in MainActivity.kt) so "ошибка: ..." always reads as an error
// regardless of which feature printed it, instead of every section
// re-deciding its own color scheme.
object StatusColors {
    val Success = BrandTeal
    val Error = BrandError
    val Idle @Composable get() = MaterialTheme.colorScheme.onSurfaceVariant
    val InProgress = Color(0xFFD8A73C)
}
