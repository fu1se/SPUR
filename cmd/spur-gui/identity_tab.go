package main

import (
	"context"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
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
	selfIDLabel := widget.NewLabel(g.client.SelfID())
	selfIDLabel.Wrapping = fyne.TextWrapBreak

	copyStatus := widget.NewLabel("")
	copyBtn := widget.NewButton(g.cat.IdentityCopy, func() {
		g.app.Clipboard().SetContent(g.client.SelfID())
		copyStatus.SetText(g.cat.IdentityCopied)
	})

	testStatus := widget.NewLabel("")
	testBtn := widget.NewButton(g.cat.IdentityTestButton, nil)
	testBtn.OnTapped = func() {
		testBtn.Disable()
		testStatus.SetText(g.cat.IdentityTestRunning)
		serverAddr := g.serverAddr()
		go func() {
			result, err := g.client.Register(context.Background(), serverAddr, g.onVersionMismatch)
			fyne.Do(func() {
				testBtn.Enable()
				if err != nil {
					testStatus.SetText(cli.Explain(err))
					return
				}
				testStatus.SetText(fmt.Sprintf("%s %s (%s)", g.cat.IdentityTestOK, result.PeerID, result.ObservedAddress))
			})
		}()
	}

	saveBtn := widget.NewButton(g.cat.SettingsSave, g.saveSettings)

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
	return container.NewVScroll(form)
}
