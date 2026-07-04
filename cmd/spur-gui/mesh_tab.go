package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/fu1se/spur/internal/adapter/cli"
	"github.com/fu1se/spur/internal/adapter/guiapp"
)

// buildMeshTab is "spur join": mesh VPN mode. Requires root/CAP_NET_ADMIN
// on Linux to create the TUN interface — same requirement as the CLI
// (see CLAUDE.md's "Фаза 6: mesh VPN" section), so a permission failure
// here is expected unless the GUI itself was started elevated.
func (g *guiApp) buildMeshTab() fyne.CanvasObject {
	cat := g.cat

	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder(cat.MeshNetworkName)
	tokenEntry := widget.NewEntry()
	tokenEntry.SetPlaceHolder(cat.MeshInviteToken)
	verboseCheck := widget.NewCheck(cat.MeshVerbose, nil)

	statusLabel := widget.NewLabel(cat.MeshStatusIdle)
	statusLabel.Wrapping = fyne.TextWrapWord
	membersLabel := widget.NewLabel("")
	membersLabel.Wrapping = fyne.TextWrapWord

	var active *guiapp.MeshSession

	var joinBtn, leaveBtn *widget.Button
	joinBtn = widget.NewButton(cat.MeshJoin, nil)
	leaveBtn = widget.NewButton(cat.MeshLeave, nil)
	leaveBtn.Disable()

	joinBtn.OnTapped = func() {
		name := nameEntry.Text
		if name == "" {
			return
		}
		joinBtn.Disable()
		statusLabel.SetText(cat.PFStatusEstablish)
		membersLabel.SetText("")

		serverAddr, stunAddr := g.serverAddr(), g.stunAddr()
		token := tokenEntry.Text
		verbose := verboseCheck.Checked

		// First learn the network's membership/CIDR (a one-shot,
		// TUN-free control-plane call, same as "spur join-network") so
		// the status line can show something meaningful right away;
		// StartMesh below repeats the JoinNetwork call internally (it's
		// idempotent for an already-known member — see
		// usecase.JoinNetwork's doc comment) as part of actually
		// creating the TUN device.
		go func() {
			preview, err := g.client.JoinNetwork(context.Background(), serverAddr, name, token, nil)
			if err == nil {
				fyne.Do(func() {
					names := make([]string, 0, len(preview.Members))
					for _, m := range preview.Members {
						names = append(names, m.PeerID+" ("+m.MeshIP+")")
					}
					membersLabel.SetText(fmt.Sprintf(cat.MeshMembers, strings.Join(names, ", ")))
				})
			}

			session, err := g.client.StartMesh(context.Background(), serverAddr, stunAddr, name, token, verbose, func(string) {}, g.onVersionMismatch)
			if err != nil {
				fyne.Do(func() {
					statusLabel.SetText(fmt.Sprintf(cat.MeshStatusFailed, cli.Explain(err)))
					joinBtn.Enable()
				})
				return
			}

			fyne.Do(func() {
				active = session
				leaveBtn.Enable()
				statusLabel.SetText(fmt.Sprintf(cat.MeshStatusJoined, preview.CIDR))
			})

			waitErr := session.Wait()
			fyne.Do(func() {
				if !errors.Is(waitErr, context.Canceled) && waitErr != nil {
					statusLabel.SetText(fmt.Sprintf(cat.MeshStatusFailed, cli.Explain(waitErr)))
				} else {
					statusLabel.SetText(cat.MeshStatusIdle)
				}
				active = nil
				joinBtn.Enable()
				leaveBtn.Disable()
			})
		}()
	}

	leaveBtn.OnTapped = func() {
		if active != nil {
			active.Stop()
		}
		leaveBtn.Disable()
	}

	form := container.NewVBox(
		widget.NewLabel(cat.MeshNetworkName),
		nameEntry,
		widget.NewLabel(cat.MeshInviteToken),
		tokenEntry,
		verboseCheck,
		container.NewHBox(joinBtn, leaveBtn),
		statusLabel,
		membersLabel,
	)
	return container.NewVScroll(form)
}
