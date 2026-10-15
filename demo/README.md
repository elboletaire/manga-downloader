# Demo fixtures

The VHS tapes in `assets/demos/` run the real commands, but against recorded
responses instead of the live sites, so a run needs no network and comes out
the same every time. The fixtures are only built in with the `demo` tag:
`make build/demo` writes `manga-downloader` at the repo root, which is where the
tapes look for it. Release builds don't include them. That binary never touches
the network, so rebuild with `make build/unix` before using it for real.

- `transport.go`: answers each request from `fixtures/manifest.json` (keyed by
  method and URL) and fails with the URL when there's no entry. Page images are
  a placeholder JPEG, each after a delay between 1.2s and 4.8s, fixed per URL.
  `MANGA_DOWNLOADER_DEMO_DELAY=<factor>` (a positive number, e.g. `0.4`)
  multiplies every page delay, so a tape can shorten or stretch its downloads.
  Each tape sets it in its hidden setup line, so a tape's light and dark twins
  share it.
- `record.go`: with `MANGA_DOWNLOADER_DEMO_RECORD=<dir>` set, goes to the live
  sites and writes the same manifest and bodies into `<dir>`. Images are
  recorded as placeholders, so no site's pages are saved.
- `trim.sh`: cuts a recording down to the chapters the tapes request
  (`sh demo/trim.sh <recording dir> demo/fixtures`, needs `jq` and `perl`).
- `fixtures/`: the trimmed manifest and bodies. Pages aren't stored at all.

## Re-recording

1. `make build/demo`, then run each tape's command with recording on, from an
   empty directory. The commands are the `Type` lines of the tapes:

   ```sh
   MANGA_DOWNLOADER_DEMO_RECORD=/tmp/rec /path/to/manga-downloader \
     https://inmanga.com/ver/manga/One-Piece/dfc7ecb5-e9b3-4aa5-a61b-a498993cd935 1140-1147
   ```

   The prompt tape needs `y` on stdin. The recording merges into `<dir>`, so
   every command can share one directory.
2. `sh demo/trim.sh /tmp/rec demo/fixtures`. Its rules are written for the
   tapes' ranges: when a tape's range or language changes, change the matching
   rule there too.
3. `make build/demo` and run the tapes; compare the archives with
   `go run ./tools/verify-cbz` before re-recording the GIFs.

Run the transport's tests with `make test/demo`.
