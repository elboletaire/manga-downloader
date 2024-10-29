Manga Downloader
================

[![Go Report Card][go report card]][go report]
[![Go Reference][go reference badge]][go reference]
[![GitHub release][release badge]][releases]
[![gitHub downloads]][downloads]
[![Docker Pulls][pulls badge]][docker hub]
[![License][license badge]][license]

This app downloads manga from websites like mangadex and stores them into cbz
files, so you can read them with your favorite ereader or reading app.

![prompt img]

Supported sites
---------------

- [asuratoon.com (Asura Scans)](https://asuratoon.com)
- [chapmanganato.to](https://chapmanganato.to)
- [inmanga.com](https://inmanga.com)
- [LHTranslation](https://lhtranslation.net)
- [lscomic.com](https://lscomic.com/)
- [Manga Monks](https://mangamonks.com)
- [mangabat.com](https://mangabat.com)
- [Mangadex](https://mangadex.org)
- [mangakakalot.com](https://mangakakalot.com)
- [mangakakalot.tv](https://mangakakalot.tv)
- [manganato.com](https://manganato.com)
- [manganelo.com](https://manganelo.com)
- [manganelo.tv](https://manganelo.tv)
- [mangapanda.in](https://mangapanda.in)
- [readmangabat.com](https://readmangabat.com)
- [tcbscans.com](https://tcbscans.com)
- [tcbscans.net](https://www.tcbscans.net)
- [tcbscans.org](https://www.tcbscans.org)

It may support even more sites of which I'm not aware. If you find a site that is not supported, feel free to open a new issue or a PR with the implementation.

Usage
-----

Only one param is required:

~~~bash
manga-downloader [URL]
~~~

The URL must be a series index file (not an individual chapter).

When only specifying the URL, it would ask you if you want to download all
chapters.

> Note: you must specify <kbd>y</kbd> in order to download them, its default
> behavior is set to "no".

You can also specify the range beforehand, the range allows you setting chapters by
ranges (i.e. 1,3,5-10):

~~~bash
manga-downloader https://inmanga.com/ver/manga/One-Piece/dfc7ecb5-e9b3-4aa5-a61b-a498993cd935 1-50
# This would download One Piece chapters 1 to 50 into our current folder
~~~

![download img]

In some sites, like mangadex, it may find multiple results for the same chapter,
given the different languages it's translated to. In these cases, every
coincidence will be downloaded into different files, but you can force a single
language to be downloaded by using `--language`:

~~~bash
manga-downloader --language es https://mangadex.org/title/a1c7c817-4e59-43b7-9365-09675a149a6f/one-piece 1-10
# would download One Piece chapters 1 to 10 in spanish
~~~

Arguments and params are not positional, you can use them in any order:

~~~bash
manga-downloader 1-10 https://mangadex.org/title/a1c7c817-4e59-43b7-9365-09675a149a6f/one-piece --language es
# exactly the same as the previous example, only changing params order
~~~

### Bundling

You can bundle all the downloaded chapters into a single file by using the
`--bundle` arg:

~~~bash
manga-downloader https://inmanga.com/ver/manga/One-Piece/dfc7ecb5-e9b3-4aa5-a61b-a498993cd935 1-8 --bundle
# would download one piece chapters 1 to 8 and bundle them into a single file
~~~

![bundle img]

### Help

Use the `help` command to see all the available options:

~~~bash
manga-downloader help
~~~

![help img]

Installation
------------

First download your desired version from the [releases section][releases].

After you downloaded and unarchived it, you can start using it in that folder:

~~~bash
./manga-downloader URL range
~~~

For Windows users would be:

~~~cmd
.\manga-downloader URL range
~~~

If you want the binary to be accessible from your terminal in whatever path you
might be, you should ensure to place the binary on a `PATH` defined folder (or
add the folder where you downloaded manga-downloader to your `PATH` env var).

Places where you can put the binary and have it accessible system-wide:

- Linux and Mac: `/usr/local/bin`
- Windows: `C:\Windows\System32`

### Windows

So if you're a windows user and place the .exe file inside `C:\Windows\System32`
you'll be able to call the program wherever you want from:

~~~bash
C:\Users\elboletaire\Desktop>manga-downloader https://mangadex.org/title/e7eabe96-aa17-476f-b431-2497d5e9d060/black-clover 1-346
~~~

The above command would download Black Clover chapters 1 to 346 to the Desktop
folder (since that's the current directory).

### Mac

Mac users will need to either add the binary to the unsigned apps whitelist, or
entirely disable Gatekeeper:

~~~bash
sudo spctl --master-disable
~~~

Othwerise you'll see an error because the binary is unsigned.

### Using Docker

You can also use manga-downloader directly via docker like so:

~~~bash
docker run --rm -it -v $PWD:/downloads elboletaire/manga-downloader --help
~~~

Note the `-v $PWD:/downloads` param, that's required in order to get the downloads in your current path.

Star history
------------

[![Stargazers over time](https://starchart.cc/elboletaire/manga-downloader.svg?variant=adaptive)](https://starchart.cc/elboletaire/manga-downloader)

License
-------

All the code contained in this repo is licensed under the
[GNU Affero General Public License v3.0][license]

    Manga Downloader GO cli
    Copyright (C) 2023-2024 Òscar Casajuana Alonso

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

[license]: ./LICENSE
[releases]: https://github.com/elboletaire/manga-downloader/releases
[issues]: https://github.com/elboletaire/manga-downloader/issues
[download img]: https://raw.githubusercontent.com/elboletaire/manga-downloader/master/demos/download.gif
[bundle img]: https://raw.githubusercontent.com/elboletaire/manga-downloader/master/demos/bundle.gif
[help img]: https://raw.githubusercontent.com/elboletaire/manga-downloader/master/demos/help.gif
[prompt img]: https://raw.githubusercontent.com/elboletaire/manga-downloader/master/demos/prompt.gif
[docker hub]: https://hub.docker.com/r/elboletaire/manga-downloader
[downloads]: https://qii404.me/github-release-statistics/?repo=https://github.com/elboletaire/manga-downloader
