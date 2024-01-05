# Build from a golang based image
FROM golang:latest as builder

LABEL maintainer="Òscar Casajuana <elboletaire@underave.net>"

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN make build/unix

# Start a new stage from scratch
FROM alpine:latest

WORKDIR /app

RUN mkdir /downloads

# Copy the pre-built binary file from the previous stage
COPY --from=builder /app/manga-downloader .

# Set manga-downloader as the entrypoint
ENTRYPOINT ["./manga-downloader", "-o", "/downloads"]
