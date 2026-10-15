# Recreates assets/demos/download.gif — run from the repo root: vhs assets/demos/download.tape
# Requires the demo binary built at the repo root (make build/demo): it serves the
# series from demo/series, so the run needs no network (see demo/README.md).
# Note: inmanga prunes early One Piece chapters from its CDN, so the demo uses
# a recent range instead of the README's illustrative 1-50.
Output assets/demos/download.gif

Set Shell "bash"
Set FontSize 14
Set Width 1104
Set Height 270
Set Padding 16
Set WindowBar Colorful
Set WindowBarSize 32
Set Theme "OneHalfDark"
Set Framerate 10
Set PlaybackSpeed 3
Set WaitTimeout 600s

Hide
Type 'export MANGA_DOWNLOADER_DEMO_DELAY=1.2 && export PATH="$PWD:$PATH" && mkdir -p /tmp/manga-downloader-demo/download && cd /tmp/manga-downloader-demo/download && rm -rf ./* && clear'
Enter
Show

Type "manga-downloader 1140-1147 https://inmanga.com/ver/manga/One-Piece/dfc7ecb5-e9b3-4aa5-a61b-a498993cd935"
Sleep 500ms
Enter
Wait
Sleep 3s
