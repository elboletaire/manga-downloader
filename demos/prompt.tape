# Recreates demos/prompt.gif — run from the repo root: vhs demos/prompt.tape
# Requires the manga-downloader binary built at the repo root (make build).
# Uses a short (5 chapter) series so answering "y" downloads everything quickly.
# Filtered to a single language ("es-la", Latin American Spanish) because the
# series is translated into several languages nowadays and the unfiltered
# download would show 3 bars per chapter.
Output demos/prompt.gif

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
Type 'export PATH="$PWD:$PATH" && mkdir -p /tmp/manga-downloader-demo/prompt && cd /tmp/manga-downloader-demo/prompt && rm -rf ./* && clear'
Enter
Show

Type "manga-downloader https://mangadex.org/title/851f6d0a-49ae-4912-9939-2aeb865d13cd/i-m-so-cute-i-might-explode --language es-la"
Sleep 500ms
Enter
Wait+Screen@60s /y\/N/
Sleep 1s
Type "y"
Sleep 500ms
Enter
Wait
Sleep 3s
