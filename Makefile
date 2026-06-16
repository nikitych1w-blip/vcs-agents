COMPOSE           = docker compose
COMPOSE_INFRA     = $(COMPOSE) -f compose.infra.yml -f compose.mcp.yml
COMPOSE_MCP       = $(COMPOSE) -f compose.mcp.yml
COMPOSE_EXECUTION = $(COMPOSE) -f compose.infra.yml -f compose.mcp.yml -f compose.execution.yml

.PHONY: up down logs ps \
        infra-up infra-down infra-logs \
        mcp-up mcp-down mcp-logs \
        worker-up worker-down \
        auth2api-login auth2api-login-codex auth2api-status \
        vault-up vault-down vault-logs \
        clean sync-models validate-models install-hooks help

# ── полный стек ────────────────────────────────────────────────

up:            ## поднять всё
	$(COMPOSE) up -d

down:          ## остановить всё
	$(COMPOSE) down

logs:          ## логи всех сервисов
	$(COMPOSE) logs -f --tail=100

ps:            ## статус контейнеров
	$(COMPOSE) ps

# ── инфра (temporal, litellm-db, litellm, auth2api) ───────────

infra-up:      ## поднять инфра-слой (+ mcp как зависимость litellm)
	$(COMPOSE_INFRA) up -d

infra-down:    ## остановить инфра-слой
	$(COMPOSE_INFRA) down

infra-logs:    ## логи инфра-слоя
	$(COMPOSE_INFRA) logs -f --tail=100

# ── MCP серверы ────────────────────────────────────────────────

mcp-up:        ## поднять MCP серверы
	$(COMPOSE_MCP) up -d --build

mcp-down:      ## остановить MCP серверы
	$(COMPOSE_MCP) stop

mcp-logs:      ## логи MCP серверов
	$(COMPOSE_MCP) logs -f --tail=100

vault-up:      ## пересобрать и поднять vault-mcp
	$(COMPOSE_MCP) up -d --build vault-mcp

vault-down:    ## остановить vault-mcp
	$(COMPOSE_MCP) stop vault-mcp

vault-logs:    ## логи vault-mcp
	$(COMPOSE_MCP) logs -f --tail=100 vault-mcp

# ── воркер ────────────────────────────────────────────────────

worker-up:     ## поднять worker (пересборка)
	$(COMPOSE_EXECUTION) up -d --build worker

worker-down:   ## остановить worker
	$(COMPOSE_EXECUTION) stop worker

# ── auth2api ──────────────────────────────────────────────────

auth2api-login: ## войти в Claude аккаунт (откроет ссылку — пройти OAuth в браузере)
	$(COMPOSE) run --rm -it auth2api node dist/index.js --login --config=/config/config.yaml

auth2api-login-codex: ## войти в ChatGPT/Codex аккаунт
	$(COMPOSE) run --rm -it auth2api node dist/index.js --login --provider=codex --config=/config/config.yaml

auth2api-status: ## статус аккаунтов auth2api
	$(COMPOSE) exec auth2api wget -qO- --header="Authorization: Bearer auth2api-internal-key" http://localhost:8317/admin/accounts

# ── модели ────────────────────────────────────────────────────

sync-models:   ## обновить opencode.json из litellm/config.yaml
	bash scripts/sync_models.sh

validate-models: ## проверить что litellm и opencode синхронизированы
	bash scripts/sync_models.sh --check

# ── прочее ────────────────────────────────────────────────────

install-hooks: ## установить git pre-commit хук
	cp scripts/pre-commit .git/hooks/pre-commit
	chmod +x .git/hooks/pre-commit
	@echo "pre-commit hook installed"

clean:         ## снести всё включая volumes
	$(COMPOSE) down -v

help:          ## показать все команды
	@grep -E '^[a-zA-Z_-]+:.*?##' $(MAKEFILE_LIST) | \
	  awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'
