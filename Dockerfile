FROM golang:1.22-alpine AS builder

WORKDIR /src

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

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/netpulse ./cmd/netpulse


FROM alpine:latest

WORKDIR /app

# pg_dump / psql for backup/restore APIs
RUN apk add --no-cache ca-certificates tzdata postgresql-client && \
    addgroup -S app && adduser -S app -G app

COPY --from=builder /out/netpulse /app/netpulse

EXPOSE 8080
EXPOSE 514/udp

USER app

ENTRYPOINT ["/app/netpulse"]

