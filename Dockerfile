# =============================================================================
# Epay Multi-Stage Dockerfile
# =============================================================================
# Stage 1: Frontend build
# Stage 2: Go build
# Stage 3: Minimal runtime image
# =============================================================================

# -----------------------------------------------------------------------------
# Stage 1: Frontend build
# -----------------------------------------------------------------------------
FROM node:24-alpine AS frontend

WORKDIR /app/frontend

# Copy dependency files first (better caching)
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm ci

# Copy source and build
COPY frontend/ ./
RUN npm run build

# -----------------------------------------------------------------------------
# Stage 2: Go build
# -----------------------------------------------------------------------------
FROM golang:1.26-alpine AS backend

WORKDIR /app

# Copy go mod files first (better caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Copy frontend dist from Stage 1
COPY --from=frontend /app/frontend/dist ./frontend/dist

# Build the binary
RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w" \
    -trimpath \
    -o /app/epay \
    ./cmd/server

# -----------------------------------------------------------------------------
# Stage 3: Runtime
# -----------------------------------------------------------------------------
FROM alpine:3.19

# Install runtime dependencies and create non-root runtime user
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -H app

COPY --from=backend /app/epay /app/epay
COPY --from=frontend /app/frontend/dist /app/frontend/dist
ENV SPA_DIR=/app/frontend/dist
WORKDIR /app
USER app

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget -q -T 5 -O /dev/null http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/epay"]
