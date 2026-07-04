// Command spur-gui is the desktop GUI client composition root: it wires
// guiapp (internal/adapter/guiapp) into a Fyne window instead of cobra
// commands. It is a peer of cmd/spur (the CLI), not a replacement —
// both binaries share the exact same identity file, known_servers trust
// store, and config.json (see internal/infra), so switching between
// "spur connect" on the command line and this GUI on the same machine
// just works, no separate setup.
//
// No business logic lives here beyond what's needed to translate button
// clicks into guiapp calls and guiapp callbacks into widget updates —
// exactly the same "composition root, not business logic" rule
// cmd/spur/main.go documents for the CLI.
package main

import (
	_ "embed"
	"fmt"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/fu1se/spur/internal/adapter/cli"
	"github.com/fu1se/spur/internal/adapter/guiapp"
	"github.com/fu1se/spur/internal/infra"
)

// appIconPNG is the same launcher icon the Android app ships (generated
// from the product's actual logo.jpg — see CLAUDE.md's "GUI и иконка
// приложения" note) — one visual identity across both clients instead of
// Fyne's default placeholder icon.
//
//go:embed assets/icon.png
var appIconPNG []byte

// guiApp bundles the state every tab needs: the fyne window (for showing
// dialogs), the loaded guiapp.Client (identity), the persisted config
// (server/stun address, shared with the CLI's config.json), and the
// active language catalog.
type guiApp struct {
	app     fyne.App
	win     fyne.Window
	client  *guiapp.Client
	cfg     infra.Config
	cfgPath string
	cat     *catalog

	serverEntry *widget.Entry
	stunEntry   *widget.Entry
}

func main() {
	cfgPath, err := infra.DefaultConfigPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cfg, err := infra.LoadConfig(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	lang := cfg.Lang
	if lang == "" {
		lang = infra.DetectSystemLanguage()
	}
	cat := catalogFor(lang)

	client, err := guiapp.NewClient(cfg.Identity)
	if err != nil {
		fmt.Fprintln(os.Stderr, cat.IdentityLoadFailed+": "+cli.Explain(err))
		os.Exit(1)
	}

	fyneApp := app.NewWithID("dev.spur.gui")
	fyneApp.Settings().SetTheme(spurTheme{})
	icon := fyne.NewStaticResource("icon.png", appIconPNG)
	fyneApp.SetIcon(icon)

	win := fyneApp.NewWindow(cat.WindowTitle)
	win.SetIcon(icon)

	g := &guiApp{
		app:         fyneApp,
		win:         win,
		client:      client,
		cfg:         cfg,
		cfgPath:     cfgPath,
		cat:         cat,
		serverEntry: widget.NewEntry(),
		stunEntry:   widget.NewEntry(),
	}
	g.serverEntry.SetText(cfg.Server)
	g.serverEntry.SetPlaceHolder("host:4443")
	g.stunEntry.SetText(cfg.StunServer)
	g.stunEntry.SetPlaceHolder("host:4444")

	// Icons mirror the Android app's SectionCard icons (Components.kt) as
	// closely as Fyne's built-in theme icon set allows — Fyne has no
	// direct equivalent of Material's Fingerprint/Router/FolderShared/
	// MeetingRoom/Hub, so each is the closest available stand-in
	// (Account for identity, Computer for port-forward, Folder for file
	// transfer, Home for a room, Desktop for a multi-machine mesh).
	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon(cat.TabIdentity, theme.AccountIcon(), g.buildIdentityTab()),
		container.NewTabItemWithIcon(cat.TabPortForward, theme.ComputerIcon(), g.buildPortForwardTab()),
		container.NewTabItemWithIcon(cat.TabTransfer, theme.FolderIcon(), g.buildTransferTab()),
		container.NewTabItemWithIcon(cat.TabRooms, theme.HomeIcon(), g.buildRoomsTab()),
		container.NewTabItemWithIcon(cat.TabMesh, theme.DesktopIcon(), g.buildMeshTab()),
	)
	tabs.SetTabLocation(container.TabLocationTop)

	win.SetContent(tabs)
	win.Resize(fyne.NewSize(720, 560))
	win.ShowAndRun()
}

// saveSettings persists the current server/stun-server fields to
// config.json — the same file `spur lang` already writes to (see
// infra.Config's doc comment), now with a second writer: the GUI editing
// server/stun-server, cli/lang.go editing Lang. Both go through
// infra.SaveConfig's atomic write, so a GUI save racing a concurrent
// `spur lang` invocation can't corrupt the file, only last-write-wins
// one field at a time.
func (g *guiApp) saveSettings() {
	g.cfg.Server = g.serverEntry.Text
	g.cfg.StunServer = g.stunEntry.Text
	if err := infra.SaveConfig(g.cfgPath, g.cfg); err != nil {
		dialog.ShowError(err, g.win)
		return
	}
	dialog.ShowInformation(g.cat.WindowTitle, g.cat.SettingsSaved, g.win)
}

// serverAddr/stunAddr read the live entry text (not g.cfg, which only
// reflects the last save) — every tab's network operations should use
// whatever's currently typed in, saved or not, same as a CLI flag
// overriding a config default for a single invocation.
func (g *guiApp) serverAddr() string { return g.serverEntry.Text }
func (g *guiApp) stunAddr() string   { return g.stunEntry.Text }

// onVersionMismatch surfaces guiapp's version-mismatch hint the same way
// the CLI does (a warning, not a hard failure) — shown as an information
// dialog rather than blocking the flow it fired from.
func (g *guiApp) onVersionMismatch(clientVersion, serverVersion string) {
	fyne.Do(func() {
		dialog.ShowInformation(g.cat.WindowTitle,
			fmt.Sprintf("client %s / server %s", clientVersion, serverVersion), g.win)
	})
}

func (g *guiApp) showError(err error) {
	fyne.Do(func() {
		dialog.ShowError(fmt.Errorf("%s", cli.Explain(err)), g.win)
	})
}
