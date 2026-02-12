# ==============================================================================
#  Bellkeeper Multi-stage Dockerfile
#  Stage 1: Build frontend (SolidJS + Vite)
#  Stage 2: Build backend (Go)
#  Stage 3: Runtime (Alpine)
# ==============================================================================

# --- Stage 1: Frontend Build ---
FROM node:20-alpine AS frontend-builder

WORKDIR /app/web

# Install pnpm
RUN corepack enable && corepack prepare pnpm@latest --activate

# Copy package files
COPY web/package.json web/pnpm-lock.yaml* ./

# Install dependencies
RUN pnpm install --frozen-lockfile || pnpm install

# Copy frontend source
COPY web/ ./

# Build frontend
RUN pnpm build

# --- Stage 2: Backend Build ---
FROM golang:1.22-alpine AS backend-builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache gcc musl-dev

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Copy frontend dist from frontend-builder
COPY --from=frontend-builder /app/web/dist ./web/dist

# Build binary with embedded frontend
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o bellkeeper ./cmd/bellkeeper

# --- Stage 3: Runtime ---
FROM alpine:3.19

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata wget

# Copy binary from builder
COPY --from=backend-builder /app/bellkeeper .
COPY --from=backend-builder /app/web/dist ./web/dist

# Copy default config (will be overridden by env vars)
COPY config/bellkeeper.yaml ./config/

# Create non-root user
RUN adduser -D -g '' bellkeeper && \
    chown -R bellkeeper:bellkeeper /app

USER bellkeeper

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/health || exit 1

# Run
ENTRYPOINT ["./bellkeeper"]
CMD ["serve"]
