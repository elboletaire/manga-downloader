# Homebrew

Manga Downloader is installed with Homebrew from a tap of its own,
[elboletaire/homebrew-tap](https://github.com/elboletaire/homebrew-tap):

```sh
brew install elboletaire/tap/manga-downloader              # the command line tool
brew install --cask elboletaire/tap/manga-downloader       # the desktop app (macOS)
```

`tap/` here is that repository's content and the source of truth for the
formula and the cask. The tap only ever gets copies of them. The tap's own
README is shared with Fastlight and is edited in the tap by hand.

| Path | What |
|---|---|
| `tap/Formula/manga-downloader.rb` | The CLI formula. `version`, `url` and `sha256` name the last release this copy was checked against; the release workflow sets them in the tap. |
| `tap/Casks/manga-downloader.rb` | The desktop app cask, same idea. Its `binary` stanza puts `manga-downloader-gui` on `PATH`, where `manga-downloader ui` looks for it, and it depends on the formula, since `ui` is the CLI's. |
| `../../scripts/update-homebrew.sh` | Sets `version` and every `sha256` in a formula or cask, from the release assets in a directory. |

## Why a tap, not homebrew-core or homebrew/cask

Neither takes the project yet, and a tap has no such requirements. The
desktop app is also ad-hoc signed and not notarized, which homebrew/cask
doesn't accept: since September 2026 casks that fail Gatekeeper are disabled
([Acceptable Casks](https://docs.brew.sh/Acceptable-Casks)).

Homebrew still quarantines the cask's download, so macOS asks for **Open
Anyway** on the first launch, which the cask's caveats explain. The formula
installs a plain binary and isn't quarantined.

## One-time setup

1. The tap repository, `elboletaire/homebrew-tap`, already exists (it holds
   Fastlight's cask too). Its default branch is `main`.

2. The token: the same fine-grained PAT Fastlight uses, with Contents read and
   write on `elboletaire/homebrew-tap` only. Store it as the repository secret
   `HOMEBREW_TAP_TOKEN`, reusing the value:

   ```sh
   gh secret set HOMEBREW_TAP_TOKEN --repo elboletaire/manga-downloader
   ```

3. Check that both install: `brew install elboletaire/tap/manga-downloader` and
   `brew install --cask elboletaire/tap/manga-downloader`.

Without the secret, the `homebrew` jobs of the two release workflows only leave
a notice and the tap stays where it is.

## Every release

After `release.yaml` and `release-gui.yaml` publish a release, their
`homebrew` jobs:

1. check out this repository at the tag (`packaging/homebrew` and `scripts`
   only) and the tap with the token (neither keeps credentials in `.git/config`),
2. download the release assets they need: the four CLI archives for the
   formula, the macOS zip for the cask,
3. copy the file from `tap/` over the tap's copy and run
   `scripts/update-homebrew.sh <version> <file> <assets dir>` on it, then
   check it with `ruby -c`,
4. commit `manga-downloader <version>` (formula) or
   `manga-downloader (app) <version>` (cask) and push to `main`, unless nothing
   changed (a re-run). A rejected push is rebased onto the tap and retried.

Both jobs share the `homebrew-tap` concurrency group, so they never push at the
same time. So a change to a formula or cask here reaches the tap with the next
release. To push one sooner, or to fix the tap by hand:

```sh
cp packaging/homebrew/tap/Formula/manga-downloader.rb ../homebrew-tap/Formula/
scripts/update-homebrew.sh 1.11.1 ../homebrew-tap/Formula/manga-downloader.rb <dir with the 1.11.1 assets>
```

The script reads each `url`, substitutes `#{version}`, and takes the sha256 of
the asset with that name from the directory. It fails loudly when an asset is
missing or the file doesn't have the shape it expects.

## Checking

With the assets of a release in a directory:

```sh
scripts/update-homebrew.sh 1.11.1 packaging/homebrew/tap/Formula/manga-downloader.rb ~/Downloads/assets
ruby -c packaging/homebrew/tap/Formula/manga-downloader.rb
```

In a scratch Homebrew (or the `homebrew/brew` image), with the files in a local
tap made by `brew tap-new` in a git repository: `brew style`,
`brew audit --strict --formula` (or `--cask`), `brew install` and `brew test`.
The cask can only be installed on macOS.
