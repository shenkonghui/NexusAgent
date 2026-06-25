.PHONY: dev backend frontend build test clean

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

# 运行后端全部测试
test:
	@go test ./...

# 清理构建产物
clean:
	@-rm -f nexusagent
	@-rm -rf web/dist
