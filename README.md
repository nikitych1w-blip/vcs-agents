# vcs-agents

Докер-окружение для запуска AI-агентов над VCS. LiteLLM проксирует все запросы к моделям, Temporal оркестрирует воркеры, два MCP-сервера дают агентам доступ к кодовой базе и запись артефактов в репозиторий.

## Быстрый старт

```bash
cp .env.local.example .env.local   # заполни CHANGE-ME
./scripts/bootstrap.sh local       # или prod
```

Скрипт проверит переменные, сгенерирует конфиги, поднимет стек и создаст API-ключ для LiteLLM.

В локальном окружении нужно один раз войти в Claude:

```bash
make auth2api-login
```

Дальше запускай opencode через `make opencode-local`.

## Окружения

| | local | prod |
|---|---|---|
| Модели | Claude / Codex / SBT (выбор через config) | Компанийские модели через SBT AI Hub |
| Vault | Локальная папка → vault-mcp (read-only) | Удалённый MCP с токеном |
| Запись артефактов | Локальный клон репо → sc-local-mcp | SC API (SC_BASE_URL + SC_TOKEN) |
| Модель по умолчанию | `claude-sonnet` | `company-main` |

Конфигурация окружений — `config.local.yml` и `config.prod.yml`. Всё остальное генерируется:

```bash
make generate-local   # → configs/litellm/, configs/auth2api/, configs/worker/
make generate         # то же для prod
```

Сгенерированные файлы в git не кладутся.

### Режимы auth2api

Управляется полем `auth2api.cloaking.entrypoint` в `config.local.yml`:

| Режим | entrypoint | Запуск стека | Логин |
|---|---|---|---|
| Claude (по умолчанию) | `cli` | `make up-local` | `make auth2api-login` |
| Codex / ChatGPT | `codex` | `make up-local` | `make auth2api-login-codex` |
| Без auth2api (только SBT) | — | `make up-local-bare` | — |

Для режима **без auth2api**: удали секцию `auth2api:` из `config.local.yml`, перегенерируй конфиги (`make generate-local`) и запускай через `make up-local-bare`. Убедись, что `default_model` указывает на SBT-модель, например `company-main`.

`bootstrap.sh` определяет режим автоматически по наличию `configs/auth2api/config.generated.yaml` и подбирает нужный Makefile-таргет и подсказки по логину.

## Модели

Список и роутинг — в `config.*.yml`. Добавил модель → перегенерировал → перезапустил litellm. Переключаться можно прямо в opencode через `/model`.

## Worker — запуск разработки по OpenSpec

Worker — Temporal-агент, который читает спеку из vault и запускает цикл разработки через LLM.

### Структура спек в vcs-vault

```
openspec/
  changes/
    vcs-00000/
      sa/
        proposal.md        # бизнес-намерение: intent, метрики, capabilities, impact
        specs/
          repos-search/
            spec.md        # требования: SHALL-утверждения + сценарии
```

### Шаги workflow

```
ReadSpec → FetchContext (опц.) → GenerateCode → WriteFiles (опц.)
```

1. **ReadSpec** — читает `spec.md` + `proposal.md` через vault-mcp
2. **FetchContext** — подтягивает скиллы и knowledge из vault (если переданы в input)
3. **GenerateCode** — вызывает LLM через LiteLLM
4. **WriteFiles** — записывает артефакт в `output/{change_id}/{spec_name}.md` через sc-local-mcp; пропускается если `SC_LOCAL_MCP_URL` не задан

### Запуск воркера

```bash
make worker-up     # пересобрать и поднять
make worker-logs   # логи в реальном времени
```

### Запуск workflow по спеке

```bash
CHANGE_ID=vcs-00000 SPEC_NAME=repos-search make run-spec
```

Или напрямую через Temporal CLI:

```bash
temporal workflow start \
  --address localhost:7233 \
  --task-queue openspec-execute \
  --type ExecuteOpenSpecWorkflow \
  --input '{"change_id":"vcs-00000","spec_name":"repos-search"}'
```

Опциональные поля input:

| Поле | По умолчанию | Описание |
|---|---|---|
| `role` | автодетект | Роль в vault (`sa`, `dev`, …) |
| `model` | `DEFAULT_MODEL` из config | Переопределить модель для этого прогона |
| `skills` | `[]` | Список skill_id из `skills/*.md` |
| `knowledge` | `[]` | Поисковые запросы к `knowledge/` |

Наблюдать выполнение: **Temporal Web UI** → `http://localhost:8233`

## MCP-серверы

### vault-mcp (порт 8080, read-only)

Доступ к vcs-vault. Инструменты доступны агентам через LiteLLM MCP.

| Инструмент | Описание |
|---|---|
| `list_openspec_changes` | Список изменений с ролями и спеками |
| `read_openspec_proposal` | Proposal (бизнес-намерение) по `change_id` |
| `read_openspec_spec` | Spec.md (требования) по `change_id` + `spec_name` |
| `read_skill` | Скилл из `skills/{id}.md` |
| `search_knowledge` | Полнотекстовый поиск по `knowledge/` |

### sc-local-mcp (порт 8081, read-write)

Запись артефактов в локальный клон репозитория. Монтирует `SC_LOCAL_PATH` с правами `rw`. В проде заменяется на вызов SC API (SC_BASE_URL + SC_TOKEN).

| Инструмент | Описание |
|---|---|
| `write_file` | Записать файл по относительному пути (создаёт директории) |
| `read_file` | Прочитать файл |
| `list_files` | Листинг директории |

## Переменные окружения

Секреты — в `.env.local` / `.env.prod`. Статические настройки генерируются в `configs/worker/config.generated.env`.

| Переменная | Где задаётся | Описание |
|---|---|---|
| `SC_LOCAL_PATH` | `.env.local` | Путь к локальному клону репо для записи артефактов |
| `SC_BASE_URL` | `.env.*` | URL SourceControl API (для прод-интеграции) |
| `SC_TOKEN` | `.env.*` | Токен SC API |
| `VAULT_LOCAL_PATH` | `.env.local` | Путь к локальному клону vcs-vault |
| `TEMPORAL_ADDRESS` | `.env.*` | Адрес Temporal (`temporal:7233` внутри Docker) |
| `DEFAULT_MODEL` | генерируется | Берётся из `default_model` в `config.*.yml` |
| `TEMPORAL_NAMESPACE` | генерируется | Берётся из `worker.temporal_namespace` в config |

## Заметки

**Temporal** — dev-сервер на SQLite. Для настоящего прода нужен отдельный PostgreSQL.

**auth2api** — неофициальный прокси поверх Claude Code. Удобно локально, но может сломаться при обновлении CLI. Для стабильности — API-ключ на platform.anthropic.com.

**LITELLM_API_KEY** в `.env` — виртуальный ключ, который bootstrap.sh создаёт через LiteLLM API. Нужен opencode и воркеру. Мастер-ключ (`LITELLM_MASTER_KEY`) наружу не торчит.

**sc-local-mcp** в проде не поднимается. Для записи артефактов в prod нужно реализовать activity, которая использует SC API через `SC_BASE_URL` + `SC_TOKEN`.
