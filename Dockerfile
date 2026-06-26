FROM golang:1.22-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/

RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /notionchat ./cmd/notionchat

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata \
	&& mkdir -p /app/data /app/threads

WORKDIR /app

COPY --from=builder /notionchat /app/notionchat

ENV NOTIONCHAT_HOST=0.0.0.0
ENV NOTIONCHAT_PORT=8787
ENV NOTIONCHAT_SESSION_FILE=/app/data/session.json
ENV NOTIONCHAT_ACCOUNT=/app/data/notion_account.json
ENV NOTIONCHAT_THREADS_DIR=/app/threads

EXPOSE 8787

VOLUME ["/app/data", "/app/threads"]

CMD ["/app/notionchat"]