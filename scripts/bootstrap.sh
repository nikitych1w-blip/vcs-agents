#!/usr/bin/env bash
# setup.sh — первый запуск окружения vcs-agents
# Usage: ./scripts/setup.sh [local|prod]

set -euo pipefail

ENV=${1:-prod}
ENV_FILE=".env.${ENV}"
LITELLM_URL="http://localhost:4000"

# ── цвета ─────────────────────────────────────────────────────────────────────

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'
info()    { echo -e "${CYAN}→${NC} $*"; }
success() { echo -e "${GREEN}✓${NC} $*"; }
warn()    { echo -e "${YELLOW}!${NC} $*"; }
die()     { echo -e "${RED}✗${NC} $*" >&2; exit 1; }

# ── 1. проверка аргумента ─────────────────────────────────────────────────────

[[ "$ENV" == "local" || "$ENV" == "prod" ]] \
  || die "неизвестное окружение: $ENV (используй 'local' или 'prod')"

echo ""
echo "  vcs-agents setup — env: ${CYAN}${ENV}${NC}"
echo ""

# ── 2. проверка .env файла ────────────────────────────────────────────────────

info "проверяю ${ENV_FILE}..."

if [[ ! -f "$ENV_FILE" ]]; then
  warn "${ENV_FILE} не найден"
  if [[ -f "${ENV_FILE}.example" ]]; then
    read -rp "  скопировать из ${ENV_FILE}.example? [y/N] " ans
    [[ "$ans" =~ ^[Yy]$ ]] || die "создай ${ENV_FILE} из ${ENV_FILE}.example и запусти снова"
    cp "${ENV_FILE}.example" "$ENV_FILE"
    success "создан ${ENV_FILE} — заполни CHANGE-ME и запусти снова"
    exit 0
  else
    die "файл ${ENV_FILE}.example не найден"
  fi
fi

# ── 3. загрузка и проверка переменных ────────────────────────────────────────

info "проверяю переменные в ${ENV_FILE}..."

while IFS= read -r line; do
  [[ "$line" =~ ^#|^$ ]] && continue
  export "$line" 2>/dev/null || true
done < "$ENV_FILE"

# Internal ключи — генерируем если не заданы, не требуем от пользователя
_autofill() {
  local var="$1" prefix="$2"
  local val="${!var:-}"
  if [[ -z "$val" || "$val" == *"CHANGE-ME"* ]]; then
    local new="${prefix}-$(LC_ALL=C tr -dc 'a-f0-9' < /dev/urandom | head -c 16)"
    sed -i.bak "s|^${var}=.*|${var}=${new}|" "$ENV_FILE" && rm -f "${ENV_FILE}.bak"
    export "$var"="$new"
    warn "${var} не задан — сгенерирован автоматически"
  fi
}
_autofill LITELLM_MASTER_KEY "sk-master"
_autofill LITELLM_SALT_KEY   "sk-salt"
_autofill LITELLM_DB_PASSWORD "litellm"

# Внешние секреты — должны быть заполнены пользователем
if [[ "$ENV" == "prod" ]]; then
  REQUIRED_VARS=(AI_HUB_SBT_KEY VAULT_MCP_URL VAULT_MCP_TOKEN)
else
  REQUIRED_VARS=(AI_HUB_SBT_KEY VAULT_LOCAL_PATH)
fi

MISSING=()
for var in "${REQUIRED_VARS[@]}"; do
  val="${!var:-}"
  [[ -z "$val" || "$val" == *"CHANGE-ME"* ]] && MISSING+=("$var")
done

if [[ ${#MISSING[@]} -gt 0 ]]; then
  die "заполни в ${ENV_FILE}:\n$(printf '    %s\n' "${MISSING[@]}")"
fi

# Дополнительная проверка для local: VAULT_LOCAL_PATH должен существовать
if [[ "$ENV" == "local" ]]; then
  if [[ ! -d "${VAULT_LOCAL_PATH:-}" ]]; then
    die "VAULT_LOCAL_PATH=${VAULT_LOCAL_PATH:-} не существует или не является директорией"
  fi
  # SC_LOCAL_PATH опционален — если не задан, sc-local-mcp пишет в /tmp/sc-local
  if [[ -z "${SC_LOCAL_PATH:-}" ]]; then
    warn "SC_LOCAL_PATH не задан — артефакты воркера пойдут в /tmp/sc-local"
  elif [[ ! -d "${SC_LOCAL_PATH}" ]]; then
    warn "SC_LOCAL_PATH=${SC_LOCAL_PATH} не существует — создай директорию или задай путь к клону репо"
  fi
fi

success "${ENV_FILE} в порядке"

# ── 4. генерация конфигов ─────────────────────────────────────────────────────

info "генерирую конфиги из config.${ENV}.yml..."
make "generate${ENV:+"-$([[ "$ENV" == "local" ]] && echo local || true)"}" 2>/dev/null \
  || { [[ "$ENV" == "local" ]] && make generate-local || make generate; }
success "конфиги сгенерированы в configs/"

# Определяем режим auth2api по наличию сгенерированного конфига.
# Режим: claude | codex | none (без auth2api — только SBT-модели)
AUTH2API_CFG="configs/auth2api/config.generated.yaml"
AUTH2API_MODE="none"
if [[ -f "$AUTH2API_CFG" ]]; then
  if grep -q "entrypoint: codex" "$AUTH2API_CFG" 2>/dev/null; then
    AUTH2API_MODE="codex"
  else
    AUTH2API_MODE="claude"
  fi
fi

# ── 5. запуск стека ───────────────────────────────────────────────────────────

info "поднимаю стек (auth2api: ${AUTH2API_MODE})..."
if [[ "$ENV" == "local" ]]; then
  if [[ "$AUTH2API_MODE" != "none" ]]; then
    make up-local
  else
    make up-local-bare
  fi
else
  make up
fi
success "контейнеры запущены"

# ── 5a. проверка auth2api (только local, только когда включён) ────────────────

if [[ "$ENV" == "local" && "$AUTH2API_MODE" != "none" ]]; then
  sleep 3
  if ! docker ps --filter "name=auth2api" --filter "status=running" | grep -q auth2api; then
    echo ""
    if [[ "$AUTH2API_MODE" == "codex" ]]; then
      warn "auth2api не запустился — нужно войти в Codex/ChatGPT (один раз):"
      echo ""
      echo "    make auth2api-login-codex"
    else
      warn "auth2api не запустился — нужно войти в Claude (один раз):"
      echo ""
      echo "    make auth2api-login"
    fi
    echo ""
    info "после авторизации auth2api поднимется автоматически (restart: on-failure)"
    echo ""
  else
    success "auth2api запущен (${AUTH2API_MODE})"
  fi
fi

# ── 6. ожидание litellm ───────────────────────────────────────────────────────

info "жду litellm (${LITELLM_URL})..."
TRIES=0
until curl -sf "${LITELLM_URL}/health/liveliness" > /dev/null 2>&1; do
  TRIES=$((TRIES + 1))
  [[ $TRIES -gt 40 ]] && die "litellm не поднялся за 2 минуты — проверь: make logs"
  echo -n "."
  sleep 3
done
echo ""
success "litellm готов"

# ── 7. генерация LITELLM_API_KEY ──────────────────────────────────────────────

CURRENT_KEY="${LITELLM_API_KEY:-}"
if [[ -z "$CURRENT_KEY" || "$CURRENT_KEY" == *"CHANGE-ME"* ]]; then
  info "генерирую LITELLM_API_KEY через litellm..."

  RESPONSE=$(curl -sf -X POST "${LITELLM_URL}/key/generate" \
    -H "Authorization: Bearer ${LITELLM_MASTER_KEY}" \
    -H "Content-Type: application/json" \
    -d "{\"key_alias\": \"opencode-${ENV}\", \"duration\": null}") \
    || die "не удалось сгенерировать ключ — проверь LITELLM_MASTER_KEY"

  NEW_KEY=$(echo "$RESPONSE" | jq -r '.key')
  [[ "$NEW_KEY" == "null" || -z "$NEW_KEY" ]] && die "litellm вернул пустой ключ: $RESPONSE"

  tmp=$(mktemp)
  sed "s|^LITELLM_API_KEY=.*|LITELLM_API_KEY=${NEW_KEY}|" "$ENV_FILE" > "$tmp" && mv "$tmp" "$ENV_FILE"
  success "LITELLM_API_KEY записан в ${ENV_FILE}"
else
  success "LITELLM_API_KEY уже задан, пропускаю"
fi

# ── готово ────────────────────────────────────────────────────────────────────

echo ""
echo -e "  ${GREEN}всё готово!${NC}"
echo ""
echo "  LiteLLM UI  → ${LITELLM_URL}/ui"
echo "  Temporal UI → http://localhost:8233"
if [[ "$ENV" == "local" ]]; then
  echo ""
  echo "  запусти opencode:"
  echo "    make opencode-local"
  if [[ "$AUTH2API_MODE" == "codex" ]]; then
    echo ""
    echo "  первый вход в Codex/ChatGPT (один раз):"
    echo "    make auth2api-login-codex"
  elif [[ "$AUTH2API_MODE" == "claude" ]]; then
    echo ""
    echo "  первый вход в Claude (один раз):"
    echo "    make auth2api-login"
  fi
  echo ""
  echo "  запусти воркер (опционально):"
  echo "    make worker-up"
  echo "    CHANGE_ID=vcs-00000 SPEC_NAME=repos-search make run-spec"
else
  echo ""
  echo "  запусти opencode:"
  echo "    make opencode"
  echo ""
  echo "  запусти воркер (опционально):"
  echo "    make worker-up"
  echo "    CHANGE_ID=vcs-00000 SPEC_NAME=repos-search make run-spec"
fi
echo ""
