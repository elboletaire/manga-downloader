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

grabber: grabber/inmanga grabber/mangadex grabber/mangabats grabber/qimanga grabber/tcb grabber/html

grabber/inmanga:
	go run . https://inmanga.com/ver/manga/One-Piece/dfc7ecb5-e9b3-4aa5-a61b-a498993cd935 1187

grabber/mangadex:
	go run . https://mangadex.org/title/a1c7c817-4e59-43b7-9365-09675a149a6f/one-piece --language es 373 --bundle

grabber/mangabats:
	go run . https://www.mangabats.com/manga/after-the-possessor-left 1

grabber/qimanga:
	go run . https://qimanga.com/series/4190634673-eleceed 2

grabber/tcb:
	go run . https://lhtranslation.net/manga/gaikotsu-kishi-sama-tadaima-isekai-e-o-dekake-chuu/ 71

grabber/html:
	go run . https://tcbonepiecechapters.com/mangas/5/one-piece 1100
	go run . https://asurascans.com/comics/absolute-regression-f886a8af 1
