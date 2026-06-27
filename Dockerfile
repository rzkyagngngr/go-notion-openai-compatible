FROM golang:1.22-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/

RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /notionchat ./cmd/notionchat

FROM alpine:3.20

RUN apk add --no-cache \
    chromium \
    nss \
    freetype \
    harfbuzz \
    ca-certificates \
    tzdata \
    xvfb-run \
    su-exec \
    && addgroup -g 10001 -S notionchat \
    && adduser -u 10001 -S notionchat -G notionchat \
    && mkdir -p /app/data /app/threads /app/data/browser-profile \
    && chown -R notionchat:notionchat /app

WORKDIR /app

COPY --from=builder --chown=notionchat:notionchat /notionchat /app/notionchat
COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

ENV NOTIONCHAT_HOST=0.0.0.0
ENV NOTIONCHAT_PORT=8787
ENV NOTIONCHAT_SESSION_FILE=/app/data/session.json
ENV NOTIONCHAT_ACCOUNT=/app/data/notion_account.json
ENV NOTIONCHAT_THREADS_DIR=/app/threads
ENV NOTION_BROWSER_CHROMIUM_PATH=/usr/bin/chromium-browser
ENV NOTION_BROWSER_NO_SANDBOX=true
ENV NOTION_BROWSER_MODE=headless
ENV NOTION_BROWSER_PROFILE_DIR=/app/data/browser-profile

EXPOSE 8787

VOLUME ["/app/data", "/app/threads"]

ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["/app/notionchat"]