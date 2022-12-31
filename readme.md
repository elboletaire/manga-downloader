Manga Downloader
================

This tool downloads mangas from websites like mangadex and stores them into cbz files, so you can read them with your
favorite ereader or reading app.

Usage
-----

The program only accepts two params for now:

~~~bash
manga-downloader [URL] [range]
~~~

The URL must be a series index file, and the range allows you setting pages by ranges (i.e. 1,3,5-10):

~~~bash
manga-downloader https://inmanga.com/ver/manga/One-Piece/dfc7ecb5-e9b3-4aa5-a61b-a498993cd935 1-50
# This would download One Piece chapters 1 to 50 into our current folder
~~~

In some sites, like mangadex, it may find multiple results for the same chapter, given the different languages it's
translated to. In these cases it will download by default all the different files, but you can force a single language
to be downloaded by using `--language`:

~~~bash
manga-downloader --language es https://mangadex.org/title/a1c7c817-4e59-43b7-9365-09675a149a6f/one-piece 1-10
# would download One Piece chapters 1 to 10 in spanish
~~~

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

If you want the binary to be accessible from your terminal in whatever path you might be, you should ensure to place the
binary on a `PATH` defined folder (or add that path to your `PATH` env var).

Places where you can put the binary and have it accessible system-wide:

- Linux and Mac: `/usr/local/bin`
- Windows: `C:\Windows\System32`


Todos
-----

- Parallel download of pages
- Add more sites
  - [ ] https://manganelo.com
  - [ ] https://chapmanganato.com (related to manganelo, similar format)
  - [ ] https://manganelo.tv (same format than chapmanganato.com)
  - [ ] https://mangakakalot.com (same as manganelo)
  - [ ] https://www.tcbscans.net/
  - [ ] Mangadex (needs parsing external sites and properly recognising those links)
  - [ ] https://mangaplus.shueisha.co.jp (one of those external sites required by Mangadex)
- Better error handling
- Bundling chapters into a single CBZ file rather than in separated files (via bool flag like `--bundle`)


License
-------

All the code contained in this repo is licensed under the [GNU Affero General Public License v3.0][license]

    Manga Downloader GO cli
    Copyright (C) 2023 Òscar Casajuana Alonso

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

[license]: ./LICENSE
[releases]: https://github.com/elboletaire/manga-downloader/releases
