VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags="-s -w -X main.version=$(VERSION)"

.PHONY: dev backend frontend build run test clean release release-dry docker-build docker-up docker-down docker-logs

# 一键启动前后端开发服务器
# 用法: make dev
dev: backend-stop
	@bash -c 'trap "kill 0 2>/dev/null" INT TERM; \
		go run ./cmd/server </dev/null & \
		(cd web && npm run dev) </dev/null & \
		wait'

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

# 使用 Pake (Tauri) 打包桌面客户端壳子
# 依赖: Rust + Node + pake-cli (pnpm install -g pake-cli@3.13.0)
# 用法: make pake
pake:
	@echo "==> 使用 Pake 打包桌面客户端"
	@./scripts/build-pake.sh dist $(VERSION)

# 完整 macOS 桌面应用打包（Go 后端 + Pake 客户端 → app bundle）
# 用法: make desktop
desktop:
	@echo "==> 1/3 构建前端"
	@cd web && npm run build
	@echo "==> 2/3 构建后端 binary"
	@mkdir -p dist
	@CGO_ENABLED=1 go build $(LDFLAGS) -o dist/nexusagent ./cmd/server
	@echo "==> 3/3 Pake 客户端 + 组装 app bundle"
	@./scripts/build-pake.sh dist $(VERSION) app
	@./scripts/package-darwin.sh dist nexusagent dist
	@echo ""
	@echo "✅ 桌面应用打包完成"
	@du -sh dist/NexusAgent.app
	@echo "   双击 dist/NexusAgent.app 启动"

# Linux amd64 桌面应用打包
# 用法: make desktop-linux
desktop-linux:
	@echo "==> 1/3 构建前端"
	@cd web && npm run build
	@echo "==> 2/3 构建后端 binary"
	@mkdir -p dist
	@CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/nexusagent-linux-amd64 ./cmd/server
	@echo "==> 3/3 Pake AppImage + 组装桌面目录"
	@./scripts/build-pake.sh dist $(VERSION) appimage
	@./scripts/package-linux.sh dist nexusagent-linux-amd64 dist
	@cd dist && tar czf nexusagent-linux-desktop.tar.gz NexusAgent
	@echo ""
	@echo "✅ Linux 桌面应用打包完成: dist/nexusagent-linux-desktop.tar.gz"

# Windows amd64 桌面应用打包（需在 Windows 或交叉编译环境运行 Pake 步骤）
# 用法: make desktop-windows
desktop-windows:
	@echo "==> 1/3 构建前端"
	@cd web && npm run build
	@echo "==> 2/3 构建后端 binary"
	@mkdir -p dist
	@CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -tags=sqlite_nocgo $(LDFLAGS) -o dist/nexusagent-windows-amd64.exe ./cmd/server
	@echo "==> 3/3 Pake 客户端 + 组装桌面目录（需在 Windows 上执行 Pake）"
	@./scripts/build-pake.sh dist $(VERSION) x64
	@pwsh -File ./scripts/package-windows.ps1 -OutDir dist -BinName nexusagent-windows-amd64.exe -PakeDir dist
	@echo ""
	@echo "✅ Windows 桌面应用打包完成: dist/NexusAgent/"

# 开发模式：构建后以 release 模式启动并打开浏览器
# 用法: make run-desktop
run-desktop: build
	@echo "==> 单端口模式启动 http://localhost:8080"
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
	@echo "==> 创建 macOS 桌面应用包"
	@if command -v pake &>/dev/null; then \
		./scripts/build-pake.sh dist $(VERSION) app && \
		./scripts/package-darwin.sh dist nexusagent-darwin-arm64 dist && \
		cd dist && tar czf nexusagent-darwin-desktop.tar.gz NexusAgent.app; \
	else \
		echo "⚠️  未安装 pake-cli，跳过桌面客户端打包"; \
		echo "   安装: pnpm install -g pake-cli"; \
	fi
	@echo "==> 发布文件已生成到 dist/"
	@ls -lh dist/

# 运行后端全部测试
test:
	@go test ./...

# 清理构建产物
clean:
	@-rm -f nexusagent
	@-rm -rf web/dist dist

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
