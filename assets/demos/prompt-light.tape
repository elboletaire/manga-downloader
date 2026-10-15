# Recreates assets/demos/prompt-light.gif, the light twin of prompt.tape (keep the two in step)
# — run from the repo root: vhs assets/demos/prompt-light.tape
# Requires the demo binary built at the repo root (make build/demo): it serves the
# series from demo/series, so the run needs no network (see demo/README.md).
# Uses a short (5 chapter) series so answering "y" downloads everything quickly.
# Filtered to a single language ("es-la", Latin American Spanish) because the
# series is translated into several languages nowadays and the unfiltered
# download would show 3 bars per chapter.
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

Type "manga-downloader https://mangadex.org/title/851f6d0a-49ae-4912-9939-2aeb865d13cd --language es-la"
Sleep 500ms
Enter
Wait+Screen@60s /y\/N/
Sleep 1s
Type "y"
Sleep 500ms
Enter
Wait
Sleep 3s
