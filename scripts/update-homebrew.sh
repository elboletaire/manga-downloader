#!/usr/bin/env bash
# Sets the version and the sha256s of a Homebrew formula or cask to a release.
# The release workflows run it on a copy of packaging/homebrew/tap/ inside the
# tap checkout; it works the same by hand.
#
#   scripts/update-homebrew.sh <version> <formula-or-cask.rb> <assets-dir>
#
# Each url is resolved against the release: #{version} is replaced and the
# asset named by the url's last segment is read from <assets-dir>, so a formula
# with four platform archives and a cask with one zip are handled alike. Each
# url's sha256 must sit right next to it: the nearest line after the url in a
# formula (every on_macos/on_linux block has url, then sha256), the nearest line
# before it in a cask (its one sha256 comes first, the stanza order brew style
# enforces). Blank lines in between don't count. Anything else fails. Prints
# whether the file changed and touches nothing else.
set -euo pipefail

usage() { echo "usage: $0 <version> <formula-or-cask.rb> <assets-dir>" >&2; exit 2; }
[ $# -eq 3 ] || usage

version="${1#v}"
file="$2"
assets="$3"
name="$(basename "$file")"

if ! [[ "$version" =~ ^[0-9]+(\.[0-9]+)*$ ]]; then
  echo "error: '$1' isn't a version like 1.11.1" >&2
  exit 1
fi
[ -f "$file" ] || { echo "error: no file at $file" >&2; exit 1; }
[ -d "$assets" ] || { echo "error: no directory at $assets" >&2; exit 1; }

# A cask puts its sha256 before its url, a formula after; the file's first
# stanza says which one this is.
sha_before_url=0
if grep -qE '^cask "' "$file"; then sha_before_url=1; fi

# Exactly one top-level version line, and as many sha256 lines as urls, or the
# file has changed shape and this script needs to change with it.
re_version='^  version "[^"]*"$'
re_url='^[[:space:]]*url "([^"]*)"$'
re_sha='^[[:space:]]*sha256 "[^"]*"$'
[ "$(grep -cE "$re_version" "$file" || true)" = 1 ] || {
  echo "error: expected one '  version \"...\"' line in $file" >&2; exit 1; }
urls="$(grep -cE '^[[:space:]]*url "' "$file" || true)"
shas="$(grep -cE '^[[:space:]]*sha256 "' "$file" || true)"
if [ "$urls" = 0 ] || [ "$urls" != "$shas" ]; then
  echo "error: $file has $urls url and $shas sha256 lines" >&2
  exit 1
fi

sha256_of() {
  if command -v sha256sum >/dev/null; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

lines=()
while IFS= read -r line || [ -n "$line" ]; do
  lines+=("$line")
done < "$file"
n=${#lines[@]}

# First pass: each url's sha256 goes to the line next to it, which must be a
# sha256 line. Indexed by line number, so a sparse array is enough.
sums_at=()
for ((i = 0; i < n; i++)); do
  [[ "${lines[i]}" =~ $re_url ]] || continue
  url="${BASH_REMATCH[1]}"
  step=1
  if [ "$sha_before_url" = 1 ]; then step=-1; fi
  j=$((i + step))
  while [ "$j" -ge 0 ] && [ "$j" -lt "$n" ] && [ -z "${lines[j]//[[:space:]]/}" ]; do
    j=$((j + step))
  done
  if [ "$j" -lt 0 ] || [ "$j" -ge "$n" ] || ! [[ "${lines[j]}" =~ $re_sha ]]; then
    echo "error: the url at line $((i + 1)) of $name has no sha256 line next to it" >&2
    exit 1
  fi
  asset="${url##*/}"
  asset="${asset//'#{version}'/$version}"
  if [[ "$asset" == *'#{'* ]]; then
    echo "error: can't resolve '$url' in $name" >&2
    exit 1
  fi
  if [ ! -f "$assets/$asset" ]; then
    echo "error: $name needs $asset, which isn't in $assets" >&2
    exit 1
  fi
  sums_at[j]="$(sha256_of "$assets/$asset")"
done

# Second pass: write the version and each sha256 line, copy everything else.
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
for ((i = 0; i < n; i++)); do
  line="${lines[i]}"
  if [[ "$line" =~ $re_version ]]; then
    printf '  version "%s"\n' "$version" >> "$tmp"
  elif [[ "$line" =~ $re_sha ]]; then
    if [ -z "${sums_at[i]:-}" ]; then
      echo "error: the sha256 at line $((i + 1)) of $name isn't next to a url" >&2
      exit 1
    fi
    printf '%ssha256 "%s"\n' "${line%%sha256*}" "${sums_at[i]}" >> "$tmp"
  else
    printf '%s\n' "$line" >> "$tmp"
  fi
done

if cmp -s "$tmp" "$file"; then
  echo "$name already at $version"
else
  cat "$tmp" > "$file"
  echo "$name set to $version"
fi
