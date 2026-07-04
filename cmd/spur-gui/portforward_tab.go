package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/fu1se/spur/internal/adapter/cli"
	"github.com/fu1se/spur/internal/adapter/guiapp"
)

// buildPortForwardTab is "spur connect"/"spur expose" as a form: pick a
// mode, a counterpart (peer ID/pairing code, or a room — same three ways
// rendezvous.CounterpartResolverFor supports, see its doc comment), a
// port, and Start. Only one session can run per tab instance at a time,
// tracked by the closure-captured active/cancel below — matches how the
// CLI's connect/expose are each a single foreground process.
func (g *guiApp) buildPortForwardTab() fyne.CanvasObject {
	cat := g.cat
	mode := widget.NewSelect([]string{cat.PFModeConnect, cat.PFModeExpose}, nil)
	mode.SetSelectedIndex(0)

	toEntry := widget.NewEntry()
	toEntry.SetPlaceHolder(cat.PFPeerOrCode)
	roomEntry := widget.NewEntry()
	roomEntry.SetPlaceHolder(cat.PFRoomName)
	portEntry := widget.NewEntry()
	portEntry.SetText("2222")

	codeLabel := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	statusLabel := newStatusLabel(cat.PFStatusIdle)

	var active *guiapp.PortForward
	var cancelEstablish context.CancelFunc

	var startBtn, stopBtn *widget.Button
	startBtn = widget.NewButtonWithIcon(cat.PFStart, theme.MediaPlayIcon(), nil)
	stopBtn = widget.NewButtonWithIcon(cat.PFStop, theme.MediaStopIcon(), nil)
	stopBtn.Importance = widget.DangerImportance
	stopBtn.Disable()

	resetToIdle := func() {
		startBtn.Enable()
		stopBtn.Disable()
		active = nil
		cancelEstablish = nil
	}

	startBtn.OnTapped = func() {
		port, err := strconv.Atoi(portEntry.Text)
		if err != nil {
			setStatus(statusLabel, fmt.Sprintf(cat.PFStatusFailed, err))
			return
		}
		startBtn.Disable()
		codeLabel.SetText("")
		setStatus(statusLabel, cat.PFStatusEstablish)

		ctx, cancel := context.WithCancel(context.Background())
		cancelEstablish = cancel

		serverAddr, stunAddr := g.serverAddr(), g.stunAddr()
		to, room := toEntry.Text, roomEntry.Text
		isConnect := mode.SelectedIndex() == 0

		onCode := func(code string) {
			fyne.Do(func() { codeLabel.SetText(fmt.Sprintf(cat.PFPairingCode, code)) })
		}
		onSelfID := func(string) {}

		go func() {
			var pf *guiapp.PortForward
			var err error
			if isConnect {
				pf, err = g.client.StartConnect(ctx, serverAddr, stunAddr, to, room, port, onSelfID, onCode, g.onVersionMismatch)
			} else {
				pf, err = g.client.StartExpose(ctx, serverAddr, stunAddr, to, room, port, onSelfID, onCode, g.onVersionMismatch)
			}
			if err != nil {
				fyne.Do(func() {
					setStatus(statusLabel, fmt.Sprintf(cat.PFStatusFailed, cli.Explain(err)))
					resetToIdle()
				})
				return
			}

			fyne.Do(func() {
				active = pf
				stopBtn.Enable()
				addr := pf.LocalAddr
				if addr == "" {
					addr = "-"
				}
				setStatus(statusLabel, fmt.Sprintf(cat.PFStatusRunning, addr))
			})

			waitErr := pf.Wait()
			fyne.Do(func() {
				if errors.Is(waitErr, context.Canceled) {
					setStatus(statusLabel, cat.PFStatusStopped)
				} else {
					setStatus(statusLabel, fmt.Sprintf(cat.PFStatusFailed, cli.Explain(waitErr)))
				}
				resetToIdle()
			})
		}()
	}

	stopBtn.OnTapped = func() {
		if active != nil {
			active.Stop()
		} else if cancelEstablish != nil {
			cancelEstablish()
		}
		stopBtn.Disable()
	}

	form := container.NewVBox(
		mode,
		widget.NewLabel(cat.PFTargetTo),
		toEntry,
		widget.NewLabel(cat.PFTargetRoom),
		roomEntry,
		widget.NewLabel(cat.PFLocalPort+" / "+cat.PFTargetPort),
		portEntry,
		container.NewHBox(startBtn, stopBtn),
		codeLabel,
		statusLabel,
	)
	return container.NewVScroll(container.NewPadded(widget.NewCard("", "", form)))
}
