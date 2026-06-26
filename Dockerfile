FROM golang:1.22-bookworm AS builder

WORKDIR /src
RUN apt-get update && apt-get install -y --no-install-recommends gcc libc6-dev git \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/

RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o /notionchat ./cmd/notionchat

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
RUN mkdir -p /app/data /app/threads

COPY --from=builder /notionchat /app/notionchat

ENV NOTIONCHAT_HOST=0.0.0.0
ENV NOTIONCHAT_PORT=8787
ENV NOTIONCHAT_SESSION_FILE=/app/data/session.json
ENV NOTIONCHAT_ACCOUNT=/app/data/notion_account.json
ENV NOTIONCHAT_THREADS_DIR=/app/threads

EXPOSE 8787

VOLUME ["/app/data", "/app/threads"]

CMD ["/app/notionchat"]