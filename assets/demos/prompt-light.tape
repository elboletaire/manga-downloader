# Recreates assets/demos/prompt-light.gif, the light twin of prompt.tape (keep the two in step)
# — run from the repo root: vhs assets/demos/prompt-light.tape
# Requires the demo binary built at the repo root (make build/demo): it serves the
# series from demo/series, so the run needs no network (see demo/README.md).
# Uses a short (5 chapter) series so answering "y" downloads everything quickly.
# One Piece on MangaDex, filtered to Catalan ("ca"), the language the makefile's
# MangaDex smoke target uses, because the series is translated into several
# languages and the unfiltered download would show a bar per language.
Output assets/demos/prompt-light.gif

Set Shell "bash"
Set FontSize 14
Set Width 1104
Set Height 230
Set Padding 16
Set WindowBar Colorful
Set WindowBarSize 32
Set Theme "OneHalfLight"
Set Framerate 10
Set PlaybackSpeed 3
Set WaitTimeout 600s

Hide
Type 'export MANGA_DOWNLOADER_DEMO_DELAY=0.7 && export PATH="$PWD:$PATH" && mkdir -p /tmp/manga-downloader-demo/prompt && cd /tmp/manga-downloader-demo/prompt && rm -rf ./* && clear'
Enter
Show

Type "manga-downloader https://mangadex.org/title/a1c7c817-4e59-43b7-9365-09675a149a6f/one-piece --language ca"
Sleep 500ms
Enter
Wait+Screen@60s /y\/N/
Sleep 1s
Type "y"
Sleep 500ms
Enter
Wait
Sleep 3s
