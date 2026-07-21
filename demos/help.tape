# Recreates demos/help.gif — run from the repo root: vhs demos/help.tape
# Requires the manga-downloader binary built at the repo root (make build).
Output demos/help.gif

Set Shell "bash"
Set FontSize 14
Set Width 960
Set Height 700
Set Padding 16
Set WindowBar Colorful
Set WindowBarSize 32
Set Theme "OneHalfDark"
Set Framerate 10
Set WaitTimeout 60s

Hide
Type 'export PATH="$PWD:$PATH" && clear'
Enter
Show

Type "manga-downloader help"
Sleep 500ms
Enter
Wait
Sleep 5s
