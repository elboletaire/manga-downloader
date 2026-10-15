# Recreates assets/demos/bundle.gif — run from the repo root: vhs assets/demos/bundle.tape
# Requires the demo binary built at the repo root (make build/demo): it serves the
# series from demo/series, so the run needs no network (see demo/README.md).
# The old gif used MangaDex + --language es, but all Spanish One Piece chapters
# there are external stubs now (0 pages), so this uses inmanga like the README's
# bundling example, with a recent range (early chapters 404 on inmanga's CDN).
Output assets/demos/bundle.gif

Set Shell "bash"
Set FontSize 14
Set Width 1104
Set Height 210
Set Padding 16
Set WindowBar Colorful
Set WindowBarSize 32
Set Theme "OneHalfDark"
Set Framerate 10
Set PlaybackSpeed 3
Set WaitTimeout 600s

Hide
Type 'export MANGA_DOWNLOADER_DEMO_DELAY=1.55 && export PATH="$PWD:$PATH" && mkdir -p /tmp/manga-downloader-demo/bundle && cd /tmp/manga-downloader-demo/bundle && rm -rf ./* && clear'
Enter
Show

Type "manga-downloader https://inmanga.com/ver/manga/One-Piece/dfc7ecb5-e9b3-4aa5-a61b-a498993cd935 1140-1147 --bundle"
Sleep 500ms
Enter
Wait
Sleep 3s
