#!/bin/sh
# Installs Manga Downloader for the current user only, into ~/.local, so no
# root is needed. POSIX sh, so it runs under dash too.
set -eu

app=manga-downloader-gui
usage() {
  cat <<'EOF'
Usage: install.sh [--uninstall]

Installs Manga Downloader into ~/.local (or $XDG_DATA_HOME), for your user only.
Run it again to update. --uninstall removes what it installed.
EOF
}

mode=install
case $# in
  0) ;;
  1)
    case $1 in
      --uninstall) mode=uninstall ;;
      -h | --help) usage; exit 0 ;;
      *) usage >&2; exit 2 ;;
    esac
    ;;
  *) usage >&2; exit 2 ;;
esac

say() { printf '%s\n' "$*"; }
warn() { printf 'Warning: %s\n' "$*" >&2; }
die() { printf 'Error: %s\n' "$*" >&2; exit 1; }

# sudo keeps HOME, so a root run would leave root-owned files in the user's home
if [ "$(id -u)" = 0 ]; then
  die "run it as your own user, without sudo: it installs into your home folder"
fi
if [ -z "${HOME:-}" ]; then die "HOME isn't set"; fi
case $HOME in
  /*) ;;
  *) die "HOME isn't an absolute path: $HOME" ;;
esac

# XDG says an empty or relative base directory must be ignored
data=${XDG_DATA_HOME:-}
case $data in
  /*) ;;
  *) data=$HOME/.local/share ;;
esac
config=${XDG_CONFIG_HOME:-}
case $config in
  /*) ;;
  *) config=$HOME/.config ;;
esac

src_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
bin_dir=$HOME/.local/bin
bin=$bin_dir/$app
apps_dir=$data/applications
desktop=$apps_dir/$app.desktop
icons_root=$data/icons/hicolor
icon=$icons_root/256x256/apps/$app.png

tmp=
cleanup() {
  if [ -n "$tmp" ]; then rm -f "$tmp"; fi
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

# copies through a temporary file and a rename, so an upgrade works while the app runs
put() { # src dest mode
  tmp=$2.new
  cp "$1" "$tmp"
  chmod "$3" "$tmp"
  mv -f "$tmp" "$2"
  tmp=
}

refresh() {
  if [ -d "$apps_dir" ] && command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database -q "$apps_dir" >/dev/null 2>&1 || true
  fi
  # only refresh an existing cache: one created here goes stale when other apps add icons
  if [ -f "$icons_root/icon-theme.cache" ] && command -v gtk-update-icon-cache >/dev/null 2>&1; then
    gtk-update-icon-cache -q -t -f "$icons_root" >/dev/null 2>&1 || true
  elif [ -d "$icons_root" ]; then
    touch "$icons_root"
  fi
}

# tells the user which packages to install, without running anything
hint() {
  f=/etc/os-release
  if [ ! -r "$f" ]; then f=/usr/lib/os-release; fi
  ids=" $(sed -n -e 's/^ID=//p' -e 's/^ID_LIKE=//p' "$f" 2>/dev/null | tr -d "\"'" | tr '\n' ' ') "
  case $ids in
    *" debian "* | *" ubuntu "*) cmd='sudo apt install libwebkit2gtk-4.1-0' ;;
    *" fedora "* | *" rhel "* | *" centos "*) cmd='sudo dnf install webkit2gtk4.1' ;;
    *" arch "*) cmd='sudo pacman -S --needed webkit2gtk-4.1' ;;
    *" opensuse"* | *" suse "*) cmd='sudo zypper install libwebkit2gtk-4_1-0' ;;
    *) cmd= ;;
  esac
  if [ -n "$cmd" ]; then
    say "It needs GTK 3 and WebKitGTK 4.1. Install them with:"
    say "  $cmd"
  else
    say "It needs GTK 3 and WebKitGTK 4.1: install WebKitGTK 4.1 (it brings GTK 3) with your distribution's package manager."
  fi
}

if [ "$mode" = uninstall ]; then
  for f in "$bin" "$desktop" "$icon"; do
    if [ -e "$f" ] || [ -L "$f" ]; then
      rm -f "$f"
      say "Removed $f"
    fi
  done
  refresh
  say "Manga Downloader is uninstalled. Its settings in $config/manga-downloader are kept (the command line tool uses that folder too)."
  exit 0
fi

for f in "$app" "$app.desktop" "$app.png"; do
  if [ ! -f "$src_dir/$f" ]; then
    die "$src_dir/$f is missing: run install.sh from the extracted manga-downloader-gui archive"
  fi
done

# the Exec= path is written into the .desktop file, so it must survive the desktop-entry syntax
nl='
'
case $bin in
  *'"'* | *'`'* | *'$'* | *\\* | *'%'* | *"$nl"*)
    die "can't write a menu entry for $bin: its path has characters .desktop files can't hold"
    ;;
esac
case $bin in
  *[!A-Za-z0-9/._+-]*) exec_value="\"$bin\"" ;;
  *) exec_value=$bin ;;
esac

mkdir -p "$bin_dir" "$apps_dir" "$icons_root/256x256/apps"
put "$src_dir/$app" "$bin" 0755
put "$src_dir/$app.png" "$icon" 0644

# only Exec= changes: the template is otherwise shipped as is
tmp=$desktop.new
while IFS= read -r line || [ -n "$line" ]; do
  case $line in
    Exec=*) printf 'Exec=%s\n' "$exec_value" ;;
    *) printf '%s\n' "$line" ;;
  esac
done < "$src_dir/$app.desktop" >| "$tmp"
chmod 0644 "$tmp"
mv -f "$tmp" "$desktop"
tmp=

refresh

say "Installed Manga Downloader:"
say "  $bin"
say "  $desktop"
say "  $icon"
say "It's in your applications menu (it may take a moment, or a new login, to show up)."
say "Uninstall it with: sh \"$src_dir/install.sh\" --uninstall"

# a missing library is reported, never fatal: the install itself worked
if ! command -v ldd >/dev/null 2>&1; then
  say "Couldn't check the libraries the app needs (no ldd)."
  hint
elif ! out=$(ldd "$bin" 2>&1); then
  say "Couldn't check the libraries the app needs: ${out%%"$nl"*}"
  hint
else
  missing=$(printf '%s\n' "$out" | sed -n 's/^[[:space:]]*\([^[:space:]]*\) => not found.*/\1/p' | tr '\n' ' ')
  if [ -n "$missing" ]; then
    warn "The app needs libraries this system doesn't have: ${missing% }"
    hint
  fi
fi

case ":${PATH:-}:" in
  *":$bin_dir:"* | *":$bin_dir/:"*) ;;
  *) warn "$bin_dir isn't on your PATH yet (many distros add it at your next login). The menu entry works anyway; to run $app or \`manga-downloader ui\` from a terminal, add it, e.g. in ~/.profile: export PATH=\"\$HOME/.local/bin:\$PATH\"" ;;
esac
