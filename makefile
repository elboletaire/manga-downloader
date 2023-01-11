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
	rm -fv ./proto/*.go
	rm -fv ./manga-downloader ./manga-downloader.exe

download:
	go mod download

install: download build/models

build: build/models build/linux

build/linux:
	go build -o manga-downloader ${GOFLAGS} .

build/models:
	protoc --go_opt=paths=source_relative -I=./proto --go_out=./proto ./proto/*.proto

build-win:
	go build -o manga-downloader.exe ${GOFLAGS} .
