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
	go build -o manga-downloader ${GOFLAGS} .

build/win:
	go build -o manga-downloader.exe ${GOFLAGS} .

test:
ifdef RICHGO
	richgo test -v ./...
else
	go test -v ./...
endif

grabber: grabber/manganelo grabber/inmanga grabber/mangadex grabber/tcb grabber/html

grabber/manganelo:
	go run . https://mangakakalot.com/manga/vd921334 7
	go run . https://ww5.manganelo.tv/manga/manga-aa951409 3
	go run . https://readmangabat.com/read-ov357862 23
	go run . https://chapmanganato.com/manga-aa951409 50
	go run . https://h.mangabat.com/read-tc397521 5

grabber/inmanga:
	go run . https://inmanga.com/ver/manga/One-Piece/dfc7ecb5-e9b3-4aa5-a61b-a498993cd935 1

grabber/mangadex:
	go run . https://mangadex.org/title/a1c7c817-4e59-43b7-9365-09675a149a6f/one-piece --language es 1-4 --bundle

grabber/tcb:
	go run . https://www.tcbscans.net/manga/one-piece/ 5
	go run . https://lscomic.com/manga/peerless-dad/ 285

grabber/html:
	go run . https://tcbscans.com/mangas/5/one-piece 1100
	go run . https://asuratoon.com/manga/0435219386-return-of-the-sss-class-ranker/ 85
	go run . https://mangajar.pro/manga/haite-kudasai-takamine-san 43
