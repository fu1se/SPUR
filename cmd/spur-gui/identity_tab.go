package main

import (
	"context"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/fu1se/spur/internal/adapter/cli"
)

// buildIdentityTab shows this client's own persistent peer ID (see
// guiapp.Client.SelfID — loaded once at startup, never changes), the
// shared server/STUN address settings every other tab reads from, and a
// "test connectivity" button mirroring "spur register": a one-shot,
// ephemeral-key registration purely to check the server address is
// reachable, before committing to a real rendezvous elsewhere in the UI.
func (g *guiApp) buildIdentityTab() fyne.CanvasObject {
	// Monospace, like Android's CopyableValue: makes look-alike
	// characters (0/O, 1/l) in the peer-id distinguishable.
	selfIDLabel := widget.NewLabelWithStyle(g.client.SelfID(), fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	selfIDLabel.Wrapping = fyne.TextWrapBreak

	copyStatus := newStatusLabel("")
	copyBtn := widget.NewButtonWithIcon(g.cat.IdentityCopy, theme.ContentCopyIcon(), func() {
		g.app.Clipboard().SetContent(g.client.SelfID())
		setStatus(copyStatus, g.cat.IdentityCopied)
	})

	testStatus := newStatusLabel("")
	testBtn := widget.NewButtonWithIcon(g.cat.IdentityTestButton, theme.ViewRefreshIcon(), nil)
	testBtn.OnTapped = func() {
		testBtn.Disable()
		setStatus(testStatus, g.cat.IdentityTestRunning)
		serverAddr := g.serverAddr()
		go func() {
			result, err := g.client.Register(context.Background(), serverAddr, g.onVersionMismatch)
			fyne.Do(func() {
				testBtn.Enable()
				if err != nil {
					setStatus(testStatus, cli.Explain(err))
					return
				}
				setStatus(testStatus, fmt.Sprintf("%s %s (%s)", g.cat.IdentityTestOK, result.PeerID, result.ObservedAddress))
			})
		}()
	}

	saveBtn := widget.NewButtonWithIcon(g.cat.SettingsSave, theme.DocumentSaveIcon(), g.saveSettings)

	form := container.NewVBox(
		widget.NewLabel(g.cat.IdentitySelfID),
		container.NewBorder(nil, nil, nil, copyBtn, selfIDLabel),
		copyStatus,
		widget.NewSeparator(),
		widget.NewLabel(g.cat.SettingsServer),
		g.serverEntry,
		widget.NewLabel(g.cat.SettingsStunServer),
		g.stunEntry,
		saveBtn,
		widget.NewSeparator(),
		testBtn,
		testStatus,
	)
	return container.NewVScroll(container.NewPadded(widget.NewCard("", "", form)))
}
