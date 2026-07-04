package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Brand colors lifted straight from the same logo.jpg the Android app's
// Theme.kt already uses (dark navy background, teal compass-needle
// foreground) — not a separately invented desktop palette, so the two
// clients read as the same product. See android/app/src/main/java/dev/spur/app/Theme.kt.
var (
	brandTeal         = color.NRGBA{R: 0x24, G: 0xBD, B: 0xA8, A: 0xFF}
	brandTealDark     = color.NRGBA{R: 0x17, G: 0x8C, B: 0x7C, A: 0xFF}
	brandNavyBg       = color.NRGBA{R: 0x18, G: 0x19, B: 0x1E, A: 0xFF}
	brandNavySurface  = color.NRGBA{R: 0x21, G: 0x22, B: 0x28, A: 0xFF}
	brandSurfaceVar   = color.NRGBA{R: 0x2A, G: 0x2C, B: 0x33, A: 0xFF}
	brandOnSurfaceVar = color.NRGBA{R: 0xB8, G: 0xBA, B: 0xC0, A: 0xFF}
	brandOnBg         = color.NRGBA{R: 0xE3, G: 0xE4, B: 0xE6, A: 0xFF}
	brandError        = color.NRGBA{R: 0xE0, G: 0x55, B: 0x5A, A: 0xFF}
	brandWarning      = color.NRGBA{R: 0xD8, G: 0xA7, B: 0x3C, A: 0xFF}
	brandLightBg      = color.NRGBA{R: 0xF7, G: 0xF8, B: 0xF9, A: 0xFF}
)

// spurTheme mirrors SpurDarkColors from the Android app's Theme.kt: dark
// is the primary/default scheme — it's literally what the logo was
// designed against.
//
// Unlike Android (a reliable isSystemInDarkTheme() API), this always
// renders dark regardless of the fyne.ThemeVariant passed in, rather
// than switching to a light palette for a system light-mode preference.
// That's a deliberate response to a real, reproducible problem, not
// laziness: Fyne's own light/dark detection on Linux goes through the
// org.freedesktop.appearance portal (see fyne.io/fyne/v2/app's
// watchTheme/applyVariant, internal to the toolkit), which — confirmed
// live in this environment — silently defaults to light when the portal
// can't determine a real preference, via a background goroutine that
// calls Settings.applyVariant directly and overrides even an explicit
// FYNE_THEME=dark environment variable (applyVariant doesn't consult it
// at all). Chasing that race isn't worth it when the product's actual
// visual identity is a single fixed dark palette anyway — the Android
// app defaults to dark for the same reason (the logo was designed
// against it); light there is just a plain derivative for a preference
// this desktop client doesn't attempt to honor at all.
//
// widget.Label's Importance field (DangerImportance/WarningImportance/
// SuccessImportance) maps onto ColorNameError/ColorNameWarning/
// ColorNameSuccess below — see statuslabel.go's setStatus, which mirrors
// Android's StatusText the same way.
type spurTheme struct{}

var _ fyne.Theme = spurTheme{}

func (spurTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	const dark = true
	switch name {
	case theme.ColorNamePrimary, theme.ColorNameHyperlink:
		if dark {
			return brandTeal
		}
		return brandTealDark
	case theme.ColorNameBackground:
		if dark {
			return brandNavyBg
		}
		return brandLightBg
	case theme.ColorNameForeground:
		if dark {
			return brandOnBg
		}
		return color.Black
	case theme.ColorNameButton, theme.ColorNameInputBackground, theme.ColorNameMenuBackground, theme.ColorNameOverlayBackground:
		if dark {
			return brandNavySurface
		}
		return color.White
	case theme.ColorNameInputBorder, theme.ColorNameSeparator:
		if dark {
			return brandSurfaceVar
		}
		return color.NRGBA{R: 0xDD, G: 0xDD, B: 0xE0, A: 0xFF}
	case theme.ColorNameDisabled, theme.ColorNamePlaceHolder:
		return brandOnSurfaceVar
	case theme.ColorNameHover, theme.ColorNameFocus, theme.ColorNameSelection:
		if dark {
			return brandTealDark
		}
		return brandTeal
	case theme.ColorNameError:
		return brandError
	case theme.ColorNameWarning:
		return brandWarning
	case theme.ColorNameSuccess:
		return brandTeal
	}
	return theme.DefaultTheme().Color(name, variant)
}

func (spurTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (spurTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (spurTheme) Size(name fyne.ThemeSizeName) float32 {
	return theme.DefaultTheme().Size(name)
}
