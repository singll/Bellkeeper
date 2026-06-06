# ==============================================================================
#  Bellkeeper Multi-stage Dockerfile
#  Stage 1: Build frontend (SolidJS + Vite)
#  Stage 2: Build backend (Go)
#  Stage 3: Runtime (Alpine)
# ==============================================================================

# --- Stage 1: Frontend Build ---
FROM node:20-alpine AS frontend-builder

WORKDIR /app/web

# Install pnpm (pin version to avoid re-download)
RUN corepack enable && corepack prepare pnpm@10 --activate

# Copy package files first for layer cache
COPY web/package.json web/pnpm-lock.yaml* ./

# Install dependencies
RUN pnpm install --frozen-lockfile || pnpm install

# Copy frontend source and build
COPY web/ ./
RUN pnpm build

# --- Stage 2: Backend Build ---
FROM golang:1.25-alpine AS backend-builder

WORKDIR /app

ENV GOTOOLCHAIN=auto
# 国内 Go 模块代理：默认 proxy.golang.org 经本网络不可达(EOF)，goproxy.cn 直连稳定
ENV GOPROXY=https://goproxy.cn,direct

# Copy go mod files first for layer cache
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Copy frontend dist from frontend-builder
COPY --from=frontend-builder /app/web/dist ./web/dist

# Build binary with embedded frontend
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o bellkeeper ./cmd/bellkeeper

# --- Stage 3: Runtime ---
FROM alpine:3.21

WORKDIR /app

# Install runtime dependencies (trafilatura pinned to avoid surprises)
RUN apk add --no-cache ca-certificates tzdata wget python3 py3-pip && \
    pip3 install --break-system-packages --no-cache-dir trafilatura==2.0.0

# Copy binary from builder
COPY --from=backend-builder /app/bellkeeper .
COPY --from=backend-builder /app/web/dist ./web/dist

# Copy trafilatura wrapper script (trafilatura 2.0 removed __main__.py)
COPY scripts/trafilatura_extract.py /app/scripts/trafilatura_extract.py

# Copy default config（含 config/pkb 提示词包/领域配置，pkb-curate 运行依赖）
COPY config/ ./config/

# Create non-root user
RUN adduser -D -g '' bellkeeper && \
    chown -R bellkeeper:bellkeeper /app

USER bellkeeper

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD wget -qO /dev/null http://127.0.0.1:8080/api/health || exit 1

ENTRYPOINT ["./bellkeeper"]
CMD ["serve"]
