<div align="center">

# Manga Downloader

**Download manga chapters from MangaDex and 80+ other sites, packed into CBZ
files ready to read on your favorite e-reader or reading app.**

[![Tests][tests badge]][tests]
[![Go Reference][go reference badge]][go reference]
[![GitHub release][release badge]][releases]
[![gitHub downloads]][releases]
[![Docker Pulls][pulls badge]][docker hub]
[![License][license badge]][license]

![prompt img]

</div>

## Quick start

Grab the binary for your system from the [releases section][releases], then
point it at a manga's index page and tell it which chapters you want:

~~~bash
manga-downloader https://mangadex.org/title/a1c7c817-4e59-43b7-9365-09675a149a6f/one-piece 1-10
~~~

That's it — you get `One Piece 0001 - Romance Dawn.cbz` and nine more files in
the current folder. Everything below is optional refinement.

## What you get

- **One self-contained binary.** No runtime, no dependencies, no config file.
- **[84 supported sites](#supported-sites)**, from big aggregators to small
  scanlation groups.
- **Chapter ranges** like `1,3,5-10` — download exactly what you're missing.
- **CBZ or plain folders** of images (`--format raw`).
- **E-reader friendly**: AVIF pages are converted to JPEG automatically, so
  chapters don't show up blank on a Kobo or in Calibre.
- **Bundling**: collapse a whole range into a single CBZ (`--bundle`).
- **Concurrent downloads**, tunable per chapter and per page.
- **Javascript and Cloudflare sites** handled through a local Chromium browser.

## Supported sites

Manga Downloader currently supports **84 sites**, from big aggregators like
MangaDex, MangaFire or MangaPark to individual scanlation groups:

<details>
<summary><b>Show the full list of supported sites</b></summary>
<br>

- [asmotoon.com (Asmodeus Scans)](https://asmotoon.com)
- [asurascans.com (Asura Scans, former asuratoon.com)](https://asurascans.com)
- [atsu.moe (Atsumaru)](https://atsu.moe)
- [aurorascans.com (redirects to qimanga.com)](https://aurorascans.com)
- [baozimh.com (包子漫画)](https://www.baozimh.com)
- [bigsolo.org](https://bigsolo.org)
- [bluesolo.org (Blue Solo, French scantrad)](https://bluesolo.org)
- [comix.to](https://comix.to) \*
- [danke.moe (Danke fürs Lesen)](https://danke.moe)
- [deathtollscans.net](https://reader.deathtollscans.net)
- [demonicscans.org (MangaDemon / Demonic Scans)](https://demonicscans.org)
- [dragontea.ink](https://dragontea.ink) \*
- [drakecomic.net (Drake Scans, former drakecomic.org)](https://drakecomic.net)
- [dynasty-scans.com (Dynasty Reader)](https://dynasty-scans.com)
- [elftoon.com](https://elftoon.com)
- [en-hijala.com (Hijala Translations)](https://en-hijala.com)
- [en-thunderscans.com (Thunderscans)](https://en-thunderscans.com)
- [fanfox.net (Manga Fox)](https://fanfox.net)
- [flamecomics.xyz (Flame Comics, former Flame Scans)](https://flamecomics.xyz)
- [fmteam.fr](https://fmteam.fr)
- [furyosociety.com](https://furyosociety.com)
- [gdscans.com (GalaxyDegenScans)](https://gdscans.com)
- [genzupdates.com (Genz Toon)](https://genzupdates.com)
- [guya.moe (Guya, Kaguya-sama)](https://guya.moe)
- [hivetoons.org (HiveToons, VoidScans)](https://hivetoons.org)
- [inmanga.com](https://inmanga.com)
- [jestful.net](https://jestful.net)
- [kappabeast.com](https://kappabeast.com) \*
- [kaynscans.com (Kayn Scans, former kaynscan.org)](https://kaynscans.com)
- [lagoonscans.com](https://lagoonscans.com)
- [leercapitulo.co](https://www.leercapitulo.co) \*
- [lhtranslation.net (LHTranslation)](https://lhtranslation.net)
- [luacomic.org (LuaScans)](https://luacomic.org)
- [madarascans.org (former madarascans.com)](https://madarascans.org)
- [mangaball.net](https://mangaball.net)
- [mangabats.com (former mangabat.com)](https://www.mangabats.com)
- [mangadenizi.net](https://www.mangadenizi.net)
- [mangadex.org (MangaDex)](https://mangadex.org)
- [mangafire.to](https://mangafire.to) \*
- [mangahere.cc (MangaHere)](https://www.mangahere.cc)
- [mangahub.io](https://mangahub.io) \*
- [mangak.io (MangaK, former mangabuddy.com)](https://mangak.io)
- [mangakakalot.gg (MangaKakalot)](https://www.mangakakalot.gg) \*
- [mangakatana.com](https://mangakatana.com)
- [mangalib.me (MangaLib)](https://mangalib.me)
- [mangalivre.to (Manga Livre, former mangalivre.tv/mangalivre.net)](https://mangalivre.to)
- [mangapark.page (MangaPark, the only live host: mangapark.to and the other mirrors are dead)](https://mangapark.page) \*
- [mangapill.com](https://mangapill.com)
- [mangaread.org](https://www.mangaread.org)
- [mangasushi.org](https://mangasushi.org)
- [mangataro.org](https://mangataro.org)
- [mangtto.com (Mangitto)](https://mangtto.com)
- [manhuaplus.com](https://manhuaplus.com)
- [manhuatop.org](https://manhuatop.org) \*
- [manhuaus.com](https://manhuaus.com) \*
- [mgeko.cc](https://www.mgeko.cc)
- [mistscans.com](https://mistscans.com)
- [mkissa.to](https://mkissa.to) \*
- [natomanga.com (MangaNato, former manganato.com/manganelo.com)](https://www.natomanga.com) \*
- [projectsuki.com](https://projectsuki.com)
- [qimanga.com](https://qimanga.com)
- [rawkuma.net](https://rawkuma.net)
- [ritharscans.com](https://ritharscans.com)
- [rokaricomics.com](https://rokaricomics.com)
- [roliascan.com](https://roliascan.com)
- [sacachispa.site](https://sacachispa.site)
- [sanascans.com (Sana Scans)](https://sanascans.com)
- [setsuscans.com](https://setsuscans.com) \*
- [silentquill.net (Armageddon Scanlation)](https://www.silentquill.net)
- [stonescape.xyz](https://stonescape.xyz)
- [sushiscan.net](https://sushiscan.net) \*
- [taiyo.moe](https://taiyo.moe)
- [tcbonepiecechapters.com (TCB Scans, former tcbscans.com)](https://tcbonepiecechapters.com)
- [team-shadowi.com](https://www.team-shadowi.com)
- [templetoons.com (Temple Scan)](https://templetoons.com)
- [toongod.org](https://www.toongod.org) \*
- [toonily.com](https://toonily.com) \*
- [tritinia.org (Tritinia Scans)](https://tritinia.org)
- [utoon.us (UToon, home of reset-scans' content)](https://www.utoon.us)
- [violetscans.org](https://violetscans.org)
- [vortexscans.org](https://vortexscans.org)
- [weebcentral.com](https://weebcentral.com)
- [witchtoons.net (WitchToons, former witchscans.com)](https://witchtoons.net)
- [writerscans.com](https://writerscans.com)
- [zonatmo.org (TuMangaOnline, former zonatmo.com)](https://zonatmo.org)

</details>

> [!NOTE]
> **Sites marked with a `*` need a Chromium-based browser installed** (Google
> Chrome, Chromium, Brave or Edge): they render with javascript or sit behind a
> Cloudflare challenge, so plain HTTP requests can't scrape them.
> manga-downloader launches the browser itself, and a window pops up on its own
> when a challenge needs to resolve — you may occasionally have to solve one
> click. Add `--browser-visible` to open it from the start and skip the
> (pointless, for these sites) headless attempt.

Other sites may work too, even if they're not listed. If you find one that
doesn't, feel free to [open an issue][issues] or a PR with the implementation.

## Installation

<details open>
<summary><b>Manual download</b> (any platform)</summary>
<br>

Download the archive for your system from the [releases section][releases] and
extract it. You can then run the binary from that folder:

~~~bash
./manga-downloader
~~~

Or on Windows:

~~~cmd
.\manga-downloader.exe
~~~

To run it from anywhere, place the binary in a folder that is in your `PATH`
(or add the folder where you extracted it to your `PATH` environment
variable). Common choices are `/usr/local/bin` on Linux and macOS, and
`C:\Windows\System32` on Windows:

~~~cmd
C:\Users\elboletaire\Desktop>manga-downloader https://mangadex.org/title/e7eabe96-aa17-476f-b431-2497d5e9d060/black-clover 1-346
~~~

The above command downloads Black Clover chapters 1 to 346 to the Desktop
folder (since that's the current directory).

</details>

<details>
<summary><b>macOS</b>: allowing an unsigned binary</summary>
<br>

Since the binary is not signed, macOS's Gatekeeper will block it the first time
you try to run it. On recent macOS versions the old terminal workarounds (like
`spctl --master-disable`) no longer work, so you have to allow it manually:

1. Run `./manga-downloader` once. macOS will show a warning saying the binary
   could not be verified and block it.
2. Open **System Settings** → **Privacy & Security**, scroll down to the
   **Security** section, and you'll see a message about *manga-downloader*
   being blocked. Click **Open Anyway** (or **Allow Anyway**).
3. Run `./manga-downloader` again and confirm in the dialog that pops up.

You only need to do this once; subsequent runs will work normally.

</details>

<details>
<summary><b>Go</b>: install from source</summary>
<br>

If you have Go installed, you can build and install the latest version in one
step:

~~~bash
go install github.com/elboletaire/manga-downloader@latest
~~~

Note the resulting binary won't report its version, since that's injected at
build time in the release builds.

</details>

<details>
<summary><b>Docker</b>: run without installing anything</summary>
<br>

~~~bash
docker run --rm -it -v $PWD:/downloads elboletaire/manga-downloader --help
~~~

Note the `-v $PWD:/downloads` param: it's required in order to get the
downloaded files in your current folder.

The container writes files as the UID/GID given by the `USER_ID` and
`GROUP_ID` environment variables (both default to `1000`). The mounted
directory must be writable by that user, which is already the case for
`-v $PWD:/downloads` when your host UID is `1000`. If your host UID differs,
pass it explicitly:

~~~bash
docker run --rm -it -e USER_ID=$(id -u) -e GROUP_ID=$(id -g) -v "$PWD:/downloads" elboletaire/manga-downloader --help
~~~

The default image ships no browser, so the sites marked with `*` in the
supported sites list (those needing a Chromium-based browser) won't work with
it. For those, use the `:browser` variant, which bundles Chromium and a
virtual display:

~~~bash
docker run --rm -it -v $PWD:/downloads elboletaire/manga-downloader:browser [URL] [chapters]
~~~

Cloudflare-protected sites work out of the box: the virtual display lets the
app escalate to a headed browser, and "Verify you are human" checkboxes are
clicked automatically. If a site still doesn't pass (e.g. a challenge needing
real human interaction), you can put the browser window on your own display,
which on a Linux desktop can be forwarded with (you may need
`xhost +local:` first):

~~~bash
docker run --rm -it -e DISPLAY=$DISPLAY -v /tmp/.X11-unix:/tmp/.X11-unix \
    -v $PWD:/downloads elboletaire/manga-downloader:browser --browser-visible [URL] [chapters]
~~~

</details>

## Usage

Only one argument is required: the URL of the manga's **index page** (the page
listing all its chapters, not an individual chapter).

~~~bash
manga-downloader [URL] [chapters]
~~~

Chapters are single numbers and/or ranges separated by commas — `1,3,5-10`:

~~~bash
manga-downloader https://inmanga.com/ver/manga/One-Piece/dfc7ecb5-e9b3-4aa5-a61b-a498993cd935 1-50
# downloads One Piece chapters 1 to 50 into the current folder
~~~

![download img]

Two things worth knowing up front:

- **Arguments are not positional**, so `manga-downloader 1-50 [URL]` does
  exactly the same as the example above.
- **Omit the chapters** and it asks whether to download all of them. You must
  answer <kbd>y</kbd>; it defaults to "no".

### Choosing a language

Some sites, like MangaDex, return the same chapter once per translated
language. By default every match is downloaded to its own file; `--language`
restricts it to one:

~~~bash
manga-downloader --language es https://mangadex.org/title/a1c7c817-4e59-43b7-9365-09675a149a6f/one-piece 1-10
# downloads One Piece chapters 1 to 10 in Spanish
~~~

### Choosing a scanlation group

Some sites, like Atsumaru, host several scanlation groups' versions of the same
chapters. Since only one version can be downloaded per chapter number, the group
with the most chapters is picked by default, and the ones available are listed
so you know a choice was made. `--scanlator` overrides it:

~~~bash
manga-downloader --scanlator alpha https://atsu.moe/manga/exqmE 1-10
# downloads chapters 1 to 10 as translated by Alpha, rather than the default pick
~~~

The name is matched case-insensitively. Use `--scanlator all` to download every
group's version instead of choosing one; the group name is then appended to each
chapter title, so the files don't overwrite each other.

### Bundling

`--bundle` merges all the downloaded chapters into a single CBZ file:

~~~bash
manga-downloader https://inmanga.com/ver/manga/One-Piece/dfc7ecb5-e9b3-4aa5-a61b-a498993cd935 1-8 --bundle
# downloads One Piece chapters 1 to 8 into a single file
~~~

Inside the bundle, each chapter gets its own folder (e.g. `Chapter 0001/`,
`Chapter 0002/`) so chapter boundaries and page numbering are preserved.

![bundle img]

### Output format

Chapters are packed into CBZ files by default. `--format raw` writes the images
into a plain folder instead, named the same as the CBZ would have been:

~~~bash
manga-downloader https://inmanga.com/ver/manga/One-Piece/dfc7ecb5-e9b3-4aa5-a61b-a498993cd935 1-8 --format raw
# downloads One Piece chapters 1 to 8, each into its own folder of images
~~~

### E-reader compatible images

Some sites serve their pages as AVIF, a format no dedicated e-reader can
display: Calibre/KCC, the Kobo CBZ reader and KOReader all fail on it, so the
resulting file looks empty on the device even though the download went fine.
**Those pages are converted to JPEG while packing, by default.**

WebP (served by a few other sites) renders fine in phone apps but is unreliable
on e-ink, so it's left alone unless you ask for it:

~~~bash
# also convert webp pages, for a strictly e-ink reading setup
manga-downloader --convert-images avif,webp <url> 1-10

# keep every page exactly as the site served it
manga-downloader --convert-images none <url> 1-10
~~~

JPEG, PNG and GIF pages are never touched: they're readable everywhere already,
and re-encoding them would only lose quality. A page that fails to convert is
kept as-is with a warning, rather than failing the whole chapter.

> [!NOTE]
> The AVIF decoder is embedded as WebAssembly, so no system library is needed on
> any platform. On the 32-bit `linux/386` and `windows/386` builds it runs
> interpreted and is noticeably slower — `--convert-images none` skips it
> entirely if that bothers you.

### Custom file names

File names are built from a [Go text/template][go template] string passed to
`--filename-template`. The default is:

~~~
{{.Series}} {{.Number}} - {{.Title}}{{if gt .Version 1}} v{{.Version}}{{end}}
~~~

The available variables are `{{.Series}}`, `{{.Number}}`, `{{.Title}}` and
`{{.Version}}` (a counter appended when a file name would be duplicated).

### All options

| Flag                  | Short | Description                                        | Default        |
| --------------------- | ----- | -------------------------------------------------- | -------------- |
| `--bundle`            | `-b`  | Bundle all specified chapters into a single file   | off            |
| `--language`          | `-l`  | Only download the specified language               | all languages  |
| `--scanlator`         | `-s`  | Only download the specified scanlation group       | most chapters  |
| `--output-dir`        | `-o`  | Where to write the downloaded files                | current folder |
| `--filename-template` | `-t`  | Template for the resulting file names              | see above      |
| `--format`            | `-f`  | Output format: `cbz` or `raw` (a plain folder)     | `cbz`          |
| `--convert-images`    |       | Formats to convert to JPEG: `avif`, `webp`, `none` | `avif`         |
| `--concurrency`       | `-c`  | Concurrent chapter downloads (max 5)               | 5              |
| `--concurrency-pages` | `-C`  | Concurrent page downloads per chapter (max 10)     | 10             |
| `--browser-visible`   |       | Open the browser window from the start             | off            |
| `--retry`             | `-r`  | Retries per failed page (max 3, 0 disables)        | 1              |

Run the `help` command to see them all from your terminal:

~~~bash
manga-downloader help
~~~

![help img]

## Star history

<div align="center">

[![Star history chart][star history]][stargazers]

</div>

## License

All the code contained in this repo is licensed under the
[GNU Affero General Public License v3.0][license].

<details>
<summary>License notice</summary>
<br>

~~~text
Manga Downloader GO cli
Copyright (C) 2023-2026 Òscar Casajuana Alonso

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
~~~

</details>

[github downloads]: https://img.shields.io/github/downloads/elboletaire/manga-downloader/total
[go reference badge]: https://pkg.go.dev/badge/github.com/elboletaire/manga-downloader.svg
[release badge]: https://img.shields.io/github/release/elboletaire/manga-downloader.svg
[pulls badge]: https://img.shields.io/docker/pulls/elboletaire/manga-downloader
[license badge]: https://img.shields.io/github/license/elboletaire/manga-downloader?color=green
[tests badge]: https://github.com/elboletaire/manga-downloader/actions/workflows/test.yaml/badge.svg?branch=master
[tests]: https://github.com/elboletaire/manga-downloader/actions/workflows/test.yaml
[go reference]: https://pkg.go.dev/github.com/elboletaire/manga-downloader

[license]: ./LICENSE
[releases]: https://github.com/elboletaire/manga-downloader/releases
[issues]: https://github.com/elboletaire/manga-downloader/issues
[stargazers]: https://github.com/elboletaire/manga-downloader/stargazers
[star history]: https://raw.githubusercontent.com/elboletaire/manga-downloader/star-tracker-data/charts/star-history.svg
[go template]: https://pkg.go.dev/text/template
[download img]: https://raw.githubusercontent.com/elboletaire/manga-downloader/master/demos/download.gif
[bundle img]: https://raw.githubusercontent.com/elboletaire/manga-downloader/master/demos/bundle.gif
[help img]: https://raw.githubusercontent.com/elboletaire/manga-downloader/master/demos/help.gif
[prompt img]: https://raw.githubusercontent.com/elboletaire/manga-downloader/master/demos/prompt.gif
[docker hub]: https://hub.docker.com/r/elboletaire/manga-downloader
