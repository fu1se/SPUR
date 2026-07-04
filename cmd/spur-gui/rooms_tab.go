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

// buildRoomsTab is "spur room create"/"spur room join": a standing,
// persistent two-member binding to use as a --room/room counterpart on
// the port-forward and transfer tabs, instead of a peer ID or a
// short-lived pairing code — see rendezvous.RoomCounterpart's doc
// comment for what a room is for.
func (g *guiApp) buildRoomsTab() fyne.CanvasObject {
	cat := g.cat

	createNameEntry := widget.NewEntry()
	createNameEntry.SetPlaceHolder(cat.RoomName)
	createStatus := newStatusLabel("")
	createBtn := widget.NewButtonWithIcon(cat.RoomCreate, theme.ContentAddIcon(), nil)
	createBtn.OnTapped = func() {
		name := createNameEntry.Text
		if name == "" {
			return
		}
		createBtn.Disable()
		serverAddr := g.serverAddr()
		go func() {
			result, err := g.client.CreateRoom(context.Background(), serverAddr, name, g.onVersionMismatch)
			fyne.Do(func() {
				createBtn.Enable()
				if err != nil {
					setStatus(createStatus, cli.Explain(err))
					return
				}
				setStatus(createStatus, fmt.Sprintf(cat.RoomCreated, result.InviteToken))
			})
		}()
	}

	joinNameEntry := widget.NewEntry()
	joinNameEntry.SetPlaceHolder(cat.RoomName)
	joinTokenEntry := widget.NewEntry()
	joinTokenEntry.SetPlaceHolder(cat.RoomInviteToken)
	joinStatus := newStatusLabel("")
	joinBtn := widget.NewButtonWithIcon(cat.RoomJoin, theme.LoginIcon(), nil)
	joinBtn.OnTapped = func() {
		name, token := joinNameEntry.Text, joinTokenEntry.Text
		if name == "" {
			return
		}
		joinBtn.Disable()
		serverAddr := g.serverAddr()
		go func() {
			err := g.client.JoinRoom(context.Background(), serverAddr, name, token, g.onVersionMismatch)
			fyne.Do(func() {
				joinBtn.Enable()
				if err != nil {
					setStatus(joinStatus, cli.Explain(err))
					return
				}
				setStatus(joinStatus, cat.RoomJoined)
			})
		}()
	}

	createCard := widget.NewCard(cat.RoomCreate, "", container.NewVBox(
		widget.NewLabel(cat.RoomName),
		createNameEntry,
		createBtn,
		createStatus,
	))
	joinCard := widget.NewCard(cat.RoomJoin, "", container.NewVBox(
		widget.NewLabel(cat.RoomName),
		joinNameEntry,
		widget.NewLabel(cat.RoomInviteToken),
		joinTokenEntry,
		joinBtn,
		joinStatus,
	))

	return container.NewVScroll(container.NewPadded(container.NewVBox(createCard, joinCard)))
}
