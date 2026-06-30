FROM golang:1.22-alpine AS builder

WORKDIR /src

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

# Required toolchain bits
RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

# Copy backend sources
COPY cmd ./cmd
COPY internal ./internal
COPY assets ./assets

# Copy built frontend assets for go:embed path.
# NOTE: Build web/dist before docker build, otherwise this step fails.
COPY web/dist ./cmd/netpulse/web/dist

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags "-s -w -X netpulse/internal/api.Version=${VERSION} -X netpulse/internal/api.Commit=${COMMIT} -X netpulse/internal/api.BuildTime=${BUILD_TIME}" \
    -o /out/netpulse ./cmd/netpulse


FROM alpine:latest

WORKDIR /app

# pg_dump / psql for backup/restore APIs; curl for container healthcheck.
RUN apk add --no-cache ca-certificates tzdata postgresql-client curl && \
    addgroup -S app && adduser -S app -G app

COPY --from=builder /out/netpulse /app/netpulse

EXPOSE 8080
EXPOSE 514/udp
EXPOSE 9162/udp

USER app

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD curl -fsS http://127.0.0.1:8080/api/healthz >/dev/null || exit 1

ENTRYPOINT ["/app/netpulse"]
