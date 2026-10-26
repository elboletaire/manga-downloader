# Recreates assets/demos/skip-existing-light.gif, the light twin of skip-existing.tape (keep the two in step)
# — run from the repo root: vhs assets/demos/skip-existing-light.tape
# Requires the demo binary built at the repo root (make build/demo): it serves the
# series from demo/series, so the run needs no network (see demo/README.md).
# Downloads five Horimiya chapters, then runs 1-8 again with --skip-existing: 1-5 are
# listed as skipped and only 6-8 are fetched. Filtered to English, so each chapter
# number has one release and one file name.
Output assets/demos/skip-existing-light.gif

Set Shell "bash"
Set FontSize 14
Set Width 1104
Set Height 270
Set Padding 16
Set WindowBar Colorful
Set WindowBarSize 32
Set Theme "OneHalfLight"
Set Framerate 10
Set PlaybackSpeed 3
Set WaitTimeout 600s

Hide
Type 'export MANGA_DOWNLOADER_DEMO_DELAY=1 && export PATH="$PWD:$PATH" && mkdir -p /tmp/manga-downloader-demo/skip-existing && cd /tmp/manga-downloader-demo/skip-existing && rm -rf ./* && clear'
Enter
Show

Type "manga-downloader https://mangadex.org/title/a25e46ec-30f7-4db6-89df-cacbc1d9a900 1-5 --language en"
Sleep 500ms
Enter
Wait
Sleep 2s

Hide
Type "clear"
Enter
Show

Type "manga-downloader --skip-existing https://mangadex.org/title/a25e46ec-30f7-4db6-89df-cacbc1d9a900 1-8 --language en"
Sleep 500ms
Enter
Wait
Sleep 3s
