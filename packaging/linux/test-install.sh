#!/bin/sh
# Runs install.sh against a stage (a release stage, or a stub built here) in
# throwaway homes: install, re-run, upgrade, XDG paths, HOME with a space, and uninstall.
# Usage: sh packaging/linux/test-install.sh [stage-dir]
set -eu

# the test must not pick up the caller's XDG dirs, and must never run as root
unset XDG_DATA_HOME XDG_CONFIG_HOME
fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
if [ "$(id -u)" = 0 ]; then fail "run the test as a non-root user"; fi

here=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repo=$here/../..
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

if [ "$#" -ge 1 ]; then
  # absolute, because the cases below cd into their homes
  stage=$(CDPATH='' cd -- "$1" && pwd)
else
  stage=$work/stage
  mkdir "$stage"
  true_bin=/usr/bin/true
  if [ ! -x "$true_bin" ]; then true_bin=/bin/true; fi
  # a dynamic binary that needs only libc, so the installer's ldd check passes
  cp "$true_bin" "$stage/manga-downloader-gui"
  cp "$repo/gui/build/linux/icon.png" "$stage/manga-downloader-gui.png"
  cp "$here/manga-downloader-gui.desktop" "$here/install.sh" "$stage/"
  chmod 0755 "$stage/manga-downloader-gui" "$stage/install.sh"
fi
if [ ! -f "$stage/install.sh" ]; then fail "no install.sh in $stage"; fi

path_ok=/usr/bin:/bin

# case 1: fresh install, with the bin dir not on PATH
h1=$work/h1
mkdir "$h1"
out=$(HOME="$h1" PATH=$path_ok sh "$stage/install.sh" 2>&1) || fail "install: $out"
bin=$h1/.local/bin/manga-downloader-gui
icon=$h1/.local/share/icons/hicolor/256x256/apps/manga-downloader-gui.png
desk=$h1/.local/share/applications/manga-downloader-gui.desktop
[ -x "$bin" ] || fail "binary isn't executable at $bin"
cmp -s "$bin" "$stage/manga-downloader-gui" || fail "installed binary differs from the stage"
cmp -s "$icon" "$stage/manga-downloader-gui.png" || fail "installed icon differs from the stage"
grep -qxF "Exec=$bin" "$desk" || fail "Exec= isn't the binary's path"
[ "$(grep -c '^Exec=' "$desk")" = 1 ] || fail "expected exactly one Exec= line"
grep -qxF 'Icon=manga-downloader-gui' "$desk" || fail "Icon= is missing"
case $out in
  *"isn't on your PATH"*) ;;
  *) fail "no PATH warning: $out" ;;
esac
case $out in
  *"doesn't have"*) fail "unexpected missing-library warning: $out" ;;
esac
[ -z "$(find "$h1" -name '*.new')" ] || fail "temporary files left behind"
if command -v desktop-file-validate >/dev/null 2>&1; then
  desktop-file-validate "$desk" || fail "desktop-file-validate rejects $desk"
else
  printf '%s\n' "skip: desktop-file-validate isn't installed"
fi

# case 2: re-run with the bin dir on PATH
out=$(HOME="$h1" PATH="$h1/.local/bin:$path_ok" sh "$stage/install.sh" 2>&1) || fail "re-run: $out"
[ "$(grep -c '^Exec=' "$desk")" = 1 ] || fail "re-run left more than one Exec= line"
grep -qxF "Exec=$bin" "$desk" || fail "re-run changed the Exec= line"
case $out in
  *"isn't on your PATH"*) fail "PATH warning after the bin dir was added: $out" ;;
esac

# case 3: upgrade in place from a newer stage
cp -R "$stage" "$work/stage2"
printf x >> "$work/stage2/manga-downloader-gui.png"
out=$(HOME="$h1" PATH="$h1/.local/bin:$path_ok" sh "$work/stage2/install.sh" 2>&1) || fail "upgrade: $out"
cmp -s "$icon" "$work/stage2/manga-downloader-gui.png" || fail "upgrade didn't replace the icon"

# case 4: absolute XDG_DATA_HOME with a space, then uninstall with the same env
h4=$work/h4
mkdir "$h4"
xdg="$work/xdg data"
out=$(HOME="$h4" XDG_DATA_HOME="$xdg" PATH=$path_ok sh "$stage/install.sh" 2>&1) || fail "XDG install: $out"
xdg_desk=$xdg/applications/manga-downloader-gui.desktop
xdg_icon=$xdg/icons/hicolor/256x256/apps/manga-downloader-gui.png
[ -f "$xdg_desk" ] || fail "desktop file isn't under XDG_DATA_HOME"
[ -f "$xdg_icon" ] || fail "icon isn't under XDG_DATA_HOME"
grep -qxF "Exec=$h4/.local/bin/manga-downloader-gui" "$xdg_desk" || fail "XDG install: Exec= isn't the binary's path"
HOME="$h4" XDG_DATA_HOME="$xdg" sh "$stage/install.sh" --uninstall >/dev/null 2>&1 || fail "XDG uninstall"
[ ! -e "$xdg_desk" ] || fail "XDG uninstall left the desktop file"
[ ! -e "$xdg_icon" ] || fail "XDG uninstall left the icon"
[ ! -e "$h4/.local/bin/manga-downloader-gui" ] || fail "XDG uninstall left the binary"

# case 4b: a relative XDG_DATA_HOME is ignored in favour of ~/.local/share
h5=$work/h5
mkdir "$h5"
(cd "$h5" && HOME="$h5" XDG_DATA_HOME=rel PATH=$path_ok sh "$stage/install.sh" >/dev/null 2>&1) || fail "relative XDG install"
[ -f "$h5/.local/share/applications/manga-downloader-gui.desktop" ] || fail "relative XDG_DATA_HOME wasn't ignored"
[ ! -e "$h5/rel" ] || fail "relative XDG_DATA_HOME was used"

# case 5: HOME with a space, so Exec= must be quoted
h2="$work/home with space"
mkdir "$h2"
out=$(HOME="$h2" PATH=$path_ok sh "$stage/install.sh" 2>&1) || fail "HOME with a space: $out"
grep -qxF "Exec=\"$h2/.local/bin/manga-downloader-gui\"" "$h2/.local/share/applications/manga-downloader-gui.desktop" \
  || fail "HOME with a space: Exec= isn't quoted"
if command -v desktop-file-validate >/dev/null 2>&1; then
  desktop-file-validate "$h2/.local/share/applications/manga-downloader-gui.desktop" \
    || fail "desktop-file-validate rejects the quoted Exec="
fi

# case 6: uninstall removes the three files and keeps the bin dir; a second run is a no-op
HOME="$h1" sh "$stage/install.sh" --uninstall >/dev/null || fail "uninstall"
for f in "$bin" "$desk" "$icon"; do
  [ ! -e "$f" ] || fail "uninstall left $f"
done
[ -d "$h1/.local/bin" ] || fail "uninstall removed the bin dir"
HOME="$h1" sh "$stage/install.sh" --uninstall >/dev/null || fail "second uninstall"

# case 7: a bad argument is a usage error
if sh "$stage/install.sh" --bogus >/dev/null 2>&1; then
  fail "--bogus was accepted"
else
  rc=$?
  [ "$rc" = 2 ] || fail "--bogus exited $rc, expected 2"
fi

# case 8: without the app's files, nothing is installed
lonely=$work/lonely
mkdir "$lonely"
cp "$stage/install.sh" "$lonely/"
h3=$work/h3
mkdir "$h3"
if HOME="$h3" sh "$lonely/install.sh" >/dev/null 2>&1; then
  fail "install without the app's files succeeded"
fi
[ ! -e "$h3/.local/bin/manga-downloader-gui" ] || fail "a failed install left a binary behind"

# case 9: HOME unset is refused
if env -u HOME sh "$stage/install.sh" >/dev/null 2>&1; then
  fail "install with HOME unset succeeded"
fi

printf 'install.sh: all checks passed\n'
