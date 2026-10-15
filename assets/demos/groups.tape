# Recreates assets/demos/groups.gif — run from the repo root: vhs assets/demos/groups.tape
# Requires the demo binary built at the repo root (make build/demo): it serves the
# series from demo/series, so the run needs no network (see demo/README.md).
# A series released by several scanlation groups: the first run keeps every release
# (the group in brackets, kept whole when the long title is cut with "…"), the second
# keeps only one group's with --scanlator.
Output assets/demos/groups.gif

Set Shell "bash"
Set FontSize 14
Set Width 1104
Set Height 330
Set Padding 16
Set WindowBar Colorful
Set WindowBarSize 32
Set Theme "OneHalfDark"
Set Framerate 10
Set PlaybackSpeed 3
Set WaitTimeout 600s

Hide
Type 'export MANGA_DOWNLOADER_DEMO_DELAY=1.8 && export PATH="$PWD:$PATH" && mkdir -p /tmp/manga-downloader-demo/groups && cd /tmp/manga-downloader-demo/groups && rm -rf ./* && clear'
Enter
Show

Type "manga-downloader https://mangadex.org/title/3f9b2c7e-5a41-4d86-9c0e-7b1d2a6e48f5 1-3"
Sleep 500ms
Enter
Wait
Sleep 2s

Hide
Type "rm -rf ./* && clear"
Enter
Show

Type 'manga-downloader https://mangadex.org/title/3f9b2c7e-5a41-4d86-9c0e-7b1d2a6e48f5 1-3 --scanlator "Moonlit Scans"'
Sleep 500ms
Enter
Wait
Sleep 3s
