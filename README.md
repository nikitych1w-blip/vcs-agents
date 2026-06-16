# vcs-agents

Докер-окружение для запуска AI-агентов над VCS. LiteLLM проксирует все запросы к моделям, Temporal оркестрирует воркеры, vault-MCP даёт агентам доступ к кодовой базе.

## Запуск

Скопируй нужный `.env.*.example` в `.env.local` или `.env.prod`, заполни `CHANGE-ME` и:

```bash
./scripts/bootstrap.sh local   # или prod
```

Скрипт проверит переменные, сгенерирует конфиги, поднимет стек и создаст API-ключ для LiteLLM.

После первого запуска в локальном окружении нужно один раз войти в Claude:

```bash
make auth2api-login
```

Дальше запускай opencode через `make opencode-local` — он подхватит нужные переменные.

## Окружения

**local** — для разработки. Claude Sonnet по умолчанию через auth2api (работает на OAuth-сессии Claude Code, без API-ключа). Vault читает из локальной папки.

**prod** — компанийские модели через SBT AI Hub. Vault подключается к удалённому MCP.

Конфигурация окружений живёт в `config.local.yml` и `config.prod.yml`. Всё остальное (litellm config, opencode json, auth2api config) генерируется из них через `make generate[-local]`. Сгенерированное в git не кладётся.

## Модели

Список моделей и их роутинг — в `config.*.yml`. Добавил модель — перегенерировал — перезапустил litellm. Переключаться между моделями можно прямо в opencode через `/model`.

## Заметки

Temporal здесь dev-сервер на SQLite — нормально для локала, для настоящего прода нужен отдельный Postgres.

auth2api — неофициальный прокси поверх Claude Code. Удобно, но может сломаться при обновлении CLI. Если нужна стабильность — бери API-ключ на platform.anthropic.com.

`LITELLM_API_KEY` в `.env` — это виртуальный ключ, который setup.sh создаёт через LiteLLM API. Он нужен opencode и воркеру. Мастер-ключ (`LITELLM_MASTER_KEY`) никуда наружу не торчит.
