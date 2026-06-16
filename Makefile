COMPOSE = docker compose

.PHONY: up down logs ps vault-up vault-down vault-logs worker-up clean help

up:
	$(COMPOSE) up -d

down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f --tail=100

ps:
	$(COMPOSE) ps

vault-up:
	$(COMPOSE) up -d --build vault-mcp

vault-down:
	$(COMPOSE) stop vault-mcp

vault-logs:
	$(COMPOSE) logs -f --tail=100 vault-mcp

worker-up:
	$(COMPOSE) --profile worker up -d --build

clean:
	$(COMPOSE) --profile worker down -v

help:          ## показать все команды
	@grep -E '^[a-zA-Z_-]+:.*?##' $(MAKEFILE_LIST) | \
	  awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'