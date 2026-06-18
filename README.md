# vcs-agents

Докер-окружение для запуска AI-агентов над VCS. LiteLLM проксирует запросы к моделям, Temporal оркестрирует воркеры, два MCP-сервера дают агентам доступ к vcs-vault и запись артефактов в репозиторий.

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

---

## Окружения

| | local | prod |
|---|---|---|
| Модели | Claude / Codex / SBT | Компанийские модели через SBT AI Hub |
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

---

## Worker — DAG-пайплайн по OpenSpec

Worker — Temporal-агент, читает пайплайн из `vcs-vault` и запускает шаги через LLM.

### Архитектура

Workflow data-driven: шаги, роли и зависимости определяются **не в коде**, а в двух файлах vcs-vault:

| Файл в vcs-vault | Роль |
|---|---|
| `openspec/schemas/vcs/schema.yaml` | DAG-топология: шаги (`artifacts`), зависимости (`requires:`), инструкция (`instruction:`) |
| `openspec/config.yaml` | Системные промпты ролей (`rules:`) — SA, BE, FE, QA, QAA |

Очередь каждого шага выводится из поля `generates:` в schema.yaml:  
`sa/...` → `openspec-sa`, `be/...` → `openspec-be`, `fe/...` → `openspec-fe` и т.д.

### Шаги workflow

```
FetchConfig      — читает openspec/schemas/vcs/schema.yaml, строит DAG
ReadSpec         — читает proposal.md + spec.md для change_id из vault
──────────────────────────────────────────────────────────────
Wave 0:  proposal                          → worker-sa
Wave 1:  sa-specs                          → worker-sa
Wave 2:  be-design  fe-design  qa-plan     → worker-be / worker-fe / worker-qa  (параллельно)
Wave 3:  be-tasks   fe-tasks   qaa-tasks   → worker-be / worker-fe / worker-qaa (параллельно)
```

Каждый шаг (`RunStep`):
1. Читает системный промпт роли из `openspec/config.yaml` → `rules.<step_name>`
2. Берёт инструкцию шага из schema.yaml (`instruction:`)
3. Вызывает LLM через LiteLLM
4. Возвращает артефакт в accumulated context следующих волн

### Структура vcs-vault (обязательно)

```
openspec/
  schemas/
    vcs/
      schema.yaml          ← DAG-топология (artifacts + requires)
  config.yaml              ← промпты ролей (rules: proposal/sa-specs/be-design/…)
  changes/
    <change_id>/
      sa/
        proposal.md
        specs/
          <spec_name>/
            spec.md
```

### Воркеры

Каждый воркер слушает свою task-queue Temporal:

| Сервис | WORKER_ROLE | WORKER_QUEUE | Регистрирует workflow |
|---|---|---|---|
| `worker` | orchestrator | openspec-execute | ✓ |
| `worker-sa` | SA | openspec-sa | — |
| `worker-be` | BE | openspec-be | — |
| `worker-fe` | FE | openspec-fe | — |
| `worker-qa` | QA | openspec-qa | — |
| `worker-qaa` | QAA | openspec-qaa | — |

### Запуск воркеров

```bash
make worker-up       # пересобрать и поднять оркестратор (worker)
make worker-logs     # логи оркестратора

# Все воркеры сразу (включая роли):
docker compose --env-file .env.local \
  -f deploy/compose.infra.yml \
  -f deploy/compose.mcp.yml \
  -f deploy/compose.execution.yml \
  up -d --build
```

### Запуск workflow

```bash
CHANGE_ID=vcs-00000 SPEC_NAME=repos-search make run-spec
```

Или напрямую:

```bash
temporal workflow start \
  --address localhost:7233 \
  --task-queue openspec-execute \
  --type ExecuteOpenSpecWorkflow \
  --input '{"change_id":"vcs-00000","spec_name":"repos-search"}'
```

Поля input:

| Поле | По умолчанию | Описание |
|---|---|---|
| `change_id` | — | ID изменения в vcs-vault (обязательно) |
| `spec_name` | — | Имя спеки (обязательно) |
| `role` | автодетект | Роль в vault (`sa`, `be`, …) |
| `model` | `DEFAULT_MODEL` | Переопределить модель LLM |
| `config_ref` | `vcs` | Имя схемы в vault (`openspec/schemas/<name>/schema.yaml`) |

Наблюдать выполнение: **Temporal Web UI** → `http://localhost:8233`

В логах оркестратора (`make worker-logs`) видны волны:
```
wave start wave=0 steps=[proposal]
wave start wave=1 steps=[sa-specs]
wave start wave=2 steps=[be-design fe-design qa-plan]   ← параллельно
wave start wave=3 steps=[be-tasks fe-tasks qaa-tasks]   ← параллельно
```

---

## MCP-серверы

### vault-mcp (порт 8080, read-only)

Монтирует `VAULT_LOCAL_PATH` как `/vault`. Источник правды для workflow.

| Инструмент | Описание |
|---|---|
| `read_schema` | schema.yaml по имени схемы (`schema: vcs` по умолчанию) |
| `read_step_prompt` | Системный промпт роли из `config.yaml` по `step_name` |
| `list_openspec_changes` | Список изменений с ролями и спеками |
| `read_openspec_proposal` | proposal.md по `change_id` |
| `read_openspec_spec` | spec.md по `change_id` + `spec_name` |
| `read_skill` | Скилл из `skills/{id}.md` |
| `search_knowledge` | Полнотекстовый поиск по `knowledge/` |

### sc-local-mcp (порт 8081, read-write)

Запись артефактов в локальный клон репозитория. Монтирует `SC_LOCAL_PATH` с правами `rw`.

| Инструмент | Описание |
|---|---|
| `write_file` | Записать файл по относительному пути |
| `read_file` | Прочитать файл |
| `list_files` | Листинг директории |

В проде заменяется на вызов SC API (`SC_BASE_URL` + `SC_TOKEN`).

---

## Переменные окружения

### `.env.local` / `.env.prod` — секреты

| Переменная | Обязательна | Описание |
|---|---|---|
| `LITELLM_MASTER_KEY` | ✓ | Мастер-ключ LiteLLM (генерируется bootstrap.sh) |
| `LITELLM_SALT_KEY` | ✓ | Соль для хеширования ключей |
| `LITELLM_DB_PASSWORD` | ✓ | Пароль БД LiteLLM |
| `LITELLM_API_KEY` | ✓ | Виртуальный ключ для воркера и opencode |
| `VAULT_LOCAL_PATH` | ✓ local | Абсолютный путь к локальному клону vcs-vault |
| `TEMPORAL_ADDRESS` | ✓ | Адрес Temporal (`temporal:7233` внутри Docker) |
| `WORKER_LITELLM_BASE_URL` | ✓ | URL LiteLLM из Docker-сети (`http://litellm:4000`) |
| `SBT_API_BASE_URL` | SBT-модели | Базовый URL SBT AI Hub |
| `AI_HUB_SBT_KEY` | SBT-модели | API-ключ SBT |
| `SC_BASE_URL` | prod write | URL SourceControl API |
| `SC_TOKEN` | prod write | Токен SC API |
| `SC_LOCAL_PATH` | local write | Путь к локальному клону репо для записи артефактов |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | нет | Эндпоинт трейсинга (Jaeger/Tempo) |

### `configs/worker/config.generated.env` — статические настройки

Генерируется из `config.*.yml` через `make generate-local`.

| Переменная | Описание |
|---|---|
| `DEFAULT_MODEL` | Модель LLM по умолчанию (`claude-sonnet` / `company-main`) |
| `TEMPORAL_NAMESPACE` | Неймспейс Temporal (обычно `default`) |

### Переменные воркеров (задаются в compose.execution.yml)

| Переменная | Значение | Описание |
|---|---|---|
| `WORKER_ROLE` | `orchestrator` / `SA` / `BE` / `FE` / `QA` / `QAA` | Роль воркера |
| `WORKER_QUEUE` | `openspec-execute` / `openspec-sa` / … | Task-queue Temporal |
| `REGISTER_WORKFLOW` | `true` (только orchestrator) | Регистрировать ли workflow |
| `VAULT_MCP_URL` | `http://vault-mcp:8080/mcp` | URL vault-mcp внутри Docker |
| `SC_LOCAL_MCP_URL` | `http://sc-local-mcp:8081/mcp` | URL sc-local-mcp (только orchestrator) |

---

## Модели

Список и роутинг — в `config.*.yml`. Добавил модель → перегенерировал → перезапустил litellm. Переключаться можно в opencode через `/model`.

---

## Заметки

**Temporal** — dev-сервер на SQLite. Для настоящего прода нужен отдельный PostgreSQL.

**auth2api** — неофициальный прокси поверх Claude Code. Удобно локально, но может сломаться при обновлении CLI. Для стабильности — API-ключ на platform.anthropic.com.

**LITELLM_API_KEY** в `.env` — виртуальный ключ, который bootstrap.sh создаёт через LiteLLM API. Нужен opencode и воркеру. Мастер-ключ (`LITELLM_MASTER_KEY`) наружу не торчит.

**sc-local-mcp** в проде не поднимается. Для записи артефактов в prod нужно реализовать activity через SC API.

**vcs-vault** — подключается как read-only volume. Workflow читает из него `schema.yaml` и `config.yaml` — оба файла должны существовать до запуска воркеров.
