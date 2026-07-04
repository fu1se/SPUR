package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/fu1se/spur/internal/adapter/cli"
	"github.com/fu1se/spur/internal/adapter/guiapp"
)

// progressUIThrottle bounds how often a background transfer's progress
// callback (which can fire once per 32KB chunk, see
// usecase.copyWithProgress) is allowed to schedule a UI update — mirrors
// cli's own progress-bar throttling (internal/adapter/cli/progress.go)
// for the same reason: redrawing on every single chunk would flood
// Fyne's main event loop on a fast local transfer without adding
// anything a human can actually perceive.
const progressUIThrottle = 100 * time.Millisecond

// buildTransferTab is "spur send"/"spur receive": pick a mode, a
// counterpart (same to/room contract as the port-forward tab), a local
// path (file/folder to send, or a destination folder to receive into),
// and Start.
func (g *guiApp) buildTransferTab() fyne.CanvasObject {
	cat := g.cat
	mode := widget.NewSelect([]string{cat.FTModeSend, cat.FTModeReceive}, nil)
	mode.SetSelectedIndex(0)

	toEntry := widget.NewEntry()
	toEntry.SetPlaceHolder(cat.PFPeerOrCode)
	roomEntry := widget.NewEntry()
	roomEntry.SetPlaceHolder(cat.PFRoomName)

	pathEntry := widget.NewEntry()
	chooseFileBtn := widget.NewButtonWithIcon(cat.FTChoosePath, theme.FileIcon(), func() {
		dialog.ShowFileOpen(func(r fyne.URIReadCloser, err error) {
			if err != nil || r == nil {
				return
			}
			defer r.Close()
			pathEntry.SetText(r.URI().Path())
		}, g.win)
	})
	chooseFolderBtn := widget.NewButtonWithIcon(cat.FTChooseFolder, theme.FolderOpenIcon(), func() {
		dialog.ShowFolderOpen(func(u fyne.ListableURI, err error) {
			if err != nil || u == nil {
				return
			}
			pathEntry.SetText(u.Path())
		}, g.win)
	})

	codeLabel := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	progressBar := widget.NewProgressBar()
	statusLabel := newStatusLabel(cat.FTStatusIdle)

	var active *guiapp.Transfer
	var cancelEstablish context.CancelFunc

	var startBtn, stopBtn *widget.Button
	startBtn = widget.NewButtonWithIcon(cat.FTStart, theme.MediaPlayIcon(), nil)
	stopBtn = widget.NewButtonWithIcon(cat.FTStop, theme.MediaStopIcon(), nil)
	stopBtn.Importance = widget.DangerImportance
	stopBtn.Disable()

	resetToIdle := func() {
		startBtn.Enable()
		stopBtn.Disable()
		active = nil
		cancelEstablish = nil
	}

	startBtn.OnTapped = func() {
		path := pathEntry.Text
		if path == "" {
			setStatus(statusLabel, fmt.Sprintf(cat.FTStatusFailed, "empty path"))
			return
		}
		startBtn.Disable()
		codeLabel.SetText("")
		progressBar.SetValue(0)
		setStatus(statusLabel, cat.PFStatusEstablish)

		ctx, cancel := context.WithCancel(context.Background())
		cancelEstablish = cancel

		serverAddr, stunAddr := g.serverAddr(), g.stunAddr()
		to, room := toEntry.Text, roomEntry.Text
		isSend := mode.SelectedIndex() == 0

		onCode := func(code string) {
			fyne.Do(func() { codeLabel.SetText(fmt.Sprintf(cat.PFPairingCode, code)) })
		}
		onSelfID := func(string) {}

		var lastUpdate time.Time
		onProgress := func(relPath string, fileDone, fileTotal, overallDone, overallTotal int64) {
			now := time.Now()
			done := overallTotal > 0 && overallDone >= overallTotal
			if !done && now.Sub(lastUpdate) < progressUIThrottle {
				return
			}
			lastUpdate = now
			fyne.Do(func() {
				if overallTotal > 0 {
					progressBar.SetValue(float64(overallDone) / float64(overallTotal))
				}
				setStatus(statusLabel, fmt.Sprintf(cat.FTStatusRunning, relPath, humanBytes(overallDone), humanBytes(overallTotal)))
			})
		}

		onResumeOffer := func(filesWithData int, alreadyHave, total int64) bool {
			respCh := make(chan bool, 1)
			fyne.Do(func() {
				dialog.ShowConfirm(cat.FTResumeTitle,
					fmt.Sprintf(cat.FTResumeQuestion, filesWithData, humanBytes(alreadyHave), humanBytes(total)),
					func(ok bool) { respCh <- ok }, g.win)
			})
			return <-respCh
		}

		go func() {
			var tr *guiapp.Transfer
			var err error
			if isSend {
				tr, err = g.client.StartSend(ctx, serverAddr, stunAddr, to, room, path, onSelfID, onProgress, onCode, g.onVersionMismatch)
			} else {
				tr, err = g.client.StartReceive(ctx, serverAddr, stunAddr, to, room, path, onSelfID, onProgress, onCode, onResumeOffer, g.onVersionMismatch)
			}
			if err != nil {
				fyne.Do(func() {
					setStatus(statusLabel, fmt.Sprintf(cat.FTStatusFailed, cli.Explain(err)))
					resetToIdle()
				})
				return
			}

			fyne.Do(func() {
				active = tr
				stopBtn.Enable()
			})

			waitErr := tr.Wait()
			fyne.Do(func() {
				switch {
				case waitErr == nil:
					setStatus(statusLabel, cat.FTStatusDone)
					progressBar.SetValue(1)
				case errors.Is(waitErr, context.Canceled):
					setStatus(statusLabel, cat.PFStatusStopped)
				default:
					setStatus(statusLabel, fmt.Sprintf(cat.FTStatusFailed, cli.Explain(waitErr)))
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
		widget.NewLabel(cat.FTPath+" / "+cat.FTDestFolder),
		pathEntry,
		container.NewHBox(chooseFileBtn, chooseFolderBtn),
		container.NewHBox(startBtn, stopBtn),
		codeLabel,
		progressBar,
		statusLabel,
	)
	return container.NewVScroll(container.NewPadded(widget.NewCard("", "", form)))
}

// humanBytes renders n as a short, human-scaled byte count (KB/MB/GB) —
// this tab's own copy of the same formatting cli's progress bar uses
// (internal/adapter/cli/progress.go's humanBytes is unexported and, per
// that package's doc comment, cli must not be imported for anything
// beyond Explain/Version/its plain callback types — duplicating one small
// formatting func is cheaper than restructuring cli's exports for it).
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
