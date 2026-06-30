VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags="-s -w -X main.version=$(VERSION)"

.PHONY: dev backend frontend build run test clean release release-dry docker-build docker-up docker-down docker-logs

# 一键启动前后端开发服务器
# 用法: make dev
dev: backend-stop
	@trap 'kill 0' EXIT; \
	$(MAKE) backend & \
	$(MAKE) frontend & \
	wait

# 启动后端 (Go, :8080)
backend:
	@echo "==> 启动后端 http://localhost:8080"
	@go run ./cmd/server

# 启动前端 (Vite, :3000)
frontend:
	@echo "==> 启动前端 http://localhost:3000"
	@cd web && npm run dev

# 尝试停止已运行的后端进程（避免端口占用）
backend-stop:
	@-lsof -ti:8080 | xargs kill -9 2>/dev/null || true
	@-lsof -ti:3000 | xargs kill -9 2>/dev/null || true

# 构建前后端
build:
	@echo "==> 构建前端"
	@cd web && npm run build
	@echo "==> 构建后端 ($(VERSION))"
	@CGO_ENABLED=1 go build $(LDFLAGS) -o nexusagent ./cmd/server

# 桌面模式：构建后以 release 模式启动并自动打开浏览器
# 用法: make run-desktop
run-desktop: build
	@echo "==> 桌面模式启动 http://localhost:8080"
	@SERVER_MODE=release ./nexusagent --open

# 单端口运行：先构建前端，再以 release 模式启动后端（前端 + API 同端口）
# 用法: make run
run: build
	@echo "==> 单端口启动 http://localhost:8080"
	@SERVER_MODE=release ./nexusagent

# 生产环境发布构建：跨平台编译
# 用法: make release
release:
	@echo "==> 构建前端"
	@cd web && npm run build
	@echo "==> 交叉编译"
	@mkdir -p dist
	@CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o "dist/nexusagent-darwin-amd64" ./cmd/server
	@CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o "dist/nexusagent-darwin-arm64" ./cmd/server
	@CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o "dist/nexusagent-linux-amd64" ./cmd/server
	@CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o "dist/nexusagent-linux-arm64" ./cmd/server
	@CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -tags=sqlite_nocgo $(LDFLAGS) -o "dist/nexusagent-windows-amd64.exe" ./cmd/server
	@echo "==> 打包"
	cd dist && tar czf nexusagent-darwin-amd64.tar.gz nexusagent-darwin-amd64 && tar czf nexusagent-darwin-arm64.tar.gz nexusagent-darwin-arm64 && tar czf nexusagent-linux-amd64.tar.gz nexusagent-linux-amd64 && tar czf nexusagent-linux-arm64.tar.gz nexusagent-linux-arm64 && zip nexusagent-windows-amd64.zip nexusagent-windows-amd64.exe
	@echo "==> 创建 macOS App 包"
	@./scripts/package-darwin.sh dist nexusagent-darwin-arm64 2>/dev/null || echo "⚠️   macOS 打包需在 macOS 上运行"
	@echo "==> 发布文件已生成到 dist/"
	@ls -lh dist/

# 运行后端全部测试
test:
	@go test ./...

# 清理构建产物
clean:
	@-rm -f nexusagent
	@-rm -rf web/dist

# 构建 Docker 镜像（多阶段构建，含前端 + 后端）
# 用法: make docker-build
docker-build:
	@echo "==> 构建 Docker 镜像 nexusagent:latest"
	@docker build -t nexusagent:latest .

# 启动 docker-compose（前台，Ctrl+C 停止）
# 用法: make docker-up
docker-up: docker-build
	@echo "==> 启动容器 http://localhost:8080"
	@docker compose up

# 后台启动 docker-compose
# 用法: make docker-up-d
docker-up-d: docker-build
	@echo "==> 后台启动容器 http://localhost:8080"
	@docker compose up -d
	@docker compose ps

# 停止并清理 docker-compose 容器（保留数据卷）
# 用法: make docker-down
docker-down:
	@echo "==> 停止容器"
	@docker compose down

# 查看 docker-compose 日志（实时跟踪）
# 用法: make docker-logs
docker-logs:
	@docker compose logs -f
