D  = deploy
C  = docker compose
EF = --env-file .env.$(or $(ENV),local)

# ── Compose-наборы ────────────────────────────────────────────────────────────
# Общая база: инфраструктура (Temporal, LiteLLM, Postgres) + MCP-серверы
BASE_F = -f $(D)/compose.infra.yml -f $(D)/compose.mcp.yml

PROD       = $(EF)                $(BASE_F)
LOCAL      = --env-file .env.local $(BASE_F) -f $(D)/compose.local.yml
LOCAL_BARE = --env-file .env.local $(BASE_F)
EXEC       = $(EF)                $(BASE_F) -f $(D)/compose.execution.yml
INFRA      = $(EF)                $(BASE_F)
MCP        = $(EF) -f $(D)/compose.mcp.yml

.PHONY: bootstrap bootstrap-local \
        up up-local up-local-bare down down-local down-local-bare logs ps clean \
        infra-up infra-down infra-logs \
        mcp-up mcp-down mcp-logs \
        vault-up vault-down vault-logs \
        sc-local-up sc-local-down sc-local-logs \
        worker-up worker-down worker-logs run-spec \
        auth2api-login auth2api-login-codex auth2api-status \
        generate generate-local \
        opencode opencode-local \
        help

# ── Стек ─────────────────────────────────────────────────────────────────────

up:              ## поднять прод-стек
	$(C) $(PROD) up -d

up-local:        ## локальный стек + auth2api (Claude или Codex)
	$(C) $(LOCAL) up -d

up-local-bare:   ## локальный стек без auth2api (только SBT-модели)
	$(C) $(LOCAL_BARE) up -d

down:            ## остановить прод-стек
	$(C) $(PROD) down

down-local:      ## остановить локальный стек (с auth2api)
	$(C) $(LOCAL) down

down-local-bare: ## остановить локальный стек (без auth2api)
	$(C) $(LOCAL_BARE) down

logs:            ## логи всех сервисов (прод)
	$(C) $(PROD) logs -f --tail=100

ps:              ## статус контейнеров (прод)
	$(C) $(PROD) ps

clean:           ## снести всё включая volumes
	$(C) $(LOCAL) down -v

# ── Слои ─────────────────────────────────────────────────────────────────────

infra-up:        ## поднять инфра-слой
	$(C) $(INFRA) up -d

infra-down:      ## остановить инфра-слой
	$(C) $(INFRA) down

infra-logs:      ## логи инфра-слоя
	$(C) $(INFRA) logs -f --tail=100

mcp-up:          ## пересобрать и поднять все MCP-серверы
	$(C) $(MCP) up -d --build

mcp-down:        ## остановить все MCP-серверы
	$(C) $(MCP) stop

mcp-logs:        ## логи всех MCP-серверов
	$(C) $(MCP) logs -f --tail=100

# ── MCP: vault ────────────────────────────────────────────────────────────────

vault-up:        ## пересобрать vault-mcp
	$(C) $(MCP) up -d --build vault-mcp

vault-down:      ## остановить vault-mcp
	$(C) $(MCP) stop vault-mcp

vault-logs:      ## логи vault-mcp
	$(C) $(MCP) logs -f --tail=100 vault-mcp

# ── MCP: sc-local ─────────────────────────────────────────────────────────────

sc-local-up:     ## пересобрать sc-local-mcp
	$(C) $(MCP) up -d --build sc-local-mcp

sc-local-down:   ## остановить sc-local-mcp
	$(C) $(MCP) stop sc-local-mcp

sc-local-logs:   ## логи sc-local-mcp
	$(C) $(MCP) logs -f --tail=100 sc-local-mcp

# ── Worker ────────────────────────────────────────────────────────────────────

worker-up:       ## пересобрать и поднять worker
	$(C) $(EXEC) up -d --build worker

worker-down:     ## остановить worker
	$(C) $(EXEC) stop worker

worker-logs:     ## логи worker
	$(C) $(EXEC) logs -f --tail=100 worker

# CHANGE_ID=vcs-00000 SPEC_NAME=repos-search make run-spec
run-spec:        ## запустить ExecuteOpenSpec workflow (нужны CHANGE_ID=... SPEC_NAME=...)
	docker exec deploy-temporal-1 temporal workflow start \
	  --address 127.0.0.1:7233 \
	  --task-queue openspec-execute \
	  --type ExecuteOpenSpecWorkflow \
	  --input "{\"change_id\":\"$(CHANGE_ID)\",\"spec_name\":\"$(SPEC_NAME)\"}"

# ── auth2api ──────────────────────────────────────────────────────────────────

auth2api-login:       ## войти в Claude (OAuth → браузер)
	bash scripts/auth2api-login.sh anthropic

auth2api-login-codex: ## войти в Codex/ChatGPT (OAuth → браузер)
	bash scripts/auth2api-login.sh codex

auth2api-status:      ## статус аккаунтов auth2api
	$(C) $(LOCAL) exec auth2api wget -qO- \
	  --header="Authorization: Bearer auth2api-internal-key" \
	  http://localhost:8317/admin/accounts

# ── Конфиги ───────────────────────────────────────────────────────────────────

bootstrap:       ## первый запуск прод-окружения
	bash scripts/bootstrap.sh prod

bootstrap-local: ## первый запуск локального окружения
	bash scripts/bootstrap.sh local

# GENV=local|prod (по умолчанию prod)
GENV ?= prod
generate:        ## сгенерировать configs/ (GENV=local|prod, по умолчанию prod)
	cd scripts/generate && go run . ENV=$(GENV)

generate-local:  ## сгенерировать configs/ из config.local.yml
	cd scripts/generate && go run . ENV=local

opencode:        ## opencode с прод-окружением
	env $$(grep -v '^[#$$]' .env.prod | xargs) opencode

opencode-local:  ## opencode с локальным окружением
	env $$(grep -v '^[#$$]' .env.local | xargs) opencode

# ── Справка ───────────────────────────────────────────────────────────────────

help:            ## показать все команды
	@grep -E '^[a-zA-Z_-]+:.*?##' $(MAKEFILE_LIST) | \
	  awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-24s\033[0m %s\n", $$1, $$2}'
