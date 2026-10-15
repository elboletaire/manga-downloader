# Demo data

The VHS tapes in `assets/demos/` run the real commands, but against fake sites
instead of the live ones, so a run needs no network and comes out the same
every time. The code is only built in with the `demo` tag: `make build/demo`
writes `manga-downloader` at the repo root, which is where the tapes look for
it. Release builds don't include it. That binary never touches the network, so
rebuild with `make build/unix` before using it for real.

- `demo.go`: a fake grabber per series, registered ahead of the real ones
  (`grabber.RegisterEarly`). Each claims exactly the URLs its file lists, and
  serves the title, chapters and page lists from it. Any other URL falls through
  to the real grabbers, which then fail as there is no network.
- `series/*.json`: one file per series (`urls`, `title`, `chapters`, and
  `languages` when the series is translated into several, which turns on the
  `--language` filter). A chapter has `number`, `title`, `language`, `id`,
  `pages` (a count), and optionally `volume`, `groups` and `locked`.
  `nameFormat` is the title a fetched chapter gets, from its whole number and
  listed title, for the sites whose chapter page names it differently.
- `pages.go`: the page URLs live on `demo.invalid`, which a transport
  (`http.SetRoundTripper`) answers with one small placeholder JPEG, after a delay
  between 1.2s and 4.8s, fixed per URL. It refuses every other host.
  `MANGA_DOWNLOADER_DEMO_DELAY=<factor>` (a positive number, e.g. `0.4`)
  multiplies every delay, so a tape can shorten or stretch its downloads. Each
  tape sets it in its hidden setup line, so a tape's light and dark twins share it.

## Changing a tape

- A different range or chapters: edit the series' `chapters`; the file names
  come from the same title template as in a real run.
- A new series: add a `series/<name>.json` with its `urls` and chapters. No
  recording is needed, and the data can be made up. Use the URL as the tape types it.
- A new language: give the chapters that `language` and list it in `languages`.
- Volumes: set `volume` on the chapters, for `--volumes auto`.
- Scanlation groups: list a chapter once per release, each with its `groups` (`groups` tape).

Then `make build/demo`, run the tape's command, and check the archives with
`go run ./tools/verify-cbz` before re-recording the GIFs. Run the demo build's
tests with `make test/demo`.
