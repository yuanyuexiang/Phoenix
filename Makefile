# ---- Go 后端(backend/)----
.PHONY: build test vet tidy run-workflow run-parser run-ai run-mcp run-all smoke
build:
	cd backend && go build ./...

test:
	cd backend && go test ./...

vet:
	cd backend && go vet ./...

tidy:
	cd backend && go mod tidy

run-workflow:
	cd backend && go run ./cmd/workflow

run-parser:
	cd backend && go run ./cmd/parser

run-ai:
	cd backend && go run ./cmd/ai

run-mcp:
	cd backend && go run ./cmd/mcp

# 前台并行起全部 Go 服务(Ctrl-C 全停,开发用);前端另起:make fe-dev
run-all:
	@trap 'kill 0' INT TERM; \
	(cd backend && go run ./cmd/parser) & \
	(cd backend && go run ./cmd/ai) & \
	(cd backend && go run ./cmd/workflow) & \
	(cd backend && go run ./cmd/mcp) & \
	wait

# 端到端冒烟:模拟 WorkBuddy 依次调用五个 MCP 工具
smoke:
	cd backend && go run ./cmd/smoke -sample ../samples/sample-generic.txt

# ---- 前端(frontend/,Next.js)----
.PHONY: fe-install fe-dev fe-build
fe-install:
	cd frontend && npm install

fe-dev: # 开发服务器,8084,/api 经 next rewrites 代理到本机 workflow
	cd frontend && npm run dev

fe-build: # 静态导出到 frontend/out(生产由 nginx 托管,见 frontend/Dockerfile)
	cd frontend && BUILD_STATIC=1 npm run build

# ---- 基础设施 / 部署(deploy/)----
.PHONY: infra-up infra-down compose-up compose-down
infra-up:
	docker compose -f deploy/docker-compose.yml up -d postgres minio redis ocr

infra-down:
	docker compose -f deploy/docker-compose.yml down

# 全套容器化(六个应用服务 + 基础设施)
compose-up:
	docker compose -f deploy/docker-compose.yml up -d --build

compose-down:
	docker compose -f deploy/docker-compose.yml down
