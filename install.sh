#!/bin/sh
# Installs spur and spur-server, and — this is the actual point of this
# script rather than just copying two files — adds the install directory
# to PATH for future shell sessions if it isn't already there. A running
# process can't modify its parent shell's environment, so "automatic" here
# means: detect the user's shell and append the right PATH line to its rc
# file, idempotently. You still need to restart the shell (or `source` the
# rc file) once for it to take effect in the *current* session — no
# installer can skip that, it's how shells work.
#
# Two ways this runs:
#   curl -fsSL https://raw.githubusercontent.com/fu1se/SPUR/master/install.sh | sh
#       downloads prebuilt binaries from the latest GitHub release —
#       no Go toolchain needed, works on a bare machine.
#   make install
#       reuses this same script (via SPUR_LOCAL_BUILD_DIR) to install
#       binaries already built locally by `make build`, so the PATH setup
#       is identical either way instead of two separate implementations.
set -eu

REPO="fu1se/SPUR"
INSTALL_DIR="${SPUR_INSTALL_DIR:-$HOME/.local/bin}"

detect_platform() {
	os=$(uname -s)
	arch=$(uname -m)
	case "$os" in
		Linux) os=linux ;;
		Darwin) os=darwin ;;
		*)
			echo "install.sh: unsupported OS: $os" >&2
			echo "Windows isn't covered by this script — download spur-windows-amd64.exe / spur-server-windows-amd64.exe from https://github.com/$REPO/releases/latest and add their folder to PATH manually (System Properties -> Environment Variables)." >&2
			exit 1
			;;
	esac
	case "$arch" in
		x86_64|amd64) arch=amd64 ;;
		arm64|aarch64) arch=arm64 ;;
		*)
			echo "install.sh: unsupported architecture: $arch" >&2
			exit 1
			;;
	esac
	echo "$os-$arch"
}

download_binary() {
	platform="$1"
	name="$2"
	url="https://github.com/$REPO/releases/latest/download/${name}-${platform}"
	echo "downloading $url"
	# Downloads into a .partial sidecar, not straight to $INSTALL_DIR/$name:
	# -C - resumes that sidecar from wherever a previous attempt left off
	# rather than restarting from byte zero across retries (see below for
	# why --retry-all-errors is also needed for this to matter), and only
	# `mv` replaces the real destination once the download actually
	# finishes — same reasoning as install_binary's rm-before-cp: `mv`
	# is a rename(2), which atomically replaces $INSTALL_DIR/$name even if
	# a currently-running process (e.g. a long-lived "spur join" mesh
	# session) still has the old binary open/executing, instead of
	# failing with "Text file busy" the way writing straight into that
	# path would.
	#
	# --retry-all-errors also retries on errors curl wouldn't otherwise
	# consider transient (e.g. exit code 23, "failed writing body" — the
	# actual failure seen on a real flaky/unstable connection: spur-server
	# is the larger of the two binaries and consistently died partway
	# through at a different byte offset each attempt, never the same one
	# twice, so it wasn't a fixed protocol/MTU boundary — just an ordinary
	# dropped connection on a multi-second transfer). Plain --retry alone
	# only retries connection-level failures, not write errors, so it
	# wouldn't have helped here.
	partial="$INSTALL_DIR/$name.partial"
	curl -fL --retry 5 --retry-delay 2 --retry-all-errors -C - "$url" -o "$partial"
	chmod +x "$partial"
	mv -f "$partial" "$INSTALL_DIR/$name"
}

# install_binary copies src to dst, replacing whatever's already there —
# rm before cp, not cp -f (which just truncates-and-rewrites the
# existing inode in place): a currently-running process still executing
# the old binary at dst (e.g. a long-lived "spur join" mesh session,
# found live on this exact machine, upgrading spur/spur-gui/spur-server
# while one was already running) holds that inode open, and overwriting
# it in place fails with "Text file busy" — or worse, could corrupt the
# running process's mapped pages on some platforms. Unlinking dst first
# and creating a fresh inode leaves the running process's already-open
# fd pointing at the old (now nameless but still valid) inode, unaffected,
# while the new binary takes over the name for the next invocation — the
# same trick every real package manager uses to upgrade a running binary.
install_binary() {
	src="$1"
	dst="$2"
	rm -f "$dst"
	cp "$src" "$dst"
	chmod +x "$dst"
}

# add_path_line_once appends line to file exactly once, creating the
# file's directory if needed. Safe to call on every install.sh run.
add_path_line_once() {
	file="$1"
	line="$2"
	if [ -f "$file" ] && grep -qF "$line" "$file" 2>/dev/null; then
		return 0
	fi
	mkdir -p "$(dirname "$file")"
	printf '\n# added by spur install.sh\n%s\n' "$line" >> "$file"
	echo "added \"$line\" to $file"
	path_line_added=1
}

mkdir -p "$INSTALL_DIR"

if [ -n "${SPUR_LOCAL_BUILD_DIR:-}" ]; then
	install_binary "$SPUR_LOCAL_BUILD_DIR/spur" "$INSTALL_DIR/spur"
	install_binary "$SPUR_LOCAL_BUILD_DIR/spur-server" "$INSTALL_DIR/spur-server"
	# spur-gui only exists here when it was actually built locally
	# (`make build`/`make install`) — it's never downloaded below, since
	# GitHub releases don't carry it yet (see Makefile's release target
	# comment: Fyne needs a native cgo build per platform, not something
	# this script's prebuilt-binary download path can offer today).
	if [ -f "$SPUR_LOCAL_BUILD_DIR/spur-gui" ]; then
		install_binary "$SPUR_LOCAL_BUILD_DIR/spur-gui" "$INSTALL_DIR/spur-gui"
	fi
else
	platform=$(detect_platform)
	download_binary "$platform" spur
	download_binary "$platform" spur-server
fi

echo "installed spur and spur-server to $INSTALL_DIR"
if [ -f "$INSTALL_DIR/spur-gui" ]; then
	echo "installed spur-gui (GUI client) to $INSTALL_DIR"
	# Applications-menu entry: only meaningful on Linux (freedesktop
	# .desktop files — macOS needs a .app bundle instead, out of scope),
	# and only when this is a local build (SPUR_LOCAL_BUILD_DIR): the
	# icon has to come from an actual image file on disk — a .desktop
	# Icon= key can't point inside the binary the way go:embed does —
	# and the curl|sh download path never has spur-gui or the repo tree
	# it'd need the icon from in the first place (see the comment above).
	if [ "$(uname -s)" = "Linux" ] && [ -n "${SPUR_LOCAL_BUILD_DIR:-}" ]; then
		script_dir=$(cd "$(dirname "$0")" && pwd)
		icon_src="$script_dir/cmd/spur-gui/assets/icon.png"
		if [ -f "$icon_src" ]; then
			data_dir="${XDG_DATA_HOME:-$HOME/.local/share}"
			icon_dir="$data_dir/spur"
			apps_dir="$data_dir/applications"
			mkdir -p "$icon_dir" "$apps_dir"
			cp "$icon_src" "$icon_dir/icon.png"
			cat > "$apps_dir/spur-gui.desktop" <<DESKTOPEOF
[Desktop Entry]
Type=Application
Name=spur
Name[ru]=spur
Comment=P2P tunnel and mesh VPN client
Comment[ru]=P2P-туннель и mesh VPN клиент
Exec=$INSTALL_DIR/spur-gui
Icon=$icon_dir/icon.png
Terminal=false
Categories=Network;
DESKTOPEOF
			# Not fatal if missing — GNOME/KDE both also just watch the
			# applications directory directly and pick up new .desktop
			# files without it; this only speeds up some desktops noticing.
			command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database "$apps_dir" >/dev/null 2>&1
			echo "added spur-gui to the applications menu ($apps_dir/spur-gui.desktop)"
		fi
	fi
fi

case ":$PATH:" in
	*":$INSTALL_DIR:"*)
		echo "$INSTALL_DIR is already in PATH — nothing more to do, 'spur' should work right now"
		exit 0
		;;
esac

path_line_added=0
shell_name=$(basename "${SHELL:-sh}")

case "$shell_name" in
	fish)
		add_path_line_once "$HOME/.config/fish/config.fish" "fish_add_path $INSTALL_DIR"
		;;
	zsh)
		add_path_line_once "$HOME/.zshrc" "export PATH=\"$INSTALL_DIR:\$PATH\""
		;;
	bash)
		# macOS's default Terminal.app starts a login shell, which reads
		# ~/.bash_profile, not ~/.bashrc; most Linux distros' interactive
		# shells read ~/.bashrc. Prefer ~/.bash_profile only if it
		# already exists (i.e. this is likely macOS or someone already
		# set one up); otherwise ~/.bashrc.
		if [ -f "$HOME/.bash_profile" ]; then
			add_path_line_once "$HOME/.bash_profile" "export PATH=\"$INSTALL_DIR:\$PATH\""
		else
			add_path_line_once "$HOME/.bashrc" "export PATH=\"$INSTALL_DIR:\$PATH\""
		fi
		;;
	*)
		echo "unrecognized shell '$shell_name' — add $INSTALL_DIR to PATH manually"
		;;
esac

if [ "$path_line_added" = 1 ]; then
	echo "restart your shell (or run 'source' on the file above) to use spur/spur-server as plain commands"
fi
