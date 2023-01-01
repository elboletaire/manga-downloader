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
	rm -fv ./manga-downloader ./manga-downloader.exe

install:
	go mod download

build:
	go build -o manga-downloader ${GOFLAGS} .

build-win:
	go build -o manga-downloader.exe ${GOFLAGS} .
