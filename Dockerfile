FROM golang:1.22-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/

RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /notionchat ./cmd/notionchat

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    chromium \
    ca-certificates \
    fonts-liberation \
    xvfb \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -r notionchat -g 10001 \
    && useradd -r -g notionchat -u 10001 -d /app notionchat \
    && mkdir -p /app/data /app/threads /app/data/browser-profile \
    && chown -R notionchat:notionchat /app

WORKDIR /app

COPY --from=builder --chown=notionchat:notionchat /notionchat /app/notionchat

USER notionchat

ENV NOTIONCHAT_HOST=0.0.0.0
ENV NOTIONCHAT_PORT=8787
ENV NOTIONCHAT_SESSION_FILE=/app/data/session.json
ENV NOTIONCHAT_ACCOUNT=/app/data/notion_account.json
ENV NOTIONCHAT_THREADS_DIR=/app/threads
ENV NOTION_BROWSER_CHROMIUM_PATH=/usr/bin/chromium
ENV NOTION_BROWSER_NO_SANDBOX=true
ENV NOTION_BROWSER_MODE=headless
ENV NOTION_BROWSER_PROFILE_DIR=/app/data/browser-profile

EXPOSE 8787

VOLUME ["/app/data", "/app/threads"]

CMD ["/app/notionchat"]