.PHONY: dev backend frontend build run test clean docker-build docker-up docker-down docker-logs

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
	@echo "==> 构建后端"
	@go build -o nexusagent ./cmd/server

# 单端口运行：先构建前端，再以 release 模式启动后端（前端 + API 同端口）
# 用法: make run
run: build
	@echo "==> 单端口启动 http://localhost:8080"
	@SERVER_MODE=release ./nexusagent

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
