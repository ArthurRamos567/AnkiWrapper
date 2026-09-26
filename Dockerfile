# Stage 1: Build the Go application
FROM golang:alpine AS builder

WORKDIR /build

# Install git and ca-certificates
RUN apk add --no-cache ca-certificates git

# Cache go dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build a fully static binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /build/anki-wrapper ./cmd/anki-wrapper

# Stage 2: Minimal runtime image
FROM alpine:3.21

WORKDIR /app

# Install runtime certificates and timezones
RUN apk add --no-cache ca-certificates tzdata

# Copy binary from builder
COPY --from=builder /build/anki-wrapper /app/anki-wrapper

# Copy Tae Kim grammar guide markdown files
COPY data/knowledge /app/data/knowledge

# Set default environment variables
ENV PORT=8080 \
    DB_PATH=/app/data/app.db \
    LLM_PROVIDER=gemini \
    GEMINI_MODEL=gemini-3.5-flash-lite

EXPOSE 8080

ENTRYPOINT ["/app/anki-wrapper", "serve"]
