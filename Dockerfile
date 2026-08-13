# syntax=docker/dockerfile:1

# ---------- build: static Go binary ----------
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS TARGETARCH VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/audiobookr ./cmd/audiobookr

# ---------- tone: single-binary audiobook tagger ----------
FROM alpine:3.22 AS tone
ARG TARGETARCH
ARG TONE_VERSION=0.2.5
RUN apk add --no-cache wget tar \
 && case "$TARGETARCH" in \
      amd64) TONE_ARCH=x64 ;; \
      arm64) TONE_ARCH=arm64 ;; \
      *) echo "unsupported arch $TARGETARCH" && exit 1 ;; \
    esac \
 && wget -qO /tmp/tone.tar.gz \
      "https://github.com/sandreas/tone/releases/download/v${TONE_VERSION}/tone-${TONE_VERSION}-linux-musl-${TONE_ARCH}.tar.gz" \
 && tar -xzf /tmp/tone.tar.gz -C /tmp \
 && find /tmp -name tone -type f -exec mv {} /usr/local/bin/tone \; \
 && chmod +x /usr/local/bin/tone

# ---------- runtime ----------
FROM alpine:3.22
RUN apk add --no-cache \
      ffmpeg \
      libstdc++ \
      su-exec \
      shadow \
      tini \
      tzdata \
      ca-certificates \
      wget \
 && (apk add --no-cache fdkaac \
     || echo "NOTE: fdkaac unavailable for this arch; the native AAC encoder is used instead")
COPY --from=tone /usr/local/bin/tone /usr/local/bin/tone
COPY --from=build /out/audiobookr /app/audiobookr
COPY docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENV PUID=1000 \
    PGID=1000 \
    TZ=Etc/UTC \
    PORT=8684 \
    CONFIG_DIR=/config \
    INPUT_DIR=/input \
    OUTPUT_DIR=/output

VOLUME ["/config", "/input", "/output"]
EXPOSE 8684

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s \
  CMD wget -qO- "http://127.0.0.1:${PORT}/healthz" || exit 1

ENTRYPOINT ["/sbin/tini", "--", "/entrypoint.sh"]
