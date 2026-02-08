# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o vpn-bot ./cmd/bot

# Final stage
FROM alpine:3.19

WORKDIR /app

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates tzdata

# Copy binary from builder
COPY --from=builder /app/vpn-bot .

# Create migrations directory (will be mounted as volume)
RUN mkdir -p /app/db/migrations

# Run
CMD ["./vpn-bot", "-config", "/app/config.yaml", "-migrations", "/app/db/migrations"]
