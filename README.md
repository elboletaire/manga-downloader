<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/brand/readme/readme-header-dark.svg">
  <img alt="manga downloader" src="assets/brand/readme/readme-header-light.svg">
</picture>

[![Tests][tests badge]][tests]
[![GitHub release][release badge]][releases]
[![GitHub downloads][downloads badge]][releases]
[![GitHub stars][stars badge]][stargazers]
[![Docker Pulls][pulls badge]][docker hub]
[![License][license badge]][license]

⭐ If you like this project, star it on GitHub!

[Website][website] • [Features](#features) • [Installation](#installation) • [Desktop app](#using-the-desktop-app---guide) • [Command line](#using-the-command-line---guide) • [Supported sites](#supported-sites) • [Troubleshooting](#troubleshooting)

</div>

Manga Downloader downloads manga chapters from MangaDex and more than 500 other sites and
packs them into CBZ, CBR or EPUB files, ready to read on your e-reader or in
your favorite comic app. It comes as a **desktop app** for Windows, macOS and
Linux, and as a **command line tool** for scripts, servers and Docker.

<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/screenshots/02-loaded-dark.webp">
  <img alt="The Manga Downloader desktop app with a series loaded" src="assets/screenshots/02-loaded-light.webp">
</picture>

</div>

## Features

- **[584 supported sites](#supported-sites)**, from big aggregators to small
  scanlation groups.
- In the app, **paste a URL and it loads**: the series, its chapters and the
  languages and scanlation groups it offers appear on their own.
- **Pick what you need**: chapter ranges such as `1,3,5-10` (Shift+click or
  *All* in the app), and the language or scanlation group you want, on sites
  that host several versions of a chapter.
- **Keep a folder up to date**: `--skip-existing` leaves out the chapters you
  already have, so a scheduled run only fetches the new ones.
- **E-reader friendly output**: AVIF pages are converted to JPEG, so they don't
  show up blank on Kobo, KOReader or Calibre (WebP too, if you ask).
- **Flexible output**: one file per chapter, per volume or for everything, as
  CBZ, CBR, EPUB or plain image folders, named from a template.
- **Skip pages** such as credits or ads (`--skip-pages`).
- In the app, **live progress**: every chapter shows up as it's fetched and
  packed, with a summary and an *Open folder* button when it's done.
- **Sites that need a browser** (JavaScript or Cloudflare) work too: the tool
  drives a Chromium-based browser you already have and clicks the "Verify you
  are human" box for you.
- **Concurrent downloads** with retries.
- **Command line**: a single binary with no runtime, no dependencies and no
  config file, plus Docker images.
- **Desktop app**: native folder picker, and a window that follows your system's
  light or dark theme.

## Installation

### Installing the desktop app

[![Desktop app downloads][app downloads badge]][releases]

The easiest way is the [website][website], which picks the right download for
your system. Or get it from the [releases page][releases]:

- **Windows**: `manga-downloader-gui-<tag>-windows-amd64-installer.exe`, or
  `-windows-arm64-installer.exe` for ARM devices such as Snapdragon laptops. It
  installs for your user alone (`%LOCALAPPDATA%\Programs\Manga Downloader`), or
  for everyone (`C:\Program Files\Manga Downloader`) when run as administrator,
  and adds a Start menu shortcut. For a portable copy, use the `.zip` of the
  same name instead: unzip it and run `manga-downloader-gui.exe`.
- **macOS**: `manga-downloader-gui-<tag>-darwin-universal.zip`. Unzip it and move
  `Manga Downloader.app` wherever you like. Or install it with
  [Homebrew][homebrew], which also puts `manga-downloader-gui` on your `PATH`:

  ~~~bash
  brew install --cask elboletaire/tap/manga-downloader-app
  ~~~

  If you installed the app under its old name, `manga-downloader`, run
  `brew uninstall --cask manga-downloader` first.

- **Linux**: `manga-downloader-gui-<tag>-linux-amd64.tar.gz`, or
  `-linux-arm64.tar.gz`. Extract it and run `install.sh` to add the app to
  your applications menu. It installs for your user only, into `~/.local`,
  with no root needed. Run it again to update, or with `--uninstall` to
  remove it (set the same `XDG_DATA_HOME`, if you used one):

  ~~~bash
  tar -xzf manga-downloader-gui-<tag>-linux-amd64.tar.gz
  ./manga-downloader-gui-<tag>-linux-amd64/install.sh
  ~~~

  Or just run `manga-downloader-gui` from the extracted folder. It needs
  WebKitGTK 4.1 (`libwebkit2gtk-4.1-0` on Debian and Ubuntu), which most
  desktops already have; `install.sh` tells you how to get it if it's
  missing.

> [!NOTE]
> **macOS** doesn't sign the app, so Gatekeeper blocks its first launch: allow it
> once under *System Settings → Privacy & Security* ([FAQ][faq-gatekeeper]).
>
> **Windows** may show SmartScreen's *"Windows protected your PC"* for the
> installer or the app, for the same reason: click **More info** → **Run anyway**
> ([FAQ][faq-smartscreen]). The app also needs the Microsoft Edge WebView2
> runtime, which Windows 11 and current Windows 10 already have; without it, the
> app offers to install it on first launch.

If the desktop app sits next to the command line tool or is on your `PATH` (the
Homebrew cask and the Linux `install.sh` put it there), `manga-downloader ui`
opens it. On Windows, double-clicking `manga-downloader.exe` does the same.

The app checks for a newer release when it starts and offers to update itself.
Settings (the gear icon, or Ctrl+, / ⌘,) turns that off and sets the app's
language; the [app guide](docs/app.md#settings-and-the-update-check) says what
the check sends.

### Installing the command line tool

[![CLI downloads][cli downloads badge]][releases]

#### Homebrew

On macOS and Linux, install the command line tool from [Homebrew][homebrew]:

~~~bash
brew install elboletaire/tap/manga-downloader
~~~

Homebrew doesn't quarantine the command line tool, so there's no Gatekeeper
step on macOS. `brew upgrade manga-downloader` updates it. Completions for
bash, zsh and fish are installed as well.

That's the command line tool alone; the [desktop app](#installing-the-desktop-app) is the
`manga-downloader-app` cask. Install both and `manga-downloader ui` opens the
app:

~~~bash
brew install elboletaire/tap/manga-downloader elboletaire/tap/manga-downloader-app
~~~

#### Prebuilt binary

Builds are published for Linux, macOS and Windows on amd64 and arm64, plus
32-bit builds for Linux and Windows (the latter named `windows-32bit-legacy`).

Download the archive for your system from the [releases page][releases], extract
it and run the binary:

~~~bash
./manga-downloader --help
~~~

To verify the download, check it against `manga-downloader-<tag>-SHA256SUMS.txt`
from the same release: `sha256sum -c --ignore-missing manga-downloader-<tag>-SHA256SUMS.txt`.

To run it from any folder, move the binary to a folder in your `PATH`, such as
`/usr/local/bin` on Linux and macOS.

`manga-downloader completion bash|zsh|fish|powershell` prints a shell
completion script. For zsh, for example:

~~~bash
manga-downloader completion zsh > "${fpath[1]}/_manga-downloader"
~~~

`manga-downloader completion --help` explains how to set up the other shells.

<details open>
<summary><b>Windows</b>: step-by-step setup</summary>
<br>

> [!IMPORTANT]
> This is the command line tool, so run it from a terminal as shown below:
> double-clicking `manga-downloader.exe` only shows the help ([FAQ][faq-exe]).

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

Download the `windows-amd64` zip from the [releases page][releases] (or
`windows-arm64` if you have an ARM device, such as a Snapdragon laptop), extract
it, and copy `manga-downloader.exe` to `C:\Windows\System32`. Windows asks for
administrator permission. After that, `manga-downloader` works in any terminal.

> [!WARNING]
> It works, but `System32` is Windows' own folder: it needs administrator
> rights, and updating or uninstalling means replacing or deleting the file
> there yourself.

If SmartScreen shows *"Windows protected your PC"*, click **More info** →
**Run anyway** ([why][faq-smartscreen]).

</details>

</details>

On macOS, Gatekeeper blocks the unsigned binary the first time: allow it once
under *System Settings → Privacy & Security* ([FAQ][faq-gatekeeper]).

#### Docker

[![Docker Pulls][pulls badge]][docker hub]

~~~bash
docker run --rm -it -v "$PWD:/downloads" elboletaire/manga-downloader [url] [chapters]
~~~

Files are written to the mounted `/downloads` folder, owned by user and group
`1000`; if yours differ, add `-e USER_ID=$(id -u) -e GROUP_ID=$(id -g)`.

The default image has no browser. For sites marked with `*`, use the `:browser`
image, which ships Chromium and a virtual display:

~~~bash
docker run --rm -it -v "$PWD:/downloads" elboletaire/manga-downloader:browser [url] [chapters]
~~~

`:latest` and `:browser` follow releases; `:master` and `:master-browser` are
the development version.

<details>
<summary>Showing the browser window on your own display</summary>
<br>

The `:browser` image passes Cloudflare challenges on its own. If a challenge
needs a real person, the browser window opens by itself, so forward your display
to see it on your Linux desktop. You may need to run `xhost +local:` first:

~~~bash
docker run --rm -it -e DISPLAY=$DISPLAY -v /tmp/.X11-unix:/tmp/.X11-unix \
    -v "$PWD:/downloads" elboletaire/manga-downloader:browser [url] [chapters]
~~~

</details>

## Using the desktop app - [guide](docs/app.md)

Not installed yet? Get it from the [website][website], or see
[Installation](#installing-the-desktop-app).

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/screenshots/01-start-dark.webp">
  <img alt="The app's start screen: the logo over an empty URL box with its Paste button, Load to its right and Settings below" src="assets/screenshots/01-start-light.webp">
</picture>

Paste a URL and it loads. Pick chapters with the range box, Shift+click or
*All*, choose the language and scanlation group when the site offers them, set
the folder, format and packing in *Options*, then press *Download*. The
*Activity* tab shows progress, and the summary's *Open folder* button takes you
to the files. Files go to your Downloads folder unless you choose another, and
your options are remembered for next time.

The [desktop app guide](docs/app.md) walks through every step and option, with screenshots.

## Using the command line - [guide](docs/cli.md)

Not installed yet? See [Installation](#installing-the-command-line-tool).

~~~bash
manga-downloader [flags] [url] [chapters]
~~~

- `url` is the manga's **index page**, the page that lists all its chapters.
  The URL of a single chapter works too: it resolves to the whole series.
- `chapters` is a list of numbers and ranges, such as `1,3,5-10`, or `all`.
  Leave it out and the tool asks before downloading the whole series.

Arguments aren't positional, so `manga-downloader 1-50 [url]` works too.
`manga-downloader list [url]` prints the chapters, languages, groups and
volumes without downloading, as JSON with `--json`; the app has no
counterpart, as its chapter panel shows the same ([guide](docs/cli.md#listing-a-series)).

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/demos/download.gif">
  <img alt="The command line downloading eight chapters at once, with the range typed before the URL, each chapter with its own progress bar" src="assets/demos/download-light.gif">
</picture>

A few of its flags:

~~~bash
# one language, where the site lists several
manga-downloader --language es [url] 1-10
# one file per volume, as the site files them
manga-downloader --volumes auto [url] 1-20
# everything in a single file
manga-downloader --bundle [url] 1-8
# EPUB instead of CBZ
manga-downloader --format epub [url] 1-8
# only the chapters not in the folder yet, no questions asked
manga-downloader --skip-existing -o ~/Manga/Series [url] all
~~~

`manga-downloader help` lists every flag; the [command line guide](docs/cli.md)
explains them: languages and scanlation groups, bundles and volumes, image
conversion, skipping pages, skipping existing files, file name templates and
environment variables.

## Supported sites

<details>
<summary><b>Show all 584 supported sites</b></summary>
<br>

| Site | Notes | Last verified |
| --- | --- | --- |
| [02.ngomik.cc](https://02.ngomik.cc) | Indonesian | 2026-11-23 |
| [108read.com](https://www.108read.com) | Thai | 2026-11-16 |
| [1manga.co](https://1manga.co) | 1Manga, English | 2026-11-11 |
| [1punchman.com](https://1punchman.com) | English, single series | 2026-11-16 |
| [3asq.online](https://3asq.online) | Arabic, former 3asq.org | 2026-11-11 |
| [4porncomic.com](https://4porncomic.com) | English, mostly NSFW | 2026-11-16 |
| [adulto.seinagi.org.es](https://adulto.seinagi.org.es) | Spanish, NSFW | 2026-11-09 |
| [afroditscans.com](https://afroditscans.com) | Turkish | 2026-10-18 |
| [akanebanashimanga.website](https://akanebanashimanga.website) | English, single series | 2026-11-16 |
| [akazascans.org](https://akazascans.org) | English | 2026-11-16 |
| [allporncomic.com](https://allporncomic.com) | English, mostly NSFW | 2026-11-25 |
| [allporncomic.io](https://allporncomic.io) | English, mostly NSFW | 2026-11-16 |
| [amangaplanet.com.tr](https://www.amangaplanet.com.tr) | Turkish | 2026-11-11 |
| [anikiga.com](https://anikiga.com) | Turkish | 2026-11-16 |
| [animeallstar1.com](https://www.animeallstar1.com) | AnimeAllStar, Spanish | 2026-11-11 |
| [animeallstar30.com](https://animeallstar30.com) | AnimeAllStar, Spanish | 2026-11-16 |
| [animeshinigami.com](https://www.animeshinigami.com) | Italian | 2026-11-16 |
| [animexnovel.com](https://www.animexnovel.com) | Portuguese, also novels | 2026-11-16 |
| [anzmanga25.com](https://www.anzmanga25.com) | AnzManga, Spanish | 2026-11-16 |
| [apcomics.org](https://apcomics.org) | English | 2026-11-23 |
| [apenasuma-fa.blogspot.com](https://apenasuma-fa.blogspot.com) | Portuguese | 2026-11-16 |
| [apollcomics.es](https://apollcomics.es) | Spanish | 2026-11-16 |
| [aquareader.org](https://aquareader.org) | English | 2026-11-16 |
| [arbxcomix.com](https://arbxcomix.com) | Arabic, mostly NSFW | 2026-11-23 |
| [archiviumbar.com](https://archiviumbar.com) | Spanish | 2026-11-11 |
| [arcurafansub.com](https://arcurafansub.com) \* | Turkish | 2026-11-11 |
| [arenascan.com](https://arenascan.com) |  | 2026-11-23 |
| [arhentai.net](https://arhentai.net) | Arabic, mostly NSFW | 2026-11-11 |
| [arlas.online](https://www.arlas.online) | Indonesian | 2026-11-25 |
| [arthurscan.xyz](https://arthurscan.xyz) | Portuguese | 2026-11-23 |
| [artlapsa.com](https://artlapsa.com) | English, locked chapters skipped | 2026-11-25 |
| [asacomi.jp](https://asacomi.jp) | Japanese (Comici), locked chapters skipped | 2026-11-23 |
| [asialotuss.com](https://asialotuss.com) | Spanish | 2026-10-19 |
| [asmotoon.com](https://asmotoon.com) \* | Asmodeus Scans | 2026-10-15 |
| [asterscans.com](https://asterscans.com) | English | 2026-11-23 |
| [asurascans.com](https://asurascans.com) | Asura Scans, former asuratoon.com | 2026-11-23 |
| [athreascans.com](https://athreascans.com) | English | 2026-11-25 |
| [atsu.moe](https://atsu.moe) | Atsumaru | 2026-11-23 |
| [auntmanhwa.com](https://auntmanhwa.com) | English, mostly NSFW | ❌ failing since 2026-11-09 (checked 2026-11-25) |
| [aurorascans.com](https://aurorascans.com) | redirects to qimanga.com | 2026-11-23 |
| [azorafly.com](https://azorafly.com) | Azorafly, Arabic | 2026-11-23 |
| [bacakomik.my](https://bacakomik.my) | Indonesian | 2026-11-25 |
| [bananascans.com](https://bananascans.com) | English | 2026-11-23 |
| [berserkkmanga.com](https://berserkkmanga.com) | English, single series, use a chapter URL | 2026-11-23 |
| [beyondtheataraxia.com](https://www.beyondtheataraxia.com) | Italian | 2026-11-23 |
| [bibibi-comic.com](https://bibibi-comic.com) | Japanese (Comici) | 2026-11-23 |
| [bigcomics.jp](https://bigcomics.jp) | Japanese, Big Comics (Comici) | 2026-11-23 |
| [bigsolo.org](https://bigsolo.org) |  | 2026-10-15 |
| [blackcloveronline.com](https://blackcloveronline.com) | Black Clover | 2026-11-16 |
| [bluesolo.org](https://bluesolo.org) | Blue Solo, French scantrad | 2026-11-23 |
| [bokunohero-academia.online](https://bokunohero-academia.online) | English, single series | 2026-11-23 |
| [boruto2bluevortex.com](https://boruto2bluevortex.com) | English, single series | 2026-11-23 |
| [bunmanga.com](https://bunmanga.com) | English | 2026-11-23 |
| [bymichiby.com](https://bymichiby.com) | Spanish | 2026-11-23 |
| [cafecomyaoi.com.br](https://cafecomyaoi.com.br) | Portuguese, mostly NSFW | 2026-11-23 |
| [cat-300.com](https://cat-300.com) | Thai | 2026-11-23 |
| [celestialmoonscan.es](https://celestialmoonscan.es) | Spanish | ❌ failing since 2026-11-09 (checked 2026-11-25) |
| [chainsawman-chapter.online](https://chainsawman-chapter.online) | Chainsaw Man | 2026-11-23 |
| [chainsawmannmanga.com](https://chainsawmannmanga.com) | Chainsaw Man | 2026-11-16 |
| [championcross.jp](https://championcross.jp) | Japanese, Champion Cross (Comici) | 2026-11-23 |
| [chikari.moe](https://chikari.moe) | English | 2026-11-16 |
| [cocomic.co](https://cocomic.co) | English, mostly NSFW | ❌ failing since 2026-11-23 (checked 2026-11-25) |
| [coffeemanga.net](https://coffeemanga.net) \* | English | 2026-11-02 |
| [comic-action.com](https://comic-action.com) | Japanese, Web Action (GigaViewer) | 2026-11-23 |
| [comic-days.com](https://comic-days.com) | Japanese, Comic DAYS (GigaViewer) | 2026-11-23 |
| [comic-earthstar.com](https://comic-earthstar.com) | Japanese, Comic Earth Star (GigaViewer) | 2026-11-23 |
| [comic-gardo.com](https://comic-gardo.com) | Japanese, Comic Gardo (GigaViewer) | 2026-11-23 |
| [comic-growl.com](https://comic-growl.com) | Japanese, Comic Growl (Comici) | 2026-11-23 |
| [comic-ogyaaa.com](https://comic-ogyaaa.com) | Japanese, Comic Ogyaaa (GigaViewer) | 2026-11-23 |
| [comic-room-base.com](https://comic-room-base.com) | Japanese (Comici) | 2026-11-23 |
| [comic-ryu.jp](https://comic-ryu.jp) | Japanese, Comic Ryu (Comici) | 2026-11-23 |
| [comic-seasons.com](https://comic-seasons.com) | Japanese, Seasons (GigaViewer) | 2026-11-23 |
| [comic-trail.com](https://comic-trail.com) | Japanese, Comic Trail (GigaViewer) | 2026-11-23 |
| [comic-walker.com](https://comic-walker.com) | Japanese, KadoComi (KADOKAWA) | 2026-11-23 |
| [comic-y-ours.com](https://comic-y-ours.com) | Japanese, Comic Y-Ours (GigaViewer) | 2026-11-23 |
| [comic-zenon.com](https://comic-zenon.com) | Japanese, Comic Zenon (GigaViewer) | 2026-11-23 |
| [comic.j-nbooks.jp](https://comic.j-nbooks.jp) | Japanese (Comici) | 2026-11-23 |
| [comic.naver.com](https://comic.naver.com) | Naver Webtoon, Korean, free episodes only | 2026-11-11 |
| [comicborder.com](https://comicborder.com) | Japanese, Comic Border (GigaViewer) | 2026-11-23 |
| [comicride.jp](https://comicride.jp) | Japanese, Comic Ride (Comici) | 2026-11-23 |
| [comics.comici.jp](https://comics.comici.jp) | Japanese (Comici), portal | 2026-11-23 |
| [comics.manga-bang.com](https://comics.manga-bang.com) | Japanese (Comici), locked chapters skipped | 2026-11-23 |
| [comirela.com](https://comirela.com) | Japanese (Comici) | 2026-11-23 |
| [comix.to](https://comix.to) \* |  | 2026-10-15 |
| [covendasbruxonas.com](https://covendasbruxonas.com) | Coven Scan, Portuguese, mostly NSFW | 2026-11-16 |
| [csmmangafree.com](https://www.csmmangafree.com) | English, single series | 2026-11-11 |
| [cucumbermanga.com](https://cucumbermanga.com) | English | 2026-11-23 |
| [dandadanmangas.online](https://dandadanmangas.online) | English, single series | 2026-11-23 |
| [danke.moe](https://danke.moe) | Danke fürs Lesen | 2026-11-23 |
| [datgarscanlation.blogspot.com](https://datgarscanlation.blogspot.com) | Spanish | 2026-11-16 |
| [deatte5.com](https://www.deatte5.com) | English | 2026-11-16 |
| [decadencescans.com](https://reader.decadencescans.com) | Decadence Scans, slow image host | 2026-11-23 |
| [deathtollscans.net](https://reader.deathtollscans.net) |  | 2026-11-23 |
| [demonicscans.org](https://demonicscans.org) | MangaDemon / Demonic Scans | 2026-11-16 |
| [demonslayermanga.us.com](https://demonslayermanga.us.com) | English, single series, use a chapter URL | 2026-11-23 |
| [desirescans.com](https://desirescans.com) | English | 2026-11-23 |
| [diamondfansub.com](https://diamondfansub.com) | Turkish | 2026-11-16 |
| [divascans.org](https://divascans.org) | English, locked chapters skipped | 2026-11-23 |
| [dojing.net](https://dojing.net) | Indonesian, mostly NSFW | 2026-11-16 |
| [doodmanga.com](https://www.doodmanga.com) | Thai | 2026-11-11 |
| [doujin-lc.net](https://doujin-lc.net) | Thai, mostly NSFW | 2026-11-23 |
| [doujin69.com](https://doujin69.com) | Thai, mostly NSFW | 2026-11-23 |
| [doujindistrict.com](https://doujindistrict.com) | English, mostly NSFW | 2026-11-23 |
| [doujinmoon.com](https://doujinmoon.com) | Thai, mostly NSFW | 2026-11-16 |
| [doujinx-h.com](https://doujinx-h.com) | Thai, mostly NSFW | 2026-11-23 |
| [dragonballmangaa.com](https://dragonballmangaa.com) | Dragon Ball Super | ❌ failing since 2026-11-09 (checked 2026-11-25) |
| [dragonballsupermanga.com](https://dragonballsupermanga.com) | English, single series, use a chapter URL | 2026-11-23 |
| [dragontea.ink](https://dragontea.ink) \* |  | 2026-10-15 |
| [dragontranslation.org](https://dragontranslation.org) | Spanish, mostly NSFW | 2026-11-23 |
| [drakecomic.net](https://drakecomic.net) | Drake Scans, former drakecomic.org | 2026-11-23 |
| [duoscanlators.blogspot.com](https://duoscanlators.blogspot.com) | Portuguese | 2026-11-16 |
| [dynasty-scans.com](https://dynasty-scans.com) | Dynasty Reader | 2026-11-23 |
| [ecchi-doujin.com](https://ecchi-doujin.com) | Thai, mostly NSFW | 2026-11-23 |
| [eldermanga.com](https://eldermanga.com) |  | 2026-10-18 |
| [elftoon.net](https://elftoon.net) | Elf Toon, former elftoon.com | 2026-11-23 |
| [en-hijala.com](https://en-hijala.com) | Hijala Translations | 2026-11-16 |
| [en-thunderscans.com](https://en-thunderscans.com) | Thunderscans | 2026-11-09 |
| [erisscans.com](https://erisscans.com) | English, locked chapters skipped | 2026-11-23 |
| [eskimangalar.com](https://eskimangalar.com) |  | 2026-10-18 |
| [euphoriascan.com](https://euphoriascan.com) | Portuguese | 2026-11-23 |
| [evascans.net](https://evascans.net) | former evascans.org | 2026-11-23 |
| [fanfox.net](https://fanfox.net) | Manga Fox | 2026-11-25 |
| [fbsquadx.com](https://fbsquadx.com) | Fleur Blanche Scan, Spanish | ❌ failing since 2026-11-16 (checked 2026-11-25) |
| [feelweb.jp](https://feelweb.jp) | Japanese, Feel Web (GigaViewer) | 2026-11-23 |
| [fenixproject.website](https://fenixproject.website) | Portuguese | 2026-11-23 |
| [fin-manga.com](https://www.fin-manga.com) | Thai, some NSFW | 2026-11-11 |
| [flamecomics.xyz](https://flamecomics.xyz) | Flame Comics, former Flame Scans | 2026-11-23 |
| [flash-manga.net](https://www.flash-manga.net) | Thai | 2026-11-23 |
| [fmteam.fr](https://fmteam.fr) |  | 2026-11-23 |
| [frieren.online](https://ww5.frieren.online) | English | 2026-11-16 |
| [furyosociety.com](https://furyosociety.com) |  | 2026-11-23 |
| [g-comi.jp](https://g-comi.jp) | Japanese (Comici), comic-medu.com redirects here | 2026-11-23 |
| [gaiatoon.com](https://gaiatoon.com) \* | Turkish | 2026-11-11 |
| [gakamangas.com](https://gakamangas.com) | English | 2026-11-23 |
| [galaxscanlator.blogspot.com](https://galaxscanlator.blogspot.com) | Portuguese | 2026-11-16 |
| [garciamanga.com](https://garciamanga.com) | Turkish | ❌ failing since 2026-11-16 (checked 2026-11-25) |
| [gdscans.com](https://gdscans.com) | GalaxyDegenScans | 2026-11-16 |
| [genztoons.org](https://genztoons.org) | Genz Toon, former genzupdates.com | 2026-11-23 |
| [ghostoon.com](https://ghostoon.com) | Turkish, mostly NSFW | 2026-11-23 |
| [globalcomix.com](https://globalcomix.com) | Western comics, free issues only | 2026-11-23 |
| [go-manga.com](https://www.go-manga.com) | Thai, some NSFW | 2026-11-23 |
| [god-doujin.com](https://god-doujin.com) | Thai, mostly NSFW | 2026-11-23 |
| [godhman.net](https://www.godhman.net) | Thai | 2026-11-23 |
| [gourmetsupremacy.com](https://gourmetsupremacy.com) | Gourmet Scans, free chapters only | 2026-11-16 |
| [grabber.zone](https://grabber.zone) | Spanish | 2026-11-16 |
| [grimscans.com](https://grimscans.com) | English, locked chapters skipped | ❌ failing since 2026-11-23 (checked 2026-11-25) |
| [gutsberserk.com](https://www.gutsberserk.com) | English, single series | 2026-10-22 |
| [guya.moe](https://guya.moe) | Guya, Kaguya-sama | 2026-11-11 |
| [hachirumi.com](https://hachirumi.com) | Hachirumi, Guya instance, has an NSFW section | 2026-11-23 |
| [hanamiheaven.org](https://hanamiheaven.org) | Portuguese, mostly NSFW | 2026-10-22 |
| [hanayume.com](https://hanayume.com) | Japanese (Comici), free episodes only | 2026-11-23 |
| [hanmokkuscan.blogspot.com](https://hanmokkuscan.blogspot.com) | Portuguese | 2026-11-16 |
| [harmony-scan.fr](https://harmony-scan.fr) | French | ❌ failing since 2026-11-16 (checked 2026-11-25) |
| [hayacomic.jp](https://hayacomic.jp) | Japanese (Comici) | 2026-11-23 |
| [heavenly-delusion.com](https://heavenly-delusion.com) | Heavenly Delusion | 2026-11-16 |
| [heavenmanga.com](https://heavenmanga.com) | Spanish | 2026-11-16 |
| [heckscans.blogspot.com](https://heckscans.blogspot.com) | Portuguese | 2026-11-16 |
| [hentai-origines.com](https://hentai-origines.com) | French, mostly NSFW | 2026-11-16 |
| [hentai-scantrad.org](https://hentai-scantrad.org) | French, mostly NSFW | 2026-11-16 |
| [hentai1.io](https://hentai1.io) | English, mostly NSFW | 2026-11-16 |
| [hentai20.io](https://hentai20.io) | English, mostly NSFW | 2026-11-16 |
| [hentai20.lol](https://hentai20.lol) | English, mostly NSFW | 2026-11-23 |
| [hentaidesi.com](https://hentaidesi.com) | English, mostly NSFW | 2026-11-16 |
| [hentaimanga.me](https://hentaimanga.me) | English, mostly NSFW | 2026-11-23 |
| [hentaiteca.online](https://hentaiteca.online) | Portuguese, mostly NSFW | 2026-11-16 |
| [hentaivn.show](https://hentaivn.show) | Vietnamese, mostly NSFW | ❌ failing since 2026-11-16 (checked 2026-11-25) |
| [hentaixcomic.com](https://hentaixcomic.com) | English, mostly NSFW | 2026-11-16 |
| [hentaixdickgirl.com](https://hentaixdickgirl.com) | English, mostly NSFW | 2026-11-16 |
| [hentaixyuri.com](https://hentaixyuri.com) | English, mostly NSFW | 2026-11-16 |
| [heros-web.com](https://heros-web.com) | Japanese, Hero's Web (Comici) | 2026-11-23 |
| [hhentai.fr](https://hhentai.fr) | French, mostly NSFW | 2026-11-16 |
| [hirayareads.com](https://hirayareads.com) | English | 2026-11-23 |
| [hivetoons.org](https://hivetoons.org) | HiveToons, VoidScans | 2026-10-15 |
| [hni-scantrad.net](https://hni-scantrad.net) | HNI-scantrad, French and English | 2026-11-23 |
| [hotcabaretscan.com](https://hotcabaretscan.com) | Portuguese | 2026-11-16 |
| [houseofotakusv2.xyz](https://houseofotakusv2.xyz) | Spanish | 2026-11-16 |
| [hunlightcomics.com](https://hunlightcomics.com) | English | 2026-11-16 |
| [ichi-the-witch.com](https://ichi-the-witch.com) | English, single series | 2026-11-11 |
| [ichicomi.com](https://ichicomi.com) | Japanese, Ichi Comic (GigaViewer) | 2026-11-23 |
| [ikuhentai.net](https://ikuhentai.net) | Spanish, NSFW | 2026-11-25 |
| [imanevilgod.com](https://imanevilgod.com) | English, single series | 2026-11-23 |
| [infrafandub.com](https://infrafandub.com) | Spanish | 2026-11-16 |
| [inkapk.net](https://inkapk.net) | Portuguese | 2026-11-16 |
| [inmanga.com](https://inmanga.com) |  | 2026-11-16 |
| [inu-manga.net](https://www.inu-manga.net) | Thai | 2026-11-16 |
| [inventariooculto.com](https://inventariooculto.com) | Spanish | 2026-11-16 |
| [izanamiscans.my.id](https://izanamiscans.my.id) | Indonesian | 2026-11-16 |
| [jellycomics.online](https://jellycomics.online) | Vietnamese, mostly NSFW, use a series URL | 2026-11-16 |
| [jestful.net](https://jestful.net) |  | 2026-11-16 |
| [jojolandsmanga.com](https://jojolandsmanga.com) | The JoJoLands | 2026-11-16 |
| [jujutsukaisenmanga.pro](https://jujutsukaisenmanga.pro) | Jujutsu Kaisen | 2026-11-16 |
| [kaijuno8chapters.com](https://kaijuno8chapters.com) | English, single series | ❌ failing since 2026-11-02 (checked 2026-11-25) |
| [kaijunochapter.com](https://kaijunochapter.com) | English, single series | 2026-11-16 |
| [kaliscan.org](https://kaliscan.org) | English, mostly NSFW | 2026-11-16 |
| [kansai.mag-garden.co.jp](https://kansai.mag-garden.co.jp) | Japanese, Mag Garden Kansai (Comici) | 2026-11-23 |
| [kanzenin.info](https://kanzenin.info) | Indonesian, mostly NSFW | 2026-11-16 |
| [kappabeast.com](https://kappabeast.com) \* |  | 2026-10-15 |
| [kaynscans.com](https://kaynscans.com) | Kayn Scans, former kaynscan.org | 2026-11-16 |
| [kazokuden.com](https://www.kazokuden.com) | Spanish | 2026-11-16 |
| [kencomics.com](https://kencomics.com) | Ken Scans, English, locked chapters skipped | 2026-11-23 |
| [kensscan.com](https://kensscan.com) | English | 2026-11-25 |
| [keroscans.net](https://keroscans.net) | English, locked chapters skipped | 2026-11-25 |
| [kewnscans.org](https://kewnscans.org) | English | 2026-11-16 |
| [kimicomi.com](https://kimicomi.com) | Japanese (Comici) | 2026-11-23 |
| [kingofshojo.com](https://kingofshojo.com) |  | 2026-11-16 |
| [kitsunedawn.com](https://kitsunedawn.com) | English | 2026-11-16 |
| [klikmanga.org](https://klikmanga.org) | Indonesian | 2026-11-11 |
| [kmansin09.top](https://kmansin09.top) | Japanese, raw | 2026-11-23 |
| [kokomangas.com](https://kokomangas.com) | English | 2026-11-16 |
| [kombatch.cc](https://kombatch.cc) | Indonesian | 2026-11-16 |
| [komikdewasa.art](https://komikdewasa.art) | Indonesian, mostly NSFW | 2026-11-16 |
| [komikdewasa.mom](https://komikdewasa.mom) | Indonesian, mostly NSFW | 2026-11-11 |
| [komikindo.ch](https://komikindo.ch) |  | 2026-11-16 |
| [komikindo.click](https://komikindo.click) | Indonesian, mostly NSFW, unrelated to komikindo.ch | 2026-11-16 |
| [komikmanga.com](https://komikmanga.com) | English, Indonesian UI | 2026-11-16 |
| [komikmu.top](https://komikmu.top) | Indonesian | 2026-11-16 |
| [komikstation.org](https://komikstation.org) | Indonesian | 2026-11-16 |
| [komiktap.info](https://komiktap.info) | Indonesian, mostly NSFW | 2026-11-23 |
| [komiku.org](https://komiku.org) | Indonesian | 2026-11-23 |
| [komikzona.my.id](https://komikzona.my.id) | Indonesian | 2026-11-23 |
| [ksgroupscans.com](https://ksgroupscans.com) | KSGroupScans | 2026-11-16 |
| [kumapoi.info](https://kumapoi.info) | Indonesian, mostly NSFW, doujinshi | 2026-11-16 |
| [kuragebunch.com](https://kuragebunch.com) | Japanese, Kurage Bunch (GigaViewer) | 2026-11-23 |
| [lagoonscans.com](https://lagoonscans.com) |  | 2026-11-16 |
| [lavascans.com](https://lavascans.com) | Arabic | 2026-11-23 |
| [lector-darkroomfansub.blogspot.com](https://lector-darkroomfansub.blogspot.com) | Spanish, mostly NSFW | 2026-11-16 |
| [lector.lolivault.net](https://lector.lolivault.net) | Spanish, NSFW | 2026-11-23 |
| [lector.pzykosis666hfansub.com](https://lector.pzykosis666hfansub.com) | Spanish, NSFW | 2026-11-23 |
| [lectorhades.latamtoon.com](https://lectorhades.latamtoon.com) | Spanish | 2026-11-16 |
| [lectortaurus.com](https://lectortaurus.com) | Spanish | 2026-11-16 |
| [leercapitulo.co](https://www.leercapitulo.co) |  | 2026-11-16 |
| [leermanhwas.com](https://leermanhwas.com) | Spanish, mostly NSFW | 2026-11-16 |
| [legionscans.com](https://legionscans.com) | Spanish | 2026-10-19 |
| [leitor.borutoexplorer.com.br](https://leitor.borutoexplorer.com.br) | Portuguese | 2026-11-23 |
| [leitor.kamisama.com.br](https://leitor.kamisama.com.br) | Portuguese | ❌ failing since 2026-11-16 (checked 2026-11-25) |
| [lelmanga.com](https://www.lelmanga.com) | French | 2026-11-16 |
| [lelscan.net](https://lelscan.net) | French | 2026-11-16 |
| [lelscanfr.com](https://www.lelscanfr.com) | French | 2026-11-16 |
| [lepoytl.my.id](https://www.lepoytl.my.id) | Indonesian | 2026-11-16 |
| [ler999.blogspot.com](https://ler999.blogspot.com) | Portuguese | 2026-10-19 |
| [lgbtics.com](https://lgbtics.com) | English, NSFW | 2026-11-23 |
| [lhtranslation.net](https://lhtranslation.net) | LHTranslation | 2026-11-09 |
| [lianscans.com](https://www.lianscans.com) | Indonesian, mostly NSFW | 2026-11-23 |
| [lilymanga.net](https://lilymanga.net) | yuri, some NSFW | 2026-10-29 |
| [limonmanga.com](https://limonmanga.com) |  | 2026-10-18 |
| [linkmanga.com](https://linkmanga.com) | English | ❌ failing since 2026-11-16 (checked 2026-11-25) |
| [loner-tl.blogspot.com](https://loner-tl.blogspot.com) | Arabic | 2026-10-19 |
| [lovesjane.site](https://lovesjane.site) | English | 2026-11-16 |
| [luacomic.org](https://luacomic.org) | LuaScans | 2026-10-29 |
| [lunatoons.net](https://lunatoons.net) | English, some NSFW | 2026-11-23 |
| [lupiteam.net](https://lupiteam.net) | LupiTeam, Italian | 2026-11-23 |
| [m1.manhwalist.my.id](https://m1.manhwalist.my.id) | Indonesian | 2026-11-16 |
| [madaradex.org](https://madaradex.org) | some NSFW | 2026-11-16 |
| [madarascans.net](https://madarascans.net) | former madarascans.org/.com | 2026-11-16 |
| [magcomi.com](https://magcomi.com) | Japanese, Mag Garden (GigaViewer) | 2026-11-23 |
| [magustoon.org](https://magustoon.org) | English, locked chapters skipped | 2026-11-16 |
| [mahoushoujobu.com](https://mahoushoujobu.com) | English | 2026-11-16 |
| [maid.my.id](https://www.maid.my.id) |  | 2026-11-16 |
| [manga-ai-land.blogspot.com](https://manga-ai-land.blogspot.com) | Arabic | 2026-10-19 |
| [manga-corporation.com](https://manga-corporation.com) | Manga-Corporation, French | 2026-11-23 |
| [manga-lc.net](https://manga-lc.net) | Thai | 2026-11-16 |
| [manga-neko.com](https://manga-neko.com) | Thai, some NSFW | 2026-11-23 |
| [manga-zegra.com](https://manga-zegra.com) | Japanese (Comici) | 2026-11-23 |
| [manga.adonisfansub.com](https://manga.adonisfansub.com) | Adonis Fansub, Turkish | 2026-11-16 |
| [manga.fansubs.cat](https://manga.fansubs.cat) | Catalan | 2026-11-16 |
| [manga.smuglo.li](https://manga.smuglo.li) | Lilyreader, yuri | ❌ failing since 2026-11-16 (checked 2026-11-25) |
| [manga18.me](https://manga18.me) | English, NSFW | 2026-11-23 |
| [manga18free.com](https://manga18free.com) | English, mostly NSFW | 2026-11-16 |
| [manga18fx.com](https://manga18fx.com) | English, NSFW | 2026-11-23 |
| [mangaball.com](https://mangaball.com) | former mangaball.net | 2026-10-18 |
| [mangabats.com](https://www.mangabats.com) | former mangabat.com | 2026-10-19 |
| [mangaberri.com](https://mangaberri.com) |  | 2026-11-16 |
| [mangabolt.com](https://mangabolt.com) | English, use a series URL | 2026-11-25 |
| [mangabu.jp](https://mangabu.jp) | Japanese (Comici) | 2026-11-23 |
| [mangacrazy.net](https://mangacrazy.net) | English, mostly NSFW | 2026-11-16 |
| [mangadass.com](https://mangadass.com) | English, NSFW | 2026-11-23 |
| [mangadelsiglotvmanga.com.mx](https://www.mangadelsiglotvmanga.com.mx) | Manga del Siglo TV Manga, Spanish | 2026-10-29 |
| [mangadenizi.net](https://www.mangadenizi.net) |  | 2026-11-16 |
| [mangadex.org](https://mangadex.org) | MangaDex | 2026-11-11 |
| [mangadistrict.com](https://mangadistrict.com) | English, mostly NSFW | 2026-11-16 |
| [mangadna.com](https://mangadna.com) | English, NSFW | 2026-11-16 |
| [mangaeclipse.com](https://mangaeclipse.com) | Manga Eclipse | 2026-11-16 |
| [mangafire.to](https://mangafire.to) \* |  | 2026-10-29 |
| [mangaforfree.com](https://mangaforfree.com) | English | 2026-11-16 |
| [mangaforfree.net](https://mangaforfree.net) | multilingual, mostly NSFW | 2026-11-25 |
| [mangafox.fun](https://mangafox.fun) | MangaFox, English | 2026-11-11 |
| [mangafree.info](https://mangafree.info) | English | 2026-11-16 |
| [mangagg.com](https://mangagg.com) | English | 2026-11-11 |
| [mangahere.cc](https://www.mangahere.cc) | MangaHere | 2026-11-16 |
| [mangahere.onl](https://mangahere.onl) | MangaHere, English | 2026-11-11 |
| [mangahub.io](https://mangahub.io) |  | 2026-11-11 |
| [mangahub.us](https://mangahub.us) | English | 2026-11-11 |
| [mangaisekaithai.net](https://www.mangaisekaithai.net) | Thai | 2026-11-11 |
| [mangak.io](https://mangak.io) | MangaK, former mangabuddy.com | 2026-11-16 |
| [mangakakalot.fun](https://mangakakalot.fun) | MangaKakalot, English | 2026-11-11 |
| [mangakakalot.gg](https://www.mangakakalot.gg) \* | MangaKakalot | 2026-10-15 |
| [mangakakalove.com](https://www.mangakakalove.com) | MangaKakalot, English | 2026-11-23 |
| [mangakatana.com](https://mangakatana.com) |  | 2026-11-16 |
| [mangakimi.com](https://www.mangakimi.com) | Thai | 2026-11-16 |
| [mangakulesi.com](https://mangakulesi.com) | Turkish | 2026-11-25 |
| [mangalami.com](https://mangalami.com) | LamiManga, Thai | 2026-11-16 |
| [mangalib.me](https://mangalib.me) | MangaLib | 2026-10-15 |
| [mangalivre.to](https://mangalivre.to) | Manga Livre, former mangalivre.tv/mangalivre.net | 2026-11-16 |
| [mangalt.jp](https://mangalt.jp) | Japanese (Comici) | 2026-11-23 |
| [mangamaniacs.org](https://mangamaniacs.org) | English, mostly NSFW, yaoi | 2026-11-16 |
| [manganato.gg](https://www.manganato.gg) \* | MangaNato | 2026-11-16 |
| [manganel.me](https://manganel.me) | MangaNel, English | 2026-11-11 |
| [mangaonline.fun](https://mangaonline.fun) | MangaOnline, English | 2026-11-11 |
| [mangaowl.io](https://mangaowl.io) | English | 2026-11-11 |
| [mangapanda.onl](https://mangapanda.onl) | MangaPanda, English | 2026-11-11 |
| [mangapark.cc](https://mangapark.cc) | MangaPark | 2026-11-16 |
| [mangapark.tv](https://mangapark.tv) | English, unrelated to mangapark.cc | 2026-11-16 |
| [mangapill.com](https://mangapill.com) |  | 2026-11-16 |
| [mangaplus.shueisha.co.jp](https://mangaplus.shueisha.co.jp) | Manga Plus, Shueisha's official reader | 2026-11-23 |
| [mangaread.org](https://www.mangaread.org) |  | 2026-11-16 |
| [mangareader.site](https://mangareader.site) | MangaReader, English | 2026-11-11 |
| [mangareadz.com](https://www.mangareadz.com) | English | 2026-11-23 |
| [mangas-origines.fr](https://mangas-origines.fr) | French | 2026-10-18 |
| [mangashelf.co.il](https://mangashelf.co.il) | Hebrew | 2026-11-23 |
| [mangaspa.nikkan-spa.jp](https://mangaspa.nikkan-spa.jp) | Japanese (Comici), mostly NSFW | 2026-11-23 |
| [mangastep.com](https://mangastep.com) | Thai | 2026-11-02 |
| [mangasushi.org](https://mangasushi.org) |  | 2026-11-11 |
| [mangasusuku.com](https://mangasusuku.com) | Indonesian, mostly NSFW | 2026-11-16 |
| [mangataro.org](https://mangataro.org) |  | 2026-11-23 |
| [mangatime-square.com](https://mangatime-square.com) | Japanese, Mangatime Square (GigaViewer) | 2026-11-23 |
| [mangatoday.fun](https://mangatoday.fun) | MangaToday, English | 2026-11-11 |
| [mangatop.org](https://mangatop.org) | English | 2026-11-16 |
| [mangatown.com](https://www.mangatown.com) |  | 2026-11-23 |
| [mangatrend.org](https://mangatrend.org) | English | 2026-11-23 |
| [mangavault.lat](https://mangavault.lat) | MangaVault | 2026-11-11 |
| [mangaworld.me](https://www.mangaworld.me) | MangaWorld, Italian, former mangaworld.mx | 2026-11-25 |
| [mangawow.org](https://mangawow.org) | MangaWOW, Turkish | 2026-11-23 |
| [mangaxhentai.com](https://mangaxhentai.com) | Arabic, mostly NSFW | 2026-11-23 |
| [mangazin.org](https://mangazin.org) | MANGA ZIN | 2026-11-23 |
| [mangtto.com](https://mangtto.com) | Mangitto | 2026-10-15 |
| [manhuabug.com](https://www.manhuabug.com) | Thai | 2026-11-23 |
| [manhuahot.com](https://manhuahot.com) | English | 2026-11-23 |
| [manhuanext.com](https://manhuanext.com) | English | 2026-11-23 |
| [manhuaplus.com](https://manhuaplus.com) |  | 2026-11-23 |
| [manhuaplus.org](https://manhuaplus.org) | English | 2026-11-16 |
| [manhuaplus.top](https://manhuaplus.top) | English | 2026-11-16 |
| [manhuathai.com](https://www.manhuathai.com) | Thai | 2026-11-23 |
| [manhuatop.org](https://manhuatop.org) \* |  | 2026-11-10 |
| [manhuaus.com](https://manhuaus.com) \* |  | 2026-10-15 |
| [manhwa-online.com](https://manhwa-online.com) | Spanish | 2026-11-16 |
| [manhwa18.cc](https://manhwa18.cc) | English, NSFW | 2026-11-23 |
| [manhwa18.net](https://manhwa18.net) | English, NSFW | 2026-11-16 |
| [manhwa18tx.com](https://manhwa18tx.com) | English, NSFW | 2026-11-23 |
| [manhwa68.com](https://manhwa68.com) | English, mostly NSFW | ❌ failing since 2026-11-16 (checked 2026-11-25) |
| [manhwabreakup.com](https://www.manhwabreakup.com) | Thai | 2026-11-23 |
| [manhwabuddy.com](https://manhwabuddy.com) | English, raws | 2026-11-16 |
| [manhwaclub.net](https://manhwaclub.net) | multilingual | 2026-11-16 |
| [manhwaddict.com](https://manhwaddict.com) | English, NSFW | 2026-11-23 |
| [manhwaden.com](https://www.manhwaden.com) | English | 2026-11-11 |
| [manhwadesu.wiki](https://manhwadesu.wiki) | Indonesian, mostly NSFW | 2026-11-16 |
| [manhwagalaxy.com](https://manhwagalaxy.com) | English, NSFW | 2026-11-23 |
| [manhwahub.to](https://manhwahub.to) | English, mostly NSFW | 2026-11-16 |
| [manhwanex.com](https://manhwanex.com) | English | 2026-11-16 |
| [manhwaonline.org](https://manhwaonline.org) |  | 2026-11-16 |
| [manhwareads.com](https://manhwareads.com) | English | 2026-11-16 |
| [manhwatop.com](https://manhwatop.com) | MANHWATOP | 2026-11-16 |
| [manhwaus.net](https://manhwaus.net) | English | 2026-11-23 |
| [marmota.me](https://marmota.me) | Spanish | 2026-11-16 |
| [mehentai.live](https://mehentai.live) | Vietnamese, mostly NSFW | 2026-11-16 |
| [merlinshoujo.com](https://merlinshoujo.com) | Turkish | 2026-11-23 |
| [mgeko.cc](https://www.mgeko.cc) |  | 2026-11-16 |
| [mhscans.com](https://mhscans.com) | Spanish | 2026-11-16 |
| [mihentai.net](https://mihentai.net) | Indonesian, mostly NSFW | 2026-11-16 |
| [mikrokosmosfb.blogspot.com](https://mikrokosmosfb.blogspot.com) | Turkish, mostly NSFW | 2026-11-25 |
| [mindafansub.nl](https://mindafansub.nl) | Turkish, mostly NSFW, former mindafansub.dev | 2026-11-23 |
| [mistscans.com](https://mistscans.com) |  | 2026-11-16 |
| [mkissa.to](https://mkissa.to) \* |  | 2026-10-15 |
| [monopolymanhua.com](https://monopolymanhua.com) | Spanish | 2026-11-16 |
| [montetaiscanlator.xyz](https://montetaiscanlator.xyz) | Portuguese | 2026-11-23 |
| [mrtenzus.com](https://mrtenzus.com) | Portuguese | ❌ failing since 2026-11-23 (checked 2026-11-25) |
| [mundomanhwa.com](https://mundomanhwa.com) | Spanish | 2026-11-16 |
| [myreadingcomic.com](https://myreadingcomic.com) | English, mostly NSFW | 2026-11-23 |
| [nabicix.com](https://www.nabicix.com) | Turkish, former nabimanga.com | 2026-11-16 |
| [namicomic.jp](https://namicomic.jp) | Japanese (Comici) | 2026-11-23 |
| [narasininja.net](https://narasininja.net) | Indonesian | ❌ failing since 2026-10-26 (checked 2026-11-25) |
| [natomanga.com](https://www.natomanga.com) \* | MangaNato, former manganato.com/manganelo.com | 2026-10-15 |
| [natsu.one](https://natsu.one) | NatsuID, Indonesian, some NSFW, former natsu.id | 2026-11-16 |
| [nebulosascan.com](https://nebulosascan.com) | Portuguese | 2026-11-23 |
| [nekoproject.org](https://nekoproject.org) | Spanish | 2026-11-23 |
| [ngamenkomik05.blogspot.com](https://ngamenkomik05.blogspot.com) | Indonesian | 2026-11-25 |
| [nhaduong.com](https://nhaduong.com) | Vietnamese | 2026-11-23 |
| [niji-translations.com](https://niji-translations.com) | Niji, Arabic, yuri | 2026-11-16 |
| [ninjacomics.xyz](https://ninjacomics.xyz) | Portuguese | 2026-11-16 |
| [nirvanamanga.com](https://nirvanamanga.com) | Turkish | ❌ failing since 2026-11-09 (checked 2026-11-25) |
| [nocfsb.com](https://nocfsb.com) | Portuguese, mostly NSFW | 2026-11-25 |
| [nonafansub.com](https://nonafansub.com) | Turkish, NSFW | ❌ failing since 2026-11-23 (checked 2026-11-25) |
| [noranofansub.com](https://www.noranofansub.com) | NoraNoFansub, Spanish | 2026-11-16 |
| [noromax02.my.id](https://noromax02.my.id) | Indonesian | 2026-11-23 |
| [novelcrow.com](https://novelcrow.com) | English, mostly NSFW, Western comics | ❌ failing since 2026-11-16 (checked 2026-11-25) |
| [noxsubs.net](https://noxsubs.net) | Turkish | 2026-11-23 |
| [ntr-manga.net](https://www.ntr-manga.net) | Thai, mostly NSFW | 2026-11-23 |
| [nyanukafe.com](https://nyanukafe.com) | English | 2026-11-23 |
| [nyraxmanga.com](https://nyraxmanga.com) | Nyrax, some NSFW | 2026-11-16 |
| [nyxscans.com](https://nyxscans.com) | English, locked chapters skipped | 2026-11-23 |
| [octopusmanga.com](https://octopusmanga.com) | English | 2026-11-23 |
| [omegascans.org](https://omegascans.org) | English, some chapters paid | 2026-11-16 |
| [omniscientreadersviewpointt.com](https://omniscientreadersviewpointt.com) | English, single series, use a chapter URL | 2026-11-23 |
| [onemanga.info](https://onemanga.info) | OneManga, English | 2026-11-11 |
| [onepunchmannmanga.com](https://onepunchmannmanga.com) | English, single series, use a chapter URL | 2026-11-23 |
| [online.seinagi.org.es](https://online.seinagi.org.es) | Spanish | 2026-11-09 |
| [onlinechainsawmanmanga.us](https://onlinechainsawmanmanga.us) | English, single series, use a chapter URL | 2026-11-23 |
| [onma.me](https://onma.me) | Arabic, former onma.top | 2026-11-25 |
| [orchisasia.org](https://www.orchisasia.org) | English, mostly NSFW | 2026-11-16 |
| [oremanga.net](https://www.oremanga.net) | Oremanga, Thai | 2026-11-23 |
| [orion-scans.com](https://orion-scans.com) | English, locked chapters skipped | 2026-11-23 |
| [orvmanhwa.online](https://orvmanhwa.online) | English, single series, use a chapter URL | 2026-11-23 |
| [ourfeel.jp](https://ourfeel.jp) | Japanese, Our Feel (GigaViewer) | 2026-11-23 |
| [paradise-bl.com](https://paradise-bl.com) \* | Arabic | 2026-11-11 |
| [paritehaber.com](https://www.paritehaber.com) | English, mostly NSFW | 2026-11-16 |
| [petrotechsociety.org](https://www.petrotechsociety.org) | English, mostly NSFW | 2026-11-16 |
| [phoenixscans.com](https://www.phoenixscans.com) | Phoenix Scans, Italian | 2026-11-23 |
| [piacomic.jp](https://piacomic.jp) | Japanese (Comici) | 2026-11-23 |
| [pornhwareader.com](https://pornhwareader.com) | English, NSFW | 2026-11-10 |
| [pornhwaz.com](https://www.pornhwaz.com) | English, mostly NSFW | 2026-11-23 |
| [projectsuki.com](https://projectsuki.com) |  | 2026-11-23 |
| [qimanga.com](https://qimanga.com) |  | 2026-11-23 |
| [r1.richtoon.top](https://r1.richtoon.top) | Spanish | ❌ failing since 2026-11-23 (checked 2026-11-25) |
| [rackusreads.com](https://rackusreads.com) | Rackus | 2026-11-16 |
| [ragnarokscanlation.org](https://ragnarokscanlation.org) | Spanish | 2026-11-23 |
| [rakuentranslations.com](https://rakuentranslations.com) | Spanish | 2026-11-23 |
| [ravenscans.org](https://ravenscans.org) \* |  | 2026-11-02 |
| [rawdex.net](https://rawdex.net) | mostly NSFW | 2026-11-23 |
| [rawkuma.net](https://rawkuma.net) |  | 2026-11-23 |
| [re-yume.my.id](https://www.re-yume.my.id) | Re-Yume, Indonesian | 2026-11-16 |
| [read-nifteam.info](https://read-nifteam.info) | Italian | 2026-11-23 |
| [read.yagami.me](https://read.yagami.me) | Russian | 2026-11-16 |
| [readberserk.com](https://readberserk.com/manga/berserk/) | English | 2026-11-23 |
| [reader.gtothegreatsite.net](https://reader.gtothegreatsite.net) | GTO The Great Site, Italian | 2026-11-23 |
| [reader.mangatellers.gr](https://reader.mangatellers.gr) | Greek | 2026-11-23 |
| [reader.seinagi.org.es](https://reader.seinagi.org.es) | English | 2026-11-09 |
| [readhunters.xyz](https://readhunters.xyz) | Portuguese | 2026-11-16 |
| [readkagura.com](https://readkagura.com) | English, single series | 2026-11-23 |
| [readmanga18.com](https://readmanga18.com) | English, NSFW | 2026-11-23 |
| [readspyxfamilymanga.com](https://readspyxfamilymanga.com) | English, single series, use a chapter URL | 2026-11-25 |
| [reapertrans.com](https://reapertrans.com) | Thai | 2026-11-23 |
| [resetscans.net](https://www.resetscans.net) | English, Reset Scans | 2026-11-10 |
| [revivalscans.com](https://www.revivalscans.com) | English | 2026-11-11 |
| [rimacomiplus.jp](https://rimacomiplus.jp) | Japanese (Comici) | 2026-11-23 |
| [ritharscans.com](https://ritharscans.com) |  | 2026-11-23 |
| [rizzfables.com](https://rizzfables.com) |  | 2026-11-25 |
| [rokaricomics.com](https://rokaricomics.com) |  | 2026-11-23 |
| [roliascan.com](https://roliascan.com) |  | 2026-11-23 |
| [rosyscans.com](https://rosyscans.com) | Indonesian | 2026-11-16 |
| [ruahapchanhday2.com](https://ruahapchanhday2.com) | Vietnamese | 2026-11-23 |
| [rubycomics.net](https://rubycomics.net) | English | 2026-11-16 |
| [ruyamanga2.com](https://www.ruyamanga2.com) | Rüya Manga, Turkish | 2026-11-16 |
| [s2read.com](https://s2read.com) | English | 2026-11-23 |
| [sacachispa.site](https://sacachispa.site) |  | 2026-11-23 |
| [sakamotodays.com.lv](https://sakamotodays.com.lv) | English, single series | 2026-11-09 |
| [samurai.j5z.xyz](https://samurai.j5z.xyz) | Spanish | 2026-11-25 |
| [sanascans.com](https://sanascans.com) | Sana Scans | 2026-11-16 |
| [sasangeyou.net](https://sasangeyou.net) | Indonesian | 2026-11-09 |
| [scan-inmortal.com](https://scan-inmortal.com) | Spanish | 2026-11-23 |
| [scan-vf.net](https://www.scan-vf.net) | ScanVF | 2026-11-09 |
| [scanpinkrosa.blogspot.com](https://scanpinkrosa.blogspot.com) | Portuguese | 2026-11-09 |
| [scythescans.com](https://scythescans.com) | English | 2026-11-09 |
| [sektedoujin.cc](https://sektedoujin.cc) | Indonesian, mostly NSFW | 2026-11-09 |
| [seraphic-deviltry.com](https://seraphic-deviltry.com) | multilingual | 2026-11-09 |
| [sereinscan.com](https://sereinscan.com) | Turkish | 2026-11-09 |
| [setsuscans.com](https://setsuscans.com) \* |  | 2026-10-15 |
| [shadowceviri.blogspot.com](https://shadowceviri.blogspot.com) | Turkish | 2026-11-25 |
| [shijiescans.com](https://shijiescans.com) | Turkish | 2026-11-09 |
| [shinobiscans.com](https://shinobiscans.com) | Italian | 2026-11-09 |
| [shiyurasub.blogspot.com](https://shiyurasub.blogspot.com) | Indonesian | 2026-11-16 |
| [shonenjumpplus.com](https://shonenjumpplus.com) | Japanese, Jump+ (GigaViewer) | 2026-11-23 |
| [siikomik.id](https://siikomik.id) | Indonesian | 2026-11-09 |
| [silentquill.net](https://silentquill.net) | Armageddon Scanlation | 2026-11-11 |
| [sing-manga.com](https://www.sing-manga.com) | Thai | 2026-11-09 |
| [slow-manga.net](https://www.slow-manga.net) | Thai | 2026-11-09 |
| [snscoeurturkey.blogspot.com](https://snscoeurturkey.blogspot.com) | Turkish, mostly NSFW | 2026-11-11 |
| [solooscan.blogspot.com](https://solooscan.blogspot.com) | Portuguese | 2026-11-23 |
| [spanish.seraphic-deviltry.com](https://spanish.seraphic-deviltry.com) | Spanish, NSFW | 2026-11-11 |
| [speed-manga.net](https://speed-manga.net) | Thai | ❌ failing since 2026-11-09 (checked 2026-11-25) |
| [springtoons.com](https://springtoons.com) | English, mostly NSFW, BL | 2026-11-09 |
| [spyxfamilymanga.com](https://spyxfamilymanga.com) | English, single series, use a chapter URL | 2026-11-09 |
| [stonescape.xyz](https://stonescape.xyz) |  | 2026-11-09 |
| [studio.booklista.co.jp](https://studio.booklista.co.jp) | Japanese (Comici), mostly NSFW | 2026-11-23 |
| [sunday-webry.com](https://www.sunday-webry.com) | Japanese, Sunday Webry (GigaViewer) | 2026-11-23 |
| [sushiscan.net](https://sushiscan.net) \* |  | 2026-10-15 |
| [sxfmanga.net](https://www.sxfmanga.net) | English, single series | 2026-11-16 |
| [taiyo.moe](https://taiyo.moe) |  | 2026-11-09 |
| [takecomic.jp](https://takecomic.jp) | Japanese, Take Comic (Comici) | 2026-11-23 |
| [tankouhentai.com](https://tankouhentai.com) | Portuguese, mostly NSFW | 2026-11-09 |
| [tanuki-manga.net](https://www.tanuki-manga.net) | Thai | 2026-11-09 |
| [tarotscans.com](https://www.tarotscans.com) | Turkish | 2026-11-09 |
| [tcbonepiecechapters.com](https://tcbonepiecechapters.com) | TCB Scans, former tcbscans.com | 2026-11-09 |
| [team-shadowi.com](https://www.team-shadowi.com) \* |  | 2026-10-18 |
| [templetoons.com](https://templetoons.com) \* | Temple Scan | 2026-11-11 |
| [tenshimanga.com](https://tenshimanga.com) | Turkish | 2026-11-25 |
| [thaimanga.net](https://www.thaimanga.net) | Thai | 2026-11-09 |
| [the-apothecary-diaries.com](https://the-apothecary-diaries.com) | The Apothecary Diaries | 2026-11-23 |
| [thegirlfromrandomchatting.com](https://thegirlfromrandomchatting.com) | English, single series | 2026-11-09 |
| [tilkiscans.com](https://www.tilkiscans.com) | Turkish | 2026-11-16 |
| [timelesstoons.org](https://timelesstoons.org) | English | 2026-11-09 |
| [tmomanhwa.com](https://tmomanhwa.com) | Spanish, mostly NSFW | 2026-11-09 |
| [tonarinoyj.jp](https://tonarinoyj.jp) | Japanese, Young Jump (GigaViewer) | 2026-11-23 |
| [toomtam-manga.com](https://toomtam-manga.com) | Thai | 2026-11-09 |
| [toongod.cc](https://toongod.cc) | English | 2026-11-09 |
| [toongod.org](https://www.toongod.org) \* |  | 2026-10-15 |
| [toonhunter.com](https://toonhunter.com) | Thai, mostly NSFW | 2026-11-09 |
| [toonily.com](https://toonily.com) \* |  | 2026-10-15 |
| [toonizy.com](https://toonizy.com) | English | 2026-11-16 |
| [toonz.to](https://toonz.to) \* |  | 2026-10-25 |
| [topcomicporno.com](https://topcomicporno.com) | Spanish, mostly NSFW | 2026-11-16 |
| [tortugaceviri.com](https://tortugaceviri.com) | Tortuga Çeviri | 2026-11-16 |
| [tritinia.org](https://tritinia.org) | Tritinia Scans | 2026-11-16 |
| [trmanga.com](https://trmanga.com) |  | 2026-11-16 |
| [trmangaoku.com](https://trmangaoku.com) | Turkish | 2026-11-16 |
| [truyentranhdammyy.site](https://truyentranhdammyy.site) | Vietnamese, mostly NSFW | 2026-11-16 |
| [tsundoku.com.br](https://tsundoku.com.br) | Portuguese | 2026-11-16 |
| [tumanga.net](https://tumanga.net) | Spanish | 2026-11-09 |
| [turkcemanga.com](https://turkcemanga.com) | Turkce Manga, Turkish | 2026-11-16 |
| [tuttoanimemanga.net](https://tuttoanimemanga.net) | TuttoAnimeManga, Italian | 2026-11-09 |
| [tvymanga3.com](https://tvymanga3.com) | Tv y Manga, Spanish | 2026-11-16 |
| [twmanga.com](https://www.twmanga.com) | 包子漫画, baozimh.com mirror | 2026-11-11 |
| [uchuujinmangas.com](https://uchuujinmangas.com) | Spanish | 2026-11-11 |
| [undertranslations.com](https://undertranslations.com) | Spanish | 2026-11-16 |
| [up-manga.com](https://www.up-manga.com) | Thai | 2026-11-23 |
| [utoon.us](https://www.utoon.us) | English, UToon | 2026-11-23 |
| [utsukushii-bg.com](https://utsukushii-bg.com) | Bulgarian | 2026-11-23 |
| [uzaymanga.com](https://uzaymanga.com) |  | 2026-10-18 |
| [v7.kiryuu.to](https://v7.kiryuu.to) | Indonesian | 2026-11-16 |
| [valirscans.org](https://valirscans.org) | English, locked chapters skipped | 2026-11-23 |
| [vermanhwa.com](https://vermanhwa.com) \* | Spanish | 2026-11-02 |
| [vinlandsaga.website](https://vinlandsaga.website) | English, single series | 2026-11-11 |
| [violetmanga.com](https://violetmanga.com) | Violet Scans, former violetscans.org | 2026-11-23 |
| [visorotaku.com](https://visorotaku.com) | Spanish | 2026-11-11 |
| [vortexscans.org](https://vortexscans.org) |  | 2026-11-25 |
| [w14.maid-sama.com](https://w14.maid-sama.com) | English, single series, rotating wNN subdomain | 2026-11-23 |
| [w14.punchonemanga.com](https://w14.punchonemanga.com) | English, single series | 2026-11-23 |
| [w2.dandadanmanga.net](https://w2.dandadanmanga.net) | English, single series | 2026-10-18 |
| [w2.omniscient-readers-viewpoint.online](https://w2.omniscient-readers-viewpoint.online) | English, single series | 2026-11-11 |
| [w2.ruridragon.com](https://w2.ruridragon.com) | English, single series | 2026-11-23 |
| [w22.thebeginningaftertheendmanga.com](https://w22.thebeginningaftertheendmanga.com) | English, single series | 2026-11-25 |
| [w26.boruto-twobluevortex.com](https://w26.boruto-twobluevortex.com) | English, single series | 2026-11-23 |
| [w27.onepunchman-manga.net](https://w27.onepunchman-manga.net) | English, single series | 2026-11-23 |
| [w28.jujutsukaisen-online.com](https://w28.jujutsukaisen-online.com) | English, single series | 2026-11-16 |
| [w28.read-tokyorevengers.com](https://w28.read-tokyorevengers.com) | English, single series | 2026-11-09 |
| [w50.blue-lock-manga.com](https://w50.blue-lock-manga.com) | English, single series | 2026-11-23 |
| [w53.omniscientreaderviewpoint.com](https://w53.omniscientreaderviewpoint.com) | English, single series | 2026-11-23 |
| [w6.dandadan.net](https://w6.dandadan.net) | English, single series | 2026-11-23 |
| [w82.thetowerofgod.com](https://w82.thetowerofgod.com) | English, single series | 2026-11-09 |
| [w9.kaijimanga.com](https://w9.kaijimanga.com) | English, single series | 2026-11-16 |
| [webtoonia.fun](https://webtoonia.fun) | Turkish, former diamondfansub.com, some NSFW | 2026-11-23 |
| [webtoons.com](https://www.webtoons.com) | WEBTOON, free episodes only | 2026-11-11 |
| [weebcentral.com](https://weebcentral.com) |  | 2026-11-23 |
| [witchtoons.net](https://witchtoons.net) | WitchToons, former witchscans.com | 2026-11-23 |
| [wolfscanbr.blogspot.com](https://wolfscanbr.blogspot.com) | Portuguese | 2026-11-25 |
| [writerscans.com](https://writerscans.com) |  | 2026-11-23 |
| [ww1.the-blue-lock.com](https://ww1.the-blue-lock.com) | English, single series, use a chapter URL | 2026-11-09 |
| [ww12.readsnk.com](https://ww12.readsnk.com) | English | 2026-11-23 |
| [ww12.tokyoghoulre.com](https://ww12.tokyoghoulre.com) | English | 2026-11-16 |
| [ww13.readonepiece.com](https://ww13.readonepiece.com) | English | 2026-11-16 |
| [ww4.readsololeveling.org](https://ww4.readsololeveling.org/manga/solo-leveling-ragnarok/) | English | 2026-11-11 |
| [ww5.blmangafree.com](https://ww5.blmangafree.com) | English, single series | 2026-11-11 |
| [ww6.readchainsawman.com](https://ww6.readchainsawman.com) | English | 2026-11-23 |
| [ww6.readjujutsukaisen.com](https://ww6.readjujutsukaisen.com/manga/jujutsu-kaisen/) | English | 2026-11-23 |
| [ww6.readkingdom.com](https://ww6.readkingdom.com) | English | 2026-11-23 |
| [ww7.readopm.com](https://ww7.readopm.com/manga/one-punch-man/) | English | 2026-11-11 |
| [ww8.read7deadlysins.com](https://ww8.read7deadlysins.com) | English | 2026-11-23 |
| [ww9.readfairytail.com](https://ww9.readfairytail.com) | English | 2026-11-23 |
| [www2.gachiakutas.com](https://www2.gachiakutas.com) | English, single series | 2026-11-11 |
| [xn--l3c0azab5a2gta.com](https://www.xn--l3c0azab5a2gta.com) | Thai | 2026-11-16 |
| [xsano-manga.com](https://www.xsano-manga.com) | Arabic | 2026-11-23 |
| [yakshacomics.com](https://yakshacomics.com) | English | 2026-11-16 |
| [yaoiflix.fit](https://yaoiflix.fit) | Turkish, mostly NSFW | 2026-11-16 |
| [yaoihub.org](https://yaoihub.org) | English, mostly NSFW | 2026-11-16 |
| [yaoiscan.me](https://yaoiscan.me) | English, NSFW | 2026-11-23 |
| [yaoiscan.org](https://yaoiscan.org) | English, mostly NSFW, BL | 2026-11-16 |
| [yokai-team.blogspot.com](https://yokai-team.blogspot.com) | Arabic | 2026-11-25 |
| [younganimal.com](https://younganimal.com) | Japanese, Young Animal (Comici) | 2026-11-23 |
| [youngchampion.jp](https://youngchampion.jp) | Japanese (Comici) | 2026-11-23 |
| [yuri-online.com](https://yuri-online.com) | Spanish, mostly NSFW | 2026-11-16 |
| [yurilab.top](https://yurilab.top) | Indonesian, yuri, some NSFW | 2026-11-16 |
| [yurimoonsub.blogspot.com](https://yurimoonsub.blogspot.com) | Arabic, mostly NSFW | 2026-11-25 |
| [yurionair.top](https://yurionair.top) | Yuri Verso, Portuguese, yuri, some NSFW | 2026-11-16 |
| [zenithscans.com](https://zenithscans.com) | Turkish | ❌ failing since 2026-11-23 (checked 2026-11-25) |
| [zinmanga.net](https://www.zinmanga.net) | ZinManga, English | 2026-11-16 |
| [zonatmo.org](https://zonatmo.org) | TuMangaOnline, former zonatmo.com | 2026-11-16 |

Verified weekly by CI. Sites marked `*` open a browser window, so they are checked by hand and their dates are older. `—` means not checked yet.

</details>

> [!NOTE]
> **Sites marked with `*` need a Chromium-based browser** (Google Chrome,
> Chromium, Brave or Edge). They either render with JavaScript or sit behind a
> Cloudflare challenge. Manga Downloader launches the browser itself and only
> shows a window when a site needs one, remembering the sites that
> needed one until it exits. `--browser-visible` shows it for every site, which
> is rarely needed.

Other sites built on the same platforms often work too, even when they aren't
listed. If one doesn't, [open an issue][issues] or send a pull request.

## Troubleshooting

The [FAQ on the website][faq] has the answers (also in Spanish, Catalan and Japanese):

- [Windows says "Windows protected your PC"][faq-smartscreen]
- [macOS won't open it][faq-gatekeeper]
- [Double-clicking `manga-downloader.exe` just flashes a window][faq-exe]
- [The newest chapter downloads empty or is missing][faq-paywall]
- [Old chapters are missing, or a warning says the range starts before the first chapter][faq-old]
- [A browser window pops up][faq-browser]
- [No browser found][faq-nobrowser]
- [Pages don't show up on my e-reader][faq-ereader]
- [My site isn't supported][faq-unsupported]

## Building from source

The command line tool needs Go 1.26 or newer (`go.mod`):

~~~bash
make install   # download dependencies
make build     # test and build ./manga-downloader
~~~

The desktop app is a [Wails](https://wails.io) app: a Go program showing a
React frontend in the system's webview. It needs Go 1.26 or newer (`gui/go.mod`),
Node 22, pnpm 12, and on Linux the WebKitGTK 4.1 and GTK 3 development headers
(`libgtk-3-dev libwebkit2gtk-4.1-dev` on Debian and Ubuntu):

~~~bash
make build/gui         # build ./manga-downloader-gui (frontend first)
make build/gui/linux   # the release's wails build into gui/build/bin/ (also /windows, /darwin)
make test/gui          # the GUI's vet, race tests and frontend tests
make install/wails     # the wails CLI the release builds with
~~~

## A note on AI

AI tools have changed what this project can be. For years, manga-downloader
gained maybe one site a year. With AI, it grew a desktop app and went from 10
supported sites to over 500 in a matter of days.

It isn't "vibe coded", though. Its foundations, the way sites are plugged in
and how chapters are fetched and packed, are the ones I wrote by hand long
before AI was involved, and the architecture is still my call. I review every
change before it lands. Each change also has to pass the tests, and a site has
to pass a real download checked with `verify-cbz`. Contributions go through the
same review: pull requests are judged on design, correctness and evidence,
whoever or whatever wrote them.

Responsible use of AI is welcome. The repository ships an `AGENTS.md` and
skills for coding agents, so an agent working on it starts from the project's
conventions.

Everyone taking part in the project is expected to follow its
[Code of Conduct](./CODE_OF_CONDUCT.md).

## Star history

<div align="center">

<a href="https://github.com/elboletaire/manga-downloader/stargazers">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/elboletaire/manga-downloader/stats-data/elboletaire/manga-downloader/charts/stars-dark.svg">
    <img alt="Star history chart" src="https://raw.githubusercontent.com/elboletaire/manga-downloader/stats-data/elboletaire/manga-downloader/charts/stars-light.svg">
  </picture>
</a>

</div>

[downloads badge]: https://raw.githubusercontent.com/elboletaire/manga-downloader/stats-data/elboletaire/manga-downloader/badges/downloads.svg
[app downloads badge]: https://raw.githubusercontent.com/elboletaire/manga-downloader/stats-data/elboletaire/manga-downloader/badges/downloads-app.svg
[cli downloads badge]: https://raw.githubusercontent.com/elboletaire/manga-downloader/stats-data/elboletaire/manga-downloader/badges/downloads-cli.svg
[stars badge]: https://raw.githubusercontent.com/elboletaire/manga-downloader/stats-data/elboletaire/manga-downloader/badges/stars.svg
[release badge]: https://img.shields.io/github/release/elboletaire/manga-downloader.svg
[pulls badge]: https://raw.githubusercontent.com/elboletaire/manga-downloader/stats-data/elboletaire/manga-downloader/badges/docker-pulls.svg
[license badge]: https://img.shields.io/github/license/elboletaire/manga-downloader?color=green
[tests badge]: https://github.com/elboletaire/manga-downloader/actions/workflows/test.yaml/badge.svg?branch=master
[tests]: https://github.com/elboletaire/manga-downloader/actions/workflows/test.yaml

[license]: ./LICENSE
[releases]: https://github.com/elboletaire/manga-downloader/releases
[stargazers]: https://github.com/elboletaire/manga-downloader/stargazers
[website]: https://mangadownloader.cat
[faq]: https://mangadownloader.cat/#faq
[faq-smartscreen]: https://mangadownloader.cat/#faq-smartscreen
[faq-gatekeeper]: https://mangadownloader.cat/#faq-gatekeeper
[faq-exe]: https://mangadownloader.cat/#faq-exe
[faq-paywall]: https://mangadownloader.cat/#faq-paywall
[faq-old]: https://mangadownloader.cat/#faq-old
[faq-browser]: https://mangadownloader.cat/#faq-browser
[faq-nobrowser]: https://mangadownloader.cat/#faq-nobrowser
[faq-ereader]: https://mangadownloader.cat/#faq-ereader
[faq-unsupported]: https://mangadownloader.cat/#faq-unsupported
[homebrew]: https://brew.sh
[issues]: https://github.com/elboletaire/manga-downloader/issues
[docker hub]: https://hub.docker.com/r/elboletaire/manga-downloader
