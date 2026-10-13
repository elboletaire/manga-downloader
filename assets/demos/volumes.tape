# Recreates assets/demos/volumes.gif — run from the repo root: vhs assets/demos/volumes.tape
# Requires the manga-downloader binary built at the repo root (make build).
# Uses a series with well-defined volumes on the site, downloaded with --volumes auto.
Output assets/demos/volumes.gif

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
Type 'export PATH="$PWD:$PATH" && mkdir -p /tmp/manga-downloader-demo/volumes && cd /tmp/manga-downloader-demo/volumes && rm -rf ./* && clear'
Enter
Show

Type "manga-downloader https://mangadex.org/title/a96676e5-8ae2-425e-b9d9-4d5385acea48/wind-breaker 1-8 --volumes auto"
Sleep 500ms
Enter
Wait
Sleep 3s
