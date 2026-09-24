<div align="center">

# Manga Downloader

[![Tests][tests badge]][tests]
[![Go Reference][go reference badge]][go reference]
[![GitHub release][release badge]][releases]
[![GitHub downloads][downloads badge]][releases]
[![Docker Pulls][pulls badge]][docker hub]
[![License][license badge]][license]

⭐ If you like this project, star it on GitHub!

[Overview](#overview) • [Features](#features) • [Supported sites](#supported-sites) • [Installation](#installation) • [Usage](#usage) • [Troubleshooting](#troubleshooting)

![prompt img]

</div>

## Overview

Manga Downloader is a command line tool that downloads manga chapters from
MangaDex and 80+ other sites and packs them into CBZ files. The files are ready
to read on your e-reader or in your favorite comic app.

Point it at a manga's index page, tell it which chapters you want, and it does
the rest:

~~~bash
manga-downloader https://mangadex.org/title/a1c7c817-4e59-43b7-9365-09675a149a6f/one-piece 1-10
~~~

This writes `One Piece 0001 - Romance Dawn.cbz` and nine more files to the
current folder.

## Features

- **A single binary**: no runtime, no dependencies and no config file. Builds
  are available for Linux, macOS and Windows, plus Docker images.
- **[84 supported sites](#supported-sites)**, from big aggregators to small
  scanlation groups.
- **Chapter ranges** such as `1,3,5-10`, so you download only what's missing.
- **E-reader friendly output**: AVIF pages are converted to JPEG automatically,
  so they don't show up blank on Kobo, KOReader or Calibre.
- **Flexible output**: one CBZ per chapter, a single bundled CBZ (`--bundle`),
  or plain image folders (`--format raw`), named from a template.
- **Language and scanlation group filters** for sites that host several versions
  of a chapter.
- **Concurrent downloads**, tunable per chapter and per page, with retries.
- **JavaScript and Cloudflare-protected sites** are handled by driving a local
  Chromium browser, which clicks "Verify you are human" checkboxes for you.

## Supported sites

<details>
<summary><b>Show all 84 supported sites</b></summary>
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
> **Sites marked with `*` need a Chromium-based browser** (Google Chrome,
> Chromium, Brave or Edge). They either render with JavaScript or sit behind a
> Cloudflare challenge. Manga Downloader launches the browser itself and only
> shows a window when a challenge needs solving. Pass `--browser-visible` to
> skip the headless attempt and show the window from the start.

Other sites built on the same platforms often work too, even when they aren't
listed. If one doesn't, [open an issue][issues] or send a pull request.

## Installation

### Prebuilt binary

Download the archive for your system from the [releases page][releases], extract
it and run the binary:

~~~bash
./manga-downloader --help
~~~

To run it from any folder, move the binary to a folder in your `PATH`, such as
`/usr/local/bin` on Linux and macOS.

<details open>
<summary><b>Windows</b>: step-by-step setup</summary>
<br>

> [!IMPORTANT]
> Manga Downloader is a command line tool. **Double-clicking
> `manga-downloader.exe` won't work**: a window flashes and closes straight
> away. Run it from a terminal instead, as shown below.

1. Open **PowerShell**: press the Start button, type `PowerShell` and press
   <kbd>Enter</kbd>.
2. Paste this line and press <kbd>Enter</kbd>:

   ~~~powershell
   irm https://raw.githubusercontent.com/elboletaire/manga-downloader/master/install.ps1 | iex
   ~~~

3. Close the terminal and open a new one. Then run it from any folder:

   ~~~cmd
   manga-downloader https://mangadex.org/title/a1c7c817-4e59-43b7-9365-09675a149a6f/one-piece 1-10
   ~~~

The [install script](./install.ps1) downloads the latest release to
`%LOCALAPPDATA%\Programs\manga-downloader` and adds that folder to your `PATH`.
It doesn't need administrator rights. Run the same line again to update, and
delete that folder to uninstall.

> [!TIP]
> Files are saved in the folder the terminal is in. To download straight into
> a folder, open it in File Explorer, type `cmd` in the address bar and press
> <kbd>Enter</kbd>. A terminal opens there, ready to use.

<details>
<summary>Prefer not to run a script?</summary>
<br>

Download the `windows-amd64` zip from the [releases page][releases], extract it,
and copy `manga-downloader.exe` to `C:\Windows\System32`. Windows asks for
administrator permission. After that, `manga-downloader` works in any terminal.

> [!WARNING]
> This works, but it isn't the best place for the program: it needs
> administrator rights and mixes it with Windows' own files. To update or
> uninstall it, you have to go back to that folder and replace or delete the
> file yourself.

If Windows SmartScreen shows *"Windows protected your PC"*, the binary is only
flagged because it isn't signed. Click **More info** → **Run anyway**.

</details>

</details>

<details>
<summary><b>macOS</b>: allowing an unsigned binary</summary>
<br>

The binary isn't signed, so Gatekeeper blocks it the first time you run it:

1. Run `./manga-downloader` once and dismiss the warning.
2. Open **System Settings** → **Privacy & Security**, scroll down to
   **Security**, and click **Open Anyway** next to the *manga-downloader*
   message.
3. Run `./manga-downloader` again and confirm the dialog.

You only need to do this once.

</details>

### Docker

~~~bash
docker run --rm -it -v "$PWD:/downloads" elboletaire/manga-downloader [url] [chapters]
~~~

Files are written to the mounted `/downloads` folder. They're owned by
`USER_ID`/`GROUP_ID`, which both default to `1000`. If your user has a
different ID, pass it with
`-e USER_ID=$(id -u) -e GROUP_ID=$(id -g)`.

The default image has no browser. For sites marked with `*`, use the `:browser`
image, which ships Chromium and a virtual display:

~~~bash
docker run --rm -it -v "$PWD:/downloads" elboletaire/manga-downloader:browser [url] [chapters]
~~~

<details>
<summary>Showing the browser window on your own display</summary>
<br>

The `:browser` image passes Cloudflare challenges on its own. If a challenge
needs a real person, forward the window to your Linux desktop. You may need to
run `xhost +local:` first:

~~~bash
docker run --rm -it -e DISPLAY=$DISPLAY -v /tmp/.X11-unix:/tmp/.X11-unix \
    -v "$PWD:/downloads" elboletaire/manga-downloader:browser --browser-visible [url] [chapters]
~~~

</details>

### Go

~~~bash
go install github.com/elboletaire/manga-downloader@latest
~~~

> [!TIP]
> Binaries installed this way don't report a version number, because the
> version is only added to release builds.

## Usage

~~~bash
manga-downloader [flags] [url] [chapters]
~~~

- `url` is the manga's **index page**, the page that lists all its chapters.
  Don't use the URL of a single chapter.
- `chapters` is a comma-separated list of numbers and ranges, such as
  `1,3,5-10`. If you leave it out, the tool asks whether to download every
  chapter. The answer defaults to "no".

Arguments aren't positional, so `manga-downloader 1-50 [url]` works too.

![download img]

### Language and scanlation group

Some sites, such as MangaDex, list each chapter once per language. Use
`--language` to download only one of them:

~~~bash
manga-downloader --language es https://mangadex.org/title/a1c7c817-4e59-43b7-9365-09675a149a6f/one-piece 1-10
~~~

Sites that host several scanlation groups' versions of the same chapter download
the group with the most chapters, and list the other groups. Use
`--scanlator <name>` to pick a group, or `--scanlator all` to download every
version. With `all`, the group name is added to each file name.

### Bundling and output format

~~~bash
# all chapters in a single CBZ, one folder per chapter inside it
manga-downloader --bundle [url] 1-8

# plain image folders instead of CBZ files
manga-downloader --format raw [url] 1-8
~~~

![bundle img]

### Image conversion

Most e-readers can't display AVIF, so a chapter with AVIF pages looks empty on
the device even though the download succeeded. **AVIF pages are converted to
JPEG by default.** WebP renders fine in phone apps but is unreliable on e-ink,
so converting it is opt-in:

~~~bash
manga-downloader --convert-images avif,webp [url] 1-10   # strict e-ink setup
manga-downloader --convert-images none [url] 1-10        # keep pages untouched
~~~

JPEG, PNG and GIF pages are never re-encoded. If a page fails to convert, it's
kept as it was and a warning is shown, so the rest of the chapter isn't lost.

> [!NOTE]
> The AVIF decoder is embedded as WebAssembly, so you don't need to install any
> system library. It's slower on the 32-bit (`386`) builds, so use
> `--convert-images none` there if speed matters.

### File names

Use `--filename-template` to change file names. It takes a
[Go text/template][go template] string, and the default is:

~~~
{{.Series}} {{.Number}} - {{.Title}}{{if gt .Version 1}} v{{.Version}}{{end}}
~~~

`{{.Version}}` is a counter that's added when two files would have the same
name.

### Options

| Flag                  | Short | Description                                              | Default        |
| --------------------- | ----- | -------------------------------------------------------- | -------------- |
| `--output-dir`        | `-o`  | Folder to write the downloaded files to                  | current folder |
| `--bundle`            | `-b`  | Bundle all chapters into a single file                   | off            |
| `--format`            | `-f`  | Output format: `cbz` or `raw`                            | `cbz`          |
| `--filename-template` | `-t`  | Template for the file names                              | see above      |
| `--language`          | `-l`  | Only download the specified language                     | all            |
| `--scanlator`         | `-s`  | Only download the specified scanlation group (or `all`)  | most chapters  |
| `--convert-images`    |       | Formats to convert to JPEG: `avif`, `webp` or `none`     | `avif`         |
| `--concurrency`       | `-c`  | Number of chapters downloaded at once (max 5)            | 5              |
| `--concurrency-pages` | `-C`  | Number of pages downloaded at once per chapter (max 10)  | 10             |
| `--retry`             | `-r`  | Retries per failed page (max 3, `0` disables retrying)   | 1              |
| `--browser-visible`   |       | Show the browser window from the start                   | off            |
| `--version`           | `-v`  | Show version information                                 |                |

Run `manga-downloader help` to list them in your terminal.

![help img]

## Troubleshooting

<details>
<summary><b>The newest chapter downloads empty or is missing</b></summary>
<br>

Many sites keep their latest chapters behind a paywall, and the page often looks
normal. Try a slightly older chapter to confirm that the site works.

</details>

<details>
<summary><b>Old chapters are missing, or a warning says the range starts before the first chapter</b></summary>
<br>

Sites often delete old chapters. The tool can only download chapters that the
site still lists.

</details>

<details>
<summary><b>A browser window pops up</b></summary>
<br>

This is expected for sites marked with `*`. The window opens when the site shows
a Cloudflare challenge. Checkboxes are clicked automatically, but some challenges
need you to click them yourself.

</details>

<details>
<summary><b>Pages don't show up on my e-reader</b></summary>
<br>

The site probably serves WebP images. Download the chapters again with
`--convert-images avif,webp`.

</details>

## Building from source

You need Go 1.19 or newer:

~~~bash
make install   # download dependencies
make build     # test and build ./manga-downloader
~~~

## Star history

<div align="center">

[![Star history chart][star history]][stargazers]

</div>

[downloads badge]: https://img.shields.io/github/downloads/elboletaire/manga-downloader/total
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
[docker hub]: https://hub.docker.com/r/elboletaire/manga-downloader
[download img]: https://raw.githubusercontent.com/elboletaire/manga-downloader/master/demos/download.gif
[bundle img]: https://raw.githubusercontent.com/elboletaire/manga-downloader/master/demos/bundle.gif
[help img]: https://raw.githubusercontent.com/elboletaire/manga-downloader/master/demos/help.gif
[prompt img]: https://raw.githubusercontent.com/elboletaire/manga-downloader/master/demos/prompt.gif
