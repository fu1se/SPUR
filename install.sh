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
	# -C - resumes from wherever a previous attempt left off instead of
	# restarting from byte zero, and --retry-all-errors also retries on
	# errors curl wouldn't otherwise consider transient (e.g. exit code
	# 23, "failed writing body" — the actual failure seen on a real
	# flaky/unstable connection: spur-server is the larger of the two
	# binaries and consistently died partway through at a different byte
	# offset each attempt, never the same one twice, so it wasn't a fixed
	# protocol/MTU boundary — just an ordinary dropped connection on a
	# multi-second transfer). Plain --retry alone only retries connection-
	# level failures, not write errors, so it wouldn't have helped here.
	curl -fL --retry 5 --retry-delay 2 --retry-all-errors -C - "$url" -o "$INSTALL_DIR/$name"
	chmod +x "$INSTALL_DIR/$name"
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
	cp "$SPUR_LOCAL_BUILD_DIR/spur" "$INSTALL_DIR/spur"
	cp "$SPUR_LOCAL_BUILD_DIR/spur-server" "$INSTALL_DIR/spur-server"
	chmod +x "$INSTALL_DIR/spur" "$INSTALL_DIR/spur-server"
else
	platform=$(detect_platform)
	download_binary "$platform" spur
	download_binary "$platform" spur-server
fi

echo "installed spur and spur-server to $INSTALL_DIR"

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
