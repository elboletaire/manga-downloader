Manga Downloader [![starline]](#star-history-)
=============================================

[![Go Report Card][go report card]][go report]
[![Go Reference][go reference badge]][go reference]
[![GitHub release][release badge]][releases]
[![gitHub downloads]][downloads]
[![Docker Pulls][pulls badge]][docker hub]
[![License][license badge]][license]

Download manga chapters from websites like MangaDex and pack them into CBZ
files, ready to read on your favorite ereader or reading app.

![prompt img]

Supported sites
---------------

- [asmotoon.com (Asmodeus Scans)](https://asmotoon.com)
- [asurascans.com (Asura Scans, former asuratoon.com)](https://asurascans.com)
- [demonicscans.org (MangaDemon / Demonic Scans)](https://demonicscans.org)
- [atsu.moe (Atsumaru)](https://atsu.moe)
- [aurorascans.com (redirects to qimanga.com)](https://aurorascans.com)
- [danke.moe (Danke fürs Lesen)](https://danke.moe)
- [bigsolo.org](https://bigsolo.org)
- [deathtollscans.net](https://reader.deathtollscans.net)
- [bluesolo.org (Blue Solo, French scantrad)](https://bluesolo.org)
- [dragontea.ink](https://dragontea.ink) \*
- [dynasty-scans.com (Dynasty Reader)](https://dynasty-scans.com)
- [fanfox.net (Manga Fox)](https://fanfox.net)
- [elftoon.com](https://elftoon.com)
- [en-hijala.com (Hijala Translations)](https://en-hijala.com)
- [flamecomics.xyz (Flame Comics, former Flame Scans)](https://flamecomics.xyz)
- [guya.moe (Guya, Kaguya-sama)](https://guya.moe)
- [hivetoons.org (HiveToons, VoidScans)](https://hivetoons.org)
- [furyosociety.com](https://furyosociety.com)
- [gdscans.com (GalaxyDegenScans)](https://gdscans.com)
- [fmteam.fr](https://fmteam.fr)
- [genzupdates.com (Genz Toon)](https://genzupdates.com)
- [inmanga.com](https://inmanga.com)
- [jestful.net](https://jestful.net)
- [kappabeast.com](https://kappabeast.com) \*
- [lagoonscans.com](https://lagoonscans.com)
- [kaynscan.org (Kayn Scan)](https://kaynscan.org)
- [leercapitulo.co](https://www.leercapitulo.co) \*
- [LHTranslation](https://lhtranslation.net)
- [madarascans.org (former madarascans.com)](https://madarascans.org)
- [luacomic.org (LuaScans)](https://luacomic.org)
- [mangaball.net](https://mangaball.net)
- [mangabats.com (former mangabat.com)](https://www.mangabats.com)
- [mangadenizi.net](https://www.mangadenizi.net)
- [mangafire.to](https://mangafire.to)
- [mangahere.cc (MangaHere)](https://www.mangahere.cc)
- [mangak.io (MangaK, former mangabuddy.com)](https://mangak.io)
- [mangakakalot.gg (MangaKakalot)](https://www.mangakakalot.gg) \*
- [mangakatana.com](https://mangakatana.com)
- [mgeko.cc](https://www.mgeko.cc)
- [mangapark.page (MangaPark, mangapark.to's mirror at the time of writing)](https://mangapark.page)
- [mangalivre.to (Manga Livre, former mangalivre.tv/mangalivre.net)](https://mangalivre.to)
- [mangalib.me (MangaLib)](https://mangalib.me)
- [Mangadex](https://mangadex.org)
- [mangapill.com](https://mangapill.com)
- [mangaread.org](https://www.mangaread.org)
- [manhuaplus.com](https://manhuaplus.com)
- [mangasushi.org](https://mangasushi.org)
- [mangataro.org](https://mangataro.org)
- [Mangitto (mangtto.com)](https://mangtto.com)
- [manhuaus.com](https://manhuaus.com) \*
- [natomanga.com (MangaNato, former manganato.com/manganelo.com)](https://www.natomanga.com) \*
- [projectsuki.com](https://projectsuki.com)
- [qimanga.com](https://qimanga.com)
- [rawkuma.net](https://rawkuma.net)
- [silentquill.net (Armageddon Scanlation)](https://www.silentquill.net)
- [rokaricomics.com](https://rokaricomics.com)
- [ritharscans.com](https://ritharscans.com)
- [roliascan.com](https://roliascan.com)
- [sanascans.com (Sana Scans)](https://sanascans.com)
- [stonescape.xyz](https://stonescape.xyz)
- [sushiscan.net](https://sushiscan.net) \*
- [taiyo.moe](https://taiyo.moe)
- [tcbonepiecechapters.com (TCB Scans, former tcbscans.com)](https://tcbonepiecechapters.com)
- [templetoons.com (Temple Scan)](https://templetoons.com)
- [team-shadowi.com](https://www.team-shadowi.com)
- [en-thunderscans.com (Thunderscans)](https://en-thunderscans.com)
- [toongod.org](https://www.toongod.org) \*
- [vortexscans.org](https://vortexscans.org)
- [tritinia.org (Tritinia Scans)](https://tritinia.org)
- [violetscans.org](https://violetscans.org)
- [weebcentral.com](https://weebcentral.com)
- [zonatmo.org (TuMangaOnline, former zonatmo.com)](https://zonatmo.org)

> \* These sites can't be scraped with plain HTTP requests (they render with
> javascript or sit behind a Cloudflare challenge), so they need a Chromium-based
> browser installed (Google Chrome, Chromium, Brave or Edge), which
> manga-downloader launches automatically to render the pages. Most sit behind a
> Cloudflare challenge that only passes in a visible browser — this happens on
> its own: if a headless attempt hits a challenge, a browser window opens
> automatically so it can resolve (you may occasionally need to solve one click):
>
> ~~~bash
> manga-downloader https://www.toongod.org/webtoon/solo-leveling/ 1-10
> ~~~
>
> You can pass `--browser-visible` to open the window from the start and skip the
> (pointless, for these sites) headless attempt.

It may support even more sites of which I'm not aware. If you find a site that
is not supported, feel free to open a new issue or a PR with the implementation.

Installation
------------

Download the archive for your system from the [releases section][releases] and
extract it. You can then run the binary from that folder:

~~~bash
./manga-downloader
~~~

Or on Windows:

~~~cmd
.\manga-downloader.exe
~~~

To be able to run it from anywhere, place the binary in a folder that is in
your `PATH` (or add the folder where you extracted it to your `PATH`
environment variable). Common choices:

- Linux and macOS: `/usr/local/bin`
- Windows: `C:\Windows\System32`

### macOS

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

### Windows

If you place the `.exe` file inside `C:\Windows\System32` you'll be able to
call the program from anywhere:

~~~cmd
C:\Users\elboletaire\Desktop>manga-downloader https://mangadex.org/title/e7eabe96-aa17-476f-b431-2497d5e9d060/black-clover 1-346
~~~

The above command downloads Black Clover chapters 1 to 346 to the Desktop
folder (since that's the current directory).

### Docker

You can also run manga-downloader directly via Docker, without installing
anything:

~~~bash
docker run --rm -it -v $PWD:/downloads elboletaire/manga-downloader --help
~~~

Note the `-v $PWD:/downloads` param: it's required in order to get the
downloaded files in your current folder.

Usage
-----

Only one argument is required: the URL of the manga's index page (the page
listing all its chapters, not an individual chapter).

~~~bash
manga-downloader [URL]
~~~

When you only specify the URL, it will ask you whether you want to download all
chapters.

> Note: you must answer <kbd>y</kbd> to download them; it defaults to "no".

You can also specify which chapters to download as a second argument, using
single numbers and/or ranges separated by commas (e.g. `1,3,5-10`):

~~~bash
manga-downloader https://inmanga.com/ver/manga/One-Piece/dfc7ecb5-e9b3-4aa5-a61b-a498993cd935 1-50
# downloads One Piece chapters 1 to 50 into the current folder
~~~

![download img]

Arguments are not positional, so you can pass them in any order:

~~~bash
manga-downloader 1-50 https://inmanga.com/ver/manga/One-Piece/dfc7ecb5-e9b3-4aa5-a61b-a498993cd935
# exactly the same as the previous example
~~~

### Choosing a language

Some sites, like MangaDex, can return the same chapter multiple times, once per
translated language. By default every match is downloaded to its own file, but
you can restrict the download to a single language with `--language`:

~~~bash
manga-downloader --language es https://mangadex.org/title/a1c7c817-4e59-43b7-9365-09675a149a6f/one-piece 1-10
# downloads One Piece chapters 1 to 10 in Spanish
~~~

### Bundling

Use `--bundle` to bundle all the downloaded chapters into a single CBZ file:

~~~bash
manga-downloader https://inmanga.com/ver/manga/One-Piece/dfc7ecb5-e9b3-4aa5-a61b-a498993cd935 1-8 --bundle
# downloads One Piece chapters 1 to 8 into a single file
~~~

Inside the bundle, each chapter gets its own folder (e.g. `Chapter 0001/`,
`Chapter 0002/`) so chapter boundaries and page numbering are preserved.

![bundle img]

### Output format

By default chapters are packed into CBZ files. Pass `--format raw` to write
the images into a plain folder instead (named the same as the CBZ file would
have been, minus the extension):

~~~bash
manga-downloader https://inmanga.com/ver/manga/One-Piece/dfc7ecb5-e9b3-4aa5-a61b-a498993cd935 1-8 --format raw
# downloads One Piece chapters 1 to 8, each into its own folder of images
~~~

### Custom file names

Resulting file names can be customized with `--filename-template`, which takes
a [Go text/template](https://pkg.go.dev/text/template) string. The available
variables are `{{.Series}}`, `{{.Number}}`, `{{.Title}}` and `{{.Version}}`
(a counter appended when a file name would be duplicated). The default is:

~~~
{{.Series}} {{.Number}} - {{.Title}}{{if gt .Version 1}} v{{.Version}}{{end}}
~~~

### All options

| Flag                  | Short | Description                                              | Default            |
| --------------------- | ----- | -------------------------------------------------------- | ------------------ |
| `--bundle`            | `-b`  | Bundle all specified chapters into a single file         | off                |
| `--language`          | `-l`  | Only download the specified language                     | all languages      |
| `--output-dir`        | `-o`  | Output directory for the downloaded files                | current folder     |
| `--filename-template` | `-t`  | Template for the resulting file names                    | see above          |
| `--format`            | `-f`  | Output format: `cbz` or `raw` (a folder with the images) | `cbz`              |
| `--concurrency`       | `-c`  | Concurrent chapter downloads (max 5)                     | 5                  |
| `--concurrency-pages` | `-C`  | Concurrent page downloads per chapter (max 10)           | 10                 |
| `--browser-visible`   |       | Open the browser from the start (opens automatically on a challenge anyway) | off        |
| `--retry`             | `-r`  | Retries for failed page downloads (max 3, 0 disables retrying)  | 1                  |

Run the `help` command to see them all from your terminal:

~~~bash
manga-downloader help
~~~

![help img]

Star history ![starline]
------------------------

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/chart?repos=elboletaire/manga-downloader&type=date&theme=dark&legend=top-left&sealed_token=GfQgqGBmkGzpKUy8jK1yIy09naBjNE6V3-CyvUqBPhqXKzB7UJU5obN0nHeR3POMpBCyi53z30dH_CyEu2bK1rlzc81eJBJwUJXMYK-Mn0CjkcOjwGs9ThDonsRCaSlhQCXsWGWtuYEeFKFUYX5O43yTbhNYclOsolN8OuQ5pl_d9zsKjNzkcnSnO6D8" />
  <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/chart?repos=elboletaire/manga-downloader&type=date&legend=top-left&sealed_token=GfQgqGBmkGzpKUy8jK1yIy09naBjNE6V3-CyvUqBPhqXKzB7UJU5obN0nHeR3POMpBCyi53z30dH_CyEu2bK1rlzc81eJBJwUJXMYK-Mn0CjkcOjwGs9ThDonsRCaSlhQCXsWGWtuYEeFKFUYX5O43yTbhNYclOsolN8OuQ5pl_d9zsKjNzkcnSnO6D8" />
  <img alt="Star History Chart" src="https://api.star-history.com/chart?repos=elboletaire/manga-downloader&type=date&legend=top-left&sealed_token=GfQgqGBmkGzpKUy8jK1yIy09naBjNE6V3-CyvUqBPhqXKzB7UJU5obN0nHeR3POMpBCyi53z30dH_CyEu2bK1rlzc81eJBJwUJXMYK-Mn0CjkcOjwGs9ThDonsRCaSlhQCXsWGWtuYEeFKFUYX5O43yTbhNYclOsolN8OuQ5pl_d9zsKjNzkcnSnO6D8" />
</picture>

License
-------

All the code contained in this repo is licensed under the
[GNU Affero General Public License v3.0][license]

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

[github downloads]: https://img.shields.io/github/downloads/elboletaire/manga-downloader/total
[go reference badge]: https://pkg.go.dev/badge/github.com/elboletaire/manga-downloader.svg
[release badge]: https://img.shields.io/github/release/elboletaire/manga-downloader.svg
[pulls badge]: https://img.shields.io/docker/pulls/elboletaire/manga-downloader
[license badge]: https://img.shields.io/github/license/elboletaire/manga-downloader?color=green
[go report]: https://goreportcard.com/report/github.com/elboletaire/manga-downloader
[go report card]: https://goreportcard.com/badge/github.com/elboletaire/manga-downloader
[go reference]: https://pkg.go.dev/github.com/elboletaire/manga-downloader
[starline]: https://starlines.qoo.monster/assets/elboletaire/manga-downloader

[license]: ./LICENSE
[releases]: https://github.com/elboletaire/manga-downloader/releases
[issues]: https://github.com/elboletaire/manga-downloader/issues
[download img]: https://raw.githubusercontent.com/elboletaire/manga-downloader/master/demos/download.gif
[bundle img]: https://raw.githubusercontent.com/elboletaire/manga-downloader/master/demos/bundle.gif
[help img]: https://raw.githubusercontent.com/elboletaire/manga-downloader/master/demos/help.gif
[prompt img]: https://raw.githubusercontent.com/elboletaire/manga-downloader/master/demos/prompt.gif
[docker hub]: https://hub.docker.com/r/elboletaire/manga-downloader
[downloads]: https://qii404.me/github-release-statistics/?repo=https://github.com/elboletaire/manga-downloader
