# ===== Stage 1: 构建前端 =====
FROM docker.linkos.org/library/node:20-alpine AS web-builder
WORKDIR /app/web

# 先复制依赖文件，利用 Docker 缓存
COPY web/package.json web/package-lock.json* ./
# 使用淘宝镜像加速（国内网络环境）
RUN npm config set registry https://registry.npmmirror.com \
    && npm ci || npm install

# 复制前端源码并构建
COPY web/ ./
RUN npm run build

# ===== Stage 2: 构建后端 =====
FROM docker.linkos.org/library/golang:1.25-alpine AS go-builder
WORKDIR /app

# 换阿里云镜像源加速 apk 安装
RUN sed -i 's#dl-cdn.alpinelinux.org#mirrors.aliyun.com#g' /etc/apk/repositories

# 安装编译 SQLite 所需的 gcc/musl-dev
RUN apk add --no-cache gcc musl-dev

# 复制 vendor 目录，使用本地依赖
COPY go.mod go.sum vendor/ ./
ENV GOFLAGS=-mod=vendor

# 复制源码并编译（CGO_ENABLED=1 以支持 SQLite）
COPY . .
COPY --from=web-builder /app/web/dist ./web/dist
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o /out/opennexus ./cmd/server

# ===== Stage 3: 运行时 =====
# 使用包含 npm/npx 的 node 镜像，因为 agents 通过 npx 调用 claude-agent-acp
FROM docker.linkos.org/library/node:20-alpine AS runtime
WORKDIR /app

# 换阿里云镜像源加速 apk 安装
RUN sed -i 's#dl-cdn.alpinelinux.org#mirrors.aliyun.com#g' /etc/apk/repositories

# 安装运行时依赖：sqlite、ca-cert、git（部分 agent 可能需要）
RUN apk add --no-cache ca-certificates sqlite-libs musl-locales tzdata git \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo "Asia/Shanghai" > /etc/timezone

# 复制后端二进制
COPY --from=go-builder /out/opennexus /app/opennexus

# 复制前端构建产物（后端 release 模式会服务这些静态文件）
COPY --from=web-builder /app/web/dist /app/web/dist

# 复制默认配置
COPY config.yaml /app/config.yaml

# 数据持久化目录
RUN mkdir -p /app/data/session
VOLUME ["/app/data"]

# 显式指定 npm 缓存目录，便于通过 docker volume 持久化。
# npx 调用 claude-agent-acp 时下载的包会缓存在 /root/.npm/_npx，
# 持久化后容器重启无需重新从网络下载。
ENV NPM_CONFIG_CACHE=/root/.npm \
    SERVER_MODE=release \
    SERVER_PORT=8080 \
    WEB_DIST=/app/web/dist \
    DATABASE_PATH=/app/data/opennexus.db \
    AGENTS_WORKSPACE_SESSION_DIR=/app/data/session \
    NODE_ENV=production

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/health >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/app/opennexus"]
