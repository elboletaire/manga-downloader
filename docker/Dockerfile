# Build from a golang based image
FROM --platform=$BUILDPLATFORM golang:latest AS builder

LABEL maintainer="Òscar Casajuana <elboletaire@underave.net>"

ARG CI_COMMIT_REF_NAME
ENV CI_COMMIT_REF_NAME ${CI_COMMIT_REF_NAME:-latest}

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN GOOS=$TARGETOS GOARCH=$TARGETARCH make build/unix

# Start a new stage from scratch
FROM alpine:latest

WORKDIR /app

RUN mkdir /downloads

# Copy the pre-built binary file from the previous stage
COPY --from=builder /app/manga-downloader /usr/bin/manga-downloader

COPY /docker/entrypoint.sh /usr/bin/entrypoint.sh
RUN chmod +x /usr/bin/entrypoint.sh

# Set manga-downloader as the entrypoint
ENTRYPOINT ["/usr/bin/entrypoint.sh", "-o", "/downloads"]
