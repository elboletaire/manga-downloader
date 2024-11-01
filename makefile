.PHONY: *

ifdef CI_COMMIT_REF_NAME
	BRANCH_OR_TAG := $(CI_COMMIT_REF_NAME)
else
	BRANCH_OR_TAG := develop
endif

VERSION := $(shell git rev-parse --short HEAD)
GOLDFLAGS += -X 'github.com/voxelost/manga-downloader/cmd.Version=$(VERSION)'
GOLDFLAGS += -X 'github.com/voxelost/manga-downloader/cmd.Tag=$(BRANCH_OR_TAG)'
GOFLAGS = -ldflags="$(GOLDFLAGS)"

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
	go test -v ./... -race -shuffle=on

