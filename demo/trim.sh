#!/bin/sh
# Trims a recording (see README.md) to the chapters the tapes request and writes
# it to a fixtures directory: trim.sh <recording dir> <fixtures dir>.
# Needs jq and perl. Responses the tapes read whole (page lists, at-home
# servers) are copied as they were recorded.
set -eu

if [ $# -ne 2 ]; then
	echo "usage: $0 <recording dir> <fixtures dir>" >&2
	exit 2
fi
src=$1
dst=$2

mkdir -p "$dst/bodies"
cp "$src/manifest.json" "$dst/manifest.json"

jq -r '.[] | select(.file != null and .file != "") | "\(.file)\t\(.url)"' "$src/manifest.json" |
while IFS="$(printf '\t')" read -r file url; do
	in="$src/bodies/$file"
	out="$dst/bodies/$file"
	case $url in
	*/chapter/getall*)
		# inmanga's chapter list: the chapters of the tape's range, 1140-1147
		jq -c '.data |= (fromjson | .result |= map(select(.Number >= 1140 and .Number <= 1147)) | tojson)' "$in" >"$out"
		;;
	*/chapterIndexControls*)
		# only the page list is read, from the first of the duplicated selects
		perl -0777 -ne 'print "<html><body>$1</body></html>\n" if /(<select[^>]*PageListClass.*?<\/select>)/s' "$in" >"$out"
		;;
	*inmanga.com/ver/manga/*)
		# the series page is read for its title only
		perl -0777 -ne 'print "<html><body>$1</body></html>\n" if /(<h1>.*?<\/h1>)/s' "$in" >"$out"
		;;
	*/feed?*)
		# MangaDex feeds: chapters 1-13, the most either tape requests
		jq -c '.data |= map(select((.attributes.chapter | tonumber? // 0) as $n | $n >= 1 and $n <= 13))' "$in" >"$out"
		;;
	*)
		cp "$in" "$out"
		;;
	esac
done
