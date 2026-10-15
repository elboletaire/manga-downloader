# Recreates assets/demos/volumes-light.gif, the light twin of volumes.tape (keep the two in step)
# — run from the repo root: vhs assets/demos/volumes-light.tape
# Requires the demo binary built at the repo root (make build/demo): it serves the
# series from demo/series, so the run needs no network (see demo/README.md).
# Uses a series whose volumes MangaDex lists (1-13 is exactly volumes 1 and 2),
# downloaded with --volumes auto. Filtered to English, as several translations
# would otherwise all be packed into the same volume.
Output assets/demos/volumes-light.gif

Set Shell "bash"
Set FontSize 14
Set Width 1104
Set Height 210
Set Padding 16
Set WindowBar Colorful
Set WindowBarSize 32
Set Theme "OneHalfLight"
Set Framerate 10
Set PlaybackSpeed 3
Set WaitTimeout 600s

Hide
Type 'export MANGA_DOWNLOADER_DEMO_DELAY=1.25 && export PATH="$PWD:$PATH" && mkdir -p /tmp/manga-downloader-demo/volumes && cd /tmp/manga-downloader-demo/volumes && rm -rf ./* && clear'
Enter
Show

Type "manga-downloader https://mangadex.org/title/a25e46ec-30f7-4db6-89df-cacbc1d9a900 1-13 --volumes auto --language en"
Sleep 500ms
Enter
Wait
Sleep 3s
