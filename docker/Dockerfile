# ============================================================
# Stage 1: Build the ottie binary
# ============================================================
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git make upx

WORKDIR /src

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN make build && upx --best --lzma build/ottie-linux-*

# ============================================================
# Stage 2: Minimal runtime image
# ============================================================
FROM alpine:3.23

RUN apk add --no-cache ca-certificates tzdata

# Health check (wget is built into busybox/alpine)
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q --spider http://localhost:18790/health || exit 1

# Copy binary
COPY --from=builder /src/build/ottie /usr/local/bin/ottie

# Create non-root user and group
RUN addgroup -g 1000 ottie && \
    adduser -D -u 1000 -G ottie ottie

# Switch to non-root user
USER ottie

# Run onboard to create initial directories and config
RUN /usr/local/bin/ottie onboard

ENTRYPOINT ["ottie"]
CMD ["gateway"]
