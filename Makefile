# ---- Go 后端(backend/)----
.PHONY: build test vet tidy run-workflow smoke smoke-rag
build:
	cd backend && go build ./...

test:
	cd backend && go test ./...

vet:
	cd backend && go vet ./...

tidy:
	cd backend && go mod tidy

run-workflow: # 唯一后端服务(REST 面 /api、/pub/v1);前端另起:make fe-dev
	cd backend && go run ./cmd/workflow

# 端到端冒烟:走 workflow 管理面 /api,上传归档 → 回传字段+正文入库 → 结构化查询(含字段级过滤)
smoke:
	cd backend && go run ./cmd/smoke -sample ../samples/sample-generic.txt

# 含知识库语义问答的冒烟:workflow 须已配置 PHX_EMBED_*(embedding 端点)
smoke-rag:
	cd backend && go run ./cmd/smoke -sample ../samples/sample-generic.txt -rag

# ---- 员工级 REST OAuth 联调(docs/员工级REST-API-OAuth接入方案.md)----
.PHONY: oauth-up oauth-down smoke-oauth
oauth-up: # 开发用 Keycloak(realm/客户端/测试用户 alice、bob 自动导入,http://localhost:8180)
	docker compose -f deploy/docker-compose.yml --profile oauth up -d keycloak

oauth-down:
	docker compose -f deploy/docker-compose.yml --profile oauth stop keycloak

# OAuth 冒烟:走员工面 /pub/v1,workflow 须以
# PHX_API_OIDC_ISSUER=http://localhost:8180/realms/phoenix 启动;
# 先负向验证无 token 被拒,再用 alice 的 token 跑全流程并断言 uploaded_by
smoke-oauth:
	cd backend && go run ./cmd/smoke -sample ../samples/sample-generic.txt \
		-oauth-issuer http://localhost:8180/realms/phoenix -oauth-user alice -oauth-pass alice123 -require-auth

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
	docker compose -f deploy/docker-compose.yml up -d postgres minio redis

infra-down:
	docker compose -f deploy/docker-compose.yml down

# 全套容器化(workflow/admin + 基础设施)
compose-up:
	docker compose -f deploy/docker-compose.yml up -d --build

compose-down:
	docker compose -f deploy/docker-compose.yml down
