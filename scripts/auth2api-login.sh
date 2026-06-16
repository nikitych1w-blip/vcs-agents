#!/usr/bin/env bash
# Запускает auth2api --login нативно на хосте, чтобы OAuth callback работал в браузере.
# После логина копирует токены в Docker volume.

set -euo pipefail

PROVIDER=${1:-anthropic}
VOLUME="deploy_auth2api-data"
TMP_APP=$(mktemp -d)
TMP_TOKENS=$(mktemp -d)

trap 'rm -rf "$TMP_APP" "$TMP_TOKENS"' EXIT

# ── найти или собрать образ ───────────────────────────────────────────────────

IMAGE=$(docker compose --env-file .env.local \
  -f deploy/compose.local.yml images -q auth2api 2>/dev/null | head -1)

if [[ -z "$IMAGE" ]]; then
  echo "→ собираю образ auth2api..."
  docker compose --env-file .env.local -f deploy/compose.local.yml build auth2api
  IMAGE=$(docker compose --env-file .env.local \
    -f deploy/compose.local.yml images -q auth2api 2>/dev/null | head -1)
fi

# ── извлечь приложение из образа ─────────────────────────────────────────────

echo "→ извлекаю auth2api из образа..."
docker run --rm -v "$TMP_APP:/out" --entrypoint="" "$IMAGE" \
  sh -c "cp -r /app/dist /app/node_modules /out/"

# ── временный конфиг с локальным auth-dir ─────────────────────────────────────

cat > "$TMP_APP/config.yaml" <<EOF
host: ""
port: 8317
auth-dir: "$TMP_TOKENS"
api-keys:
  - "auth2api-internal-key"
cloaking:
  cli-version: "2.1.153"
  entrypoint: cli
EOF

# ── логин ─────────────────────────────────────────────────────────────────────

echo "→ запускаю логин (провайдер: $PROVIDER)..."
echo ""

if [[ "$PROVIDER" == "anthropic" ]]; then
  node "$TMP_APP/dist/index.js" --login --config="$TMP_APP/config.yaml"
else
  node "$TMP_APP/dist/index.js" --login --provider="$PROVIDER" --config="$TMP_APP/config.yaml"
fi

# ── копируем токены в Docker volume ──────────────────────────────────────────

SAVED=$(ls "$TMP_TOKENS" 2>/dev/null | wc -l | tr -d ' ')
if [[ "$SAVED" -eq 0 ]]; then
  echo "✗ токены не сохранились — логин не завершён"
  exit 1
fi

echo ""
echo "→ сохраняю токены в Docker volume $VOLUME..."
docker run --rm \
  -v "$TMP_TOKENS:/tokens:ro" \
  -v "$VOLUME:/data" \
  alpine sh -c "cp -r /tokens/. /data/"

echo "✓ готово — auth2api поднимется автоматически"
