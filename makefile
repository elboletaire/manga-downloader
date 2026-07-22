ifdef CI_COMMIT_REF_NAME
BRANCH_OR_TAG := $(CI_COMMIT_REF_NAME)
else
BRANCH_OR_TAG := develop
endif

VERSION := $(shell git rev-parse --short HEAD)
GOLDFLAGS += -X 'github.com/elboletaire/manga-downloader/cmd.Version=$(VERSION)'
GOLDFLAGS += -X 'github.com/elboletaire/manga-downloader/cmd.Tag=$(BRANCH_OR_TAG)'
GOFLAGS = -ldflags="$(GOLDFLAGS)"
RICHGO := $(shell command -v richgo 2> /dev/null)

clean:
	@rm -fv ./manga-downloader* *.cbz

install:
	go mod download

build: clean test build/unix

build/all: clean test build/unix build/win

build/unix:
	CGO_ENABLED=0 go build -o manga-downloader ${GOFLAGS} .

build/win:
	GOOS=windows go build -o manga-downloader.exe ${GOFLAGS} .

test:
ifdef RICHGO
	richgo test -v ./...
else
	go test -v ./...
endif

grabber: grabber/inmanga grabber/mangadex grabber/mangabats grabber/mangafire grabber/mangak grabber/qimanga grabber/tcb grabber/flamecomics grabber/weebcentral grabber/leercapitulo grabber/vortexscans grabber/html

grabber/inmanga:
	go run . https://inmanga.com/ver/manga/One-Piece/dfc7ecb5-e9b3-4aa5-a61b-a498993cd935 1187

# note: use a language without an official publisher (i.e. not es/en/fr...):
# licensed translations get replaced by pageless mangaplus stubs on mangadex
grabber/mangadex:
	go run . https://mangadex.org/title/a1c7c817-4e59-43b7-9365-09675a149a6f/one-piece --language ca 1187 --bundle

grabber/mangabats:
	go run . https://www.mangabats.com/manga/after-the-possessor-left 1

grabber/mangafire:
	go run . https://mangafire.to/title/dkw-one-piece 1187

grabber/mangak:
	go run . https://mangak.io/a-baby-cat-who-commands-the-dog-clan 30

grabber/qimanga:
	go run . https://qimanga.com/series/4190634673-eleceed 2

grabber/tcb:
	go run . https://lhtranslation.net/manga/gaikotsu-kishi-sama-tadaima-isekai-e-o-dekake-chuu/ 71

grabber/flamecomics:
	go run . https://flamecomics.xyz/series/154 104

grabber/weebcentral:
	go run . https://weebcentral.com/series/01J76XYDXH7KT6AABVG3JAT3ZP/Shangri-La-Frontier 274

# uses a real (headless) browser just to toggle the reader's "load all pages"
# client-side preference, no --browser-visible needed (no cloudflare here)
grabber/leercapitulo:
	go run . https://www.leercapitulo.co/manga/0cj9hhn6di/kingdom/ 883

# use a chapter that's not one of the newest few (those can be paywalled
# behind coins/early access) so the smoke test doesn't flake as new chapters
# release
grabber/vortexscans:
	go run . https://vortexscans.org/series/archmage-curriculum 20

# sites needing a real browser: not part of the `grabber` target since they
# open a Chrome window and may require solving an interactive challenge
# (cloudflare). Run them one by one and solve the challenge if prompted.
grabber/browser: grabber/toongod grabber/dragontea grabber/kappabeast grabber/sushiscan grabber/mangakakalot grabber/natomanga grabber/manhuaus

grabber/toongod:
	go run . --browser-visible https://www.toongod.org/webtoon/solo-leveling/ 200

grabber/dragontea:
	go run . --browser-visible https://dragontea.ink/novel/it-all-starts-with-trillions-of-nether-currency/ 290

grabber/kappabeast:
	go run . --browser-visible https://kappabeast.com/series/tekkarian 2

grabber/sushiscan:
	go run . --browser-visible https://sushiscan.net/catalogue/mushoku-tensei/ 17

grabber/mangakakalot:
	go run . --browser-visible https://www.mangakakalot.gg/manga/akuyaku-reijou-kara-no-kareinaru-tenshin-aisare-heroine-anthology-comic 1

grabber/natomanga:
	go run . --browser-visible https://www.natomanga.com/manga/rebirth-from-0-to-1 205.9

grabber/manhuaus:
	go run . --browser-visible https://manhuaus.com/manga/solo-leveling-ragnarok/ 68

grabber/html: grabber/tcbscans grabber/asura grabber/zonatmo grabber/mangapill

grabber/tcbscans:
	go run . https://tcbonepiecechapters.com/mangas/5/one-piece 1100

grabber/asura:
	go run . https://asurascans.com/comics/absolute-regression-f886a8af 1

grabber/zonatmo:
	go run . https://zonatmo.org/library/manga/31322/one-piece 1188

grabber/mangapill:
	go run . https://mangapill.com/manga/2/one-piece 1188
