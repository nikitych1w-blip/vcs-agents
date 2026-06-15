COMPOSE = docker compose
.PHONY: up down logs ps worker-up obs-up obs-down clean

up:
	$(COMPOSE) up -d

down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f --tail=100

ps:
	$(COMPOSE) ps

worker-up:
	$(COMPOSE) --profile worker up -d --build

clean:
	$(COMPOSE) --profile observability --profile worker down -v
