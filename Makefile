D      = deploy
C      = docker compose
PROD   = -f $(D)/compose.infra.yml -f $(D)/compose.mcp.yml -f $(D)/compose.execution.yml
LOCAL  = $(PROD) -f $(D)/compose.local.yml
INFRA  = -f $(D)/compose.infra.yml -f $(D)/compose.mcp.yml
MCP    = -f $(D)/compose.mcp.yml

.PHONY: up up-local down down-local logs ps \
        infra-up infra-down infra-logs \
        mcp-up mcp-down mcp-logs \
        vault-up vault-down vault-logs \
        worker-up worker-down \
        auth2api-login auth2api-login-codex auth2api-status \
        generate generate-local \
        opencode opencode-local \
        clean help

# ── стек ──────────────────────────────────────────────────────────────────────

up:             ## поднять прод-стек
	$(C) $(PROD) up -d

up-local:       ## поднять локальный стек (+ auth2api)
	$(C) $(LOCAL) up -d

down:           ## остановить прод-стек
	$(C) $(PROD) down

down-local:     ## остановить локальный стек
	$(C) $(LOCAL) down

logs:           ## логи всех сервисов
	$(C) $(PROD) logs -f --tail=100

ps:             ## статус контейнеров
	$(C) $(PROD) ps

clean:          ## снести всё включая volumes
	$(C) $(LOCAL) down -v

# ── слои ──────────────────────────────────────────────────────────────────────

infra-up:       ## поднять инфра-слой
	$(C) $(INFRA) up -d

infra-down:     ## остановить инфра-слой
	$(C) $(INFRA) down

infra-logs:     ## логи инфра-слоя
	$(C) $(INFRA) logs -f --tail=100

mcp-up:         ## поднять MCP серверы
	$(C) $(MCP) up -d --build

mcp-down:       ## остановить MCP серверы
	$(C) $(MCP) stop

mcp-logs:       ## логи MCP серверов
	$(C) $(MCP) logs -f --tail=100

vault-up:       ## пересобрать vault-mcp
	$(C) $(MCP) up -d --build vault-mcp

vault-down:     ## остановить vault-mcp
	$(C) $(MCP) stop vault-mcp

vault-logs:     ## логи vault-mcp
	$(C) $(MCP) logs -f --tail=100 vault-mcp

worker-up:      ## пересобрать и поднять worker
	$(C) $(PROD) up -d --build worker

worker-down:    ## остановить worker
	$(C) $(PROD) stop worker

# ── auth2api ──────────────────────────────────────────────────────────────────

auth2api-login: ## войти в Claude (OAuth → браузер)
	$(C) $(LOCAL) run --rm -it auth2api node dist/index.js --login --config=/config/config.yaml

auth2api-login-codex: ## войти в Codex/ChatGPT (OAuth → браузер)
	$(C) $(LOCAL) run --rm -it auth2api node dist/index.js --login --provider=codex --config=/config/config.yaml

auth2api-status: ## статус аккаунтов auth2api
	$(C) $(LOCAL) exec auth2api wget -qO- \
	  --header="Authorization: Bearer auth2api-internal-key" \
	  http://localhost:8317/admin/accounts

# ── конфиги ───────────────────────────────────────────────────────────────────

generate:       ## сгенерировать configs/ из config.prod.yml
	go run ./scripts/generate/ ENV=prod

generate-local: ## сгенерировать configs/ из config.local.yml
	go run ./scripts/generate/ ENV=local

opencode:       ## opencode с прод-окружением (.env.prod)
	env $$(grep -v '^[#$$]' .env.prod | xargs) opencode

opencode-local: ## opencode с локальным окружением (.env.local)
	env $$(grep -v '^[#$$]' .env.local | xargs) opencode

# ── справка ───────────────────────────────────────────────────────────────────

help:           ## показать все команды
	@grep -E '^[a-zA-Z_-]+:.*?##' $(MAKEFILE_LIST) | \
	  awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'
