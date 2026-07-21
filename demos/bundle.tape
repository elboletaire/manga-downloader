# Recreates demos/bundle.gif — run from the repo root: vhs demos/bundle.tape
# Requires the manga-downloader binary built at the repo root (make build).
# The old gif used MangaDex + --language es, but all Spanish One Piece chapters
# there are external stubs now (0 pages), so this uses inmanga like the README's
# bundling example, with a recent range (early chapters 404 on inmanga's CDN).
Output demos/bundle.gif

Set Shell "bash"
Set FontSize 14
Set Width 960
Set Height 420
Set Padding 16
Set WindowBar Colorful
Set WindowBarSize 32
Set Theme "OneHalfDark"
Set Framerate 10
Set PlaybackSpeed 3
Set WaitTimeout 600s

Hide
Type 'export PATH="$PWD:$PATH" && mkdir -p /tmp/manga-downloader-demo/bundle && cd /tmp/manga-downloader-demo/bundle && rm -rf ./* && clear'
Enter
Show

Type "manga-downloader https://inmanga.com/ver/manga/One-Piece/dfc7ecb5-e9b3-4aa5-a61b-a498993cd935 1140-1147 --bundle"
Sleep 500ms
Enter
Wait
Sleep 3s
