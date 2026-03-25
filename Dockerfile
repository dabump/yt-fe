FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git

WORKDIR /build

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o yt-fe .

FROM alpine:3.23

RUN apk add --no-cache ffmpeg python3 py3-pip && \
    pip3 install --no-cache-dir --break-system-packages yt-dlp

WORKDIR /app

COPY --from=builder /build/yt-fe /app/yt-fe

COPY --from=builder /build/templates /app/templates

COPY entrypoint.sh /app/entrypoint.sh

RUN chmod +x /app/entrypoint.sh

RUN mkdir -p /app/video /app/thumbnails /app/metadata

ENTRYPOINT ["/app/entrypoint.sh"]

CMD ["/app/yt-fe"]
