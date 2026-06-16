#!/usr/bin/env bash
# Sync model list from litellm/config.yaml into opencode/opencode.json.
#
# Usage:
#   sync_models.sh          -- update opencode.json in-place
#   sync_models.sh --check  -- validate only, exit 1 if out of sync

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
LITELLM="$ROOT/litellm/config.yaml"
OPENCODE="$ROOT/opencode/opencode.json"

# Outputs tab-separated "name\tmodel_path" lines from litellm config.
# Relies on the YAML structure: model_name → litellm_params → model.
_litellm_pairs() {
    awk '
        /model_name:/    { name = $NF }
        /litellm_params:/ { want = 1 }
        want && /[[:space:]]model:/ { print name "\t" $NF; want = 0 }
    ' "$LITELLM"
}

# "openai/Vendor/Model-Name-X" → "Model Name X"
_display_name() {
    echo "$1" | awk -F'/' '{gsub(/-/, " ", $NF); print $NF}'
}

_sync() {
    local new_models='{}'
    local count=0

    while IFS=$'\t' read -r name model_path; do
        existing=$(jq -r --arg n "$name" '.provider.litellm.models[$n] // empty' "$OPENCODE")
        if [[ -n "$existing" ]]; then
            new_models=$(jq -n --argjson acc "$new_models" --arg n "$name" --argjson v "$existing" \
                '$acc + {($n): $v}')
        else
            display=$(_display_name "$model_path")
            new_models=$(jq -n --argjson acc "$new_models" --arg n "$name" --arg d "$display" \
                '$acc + {($n): {"name": $d}}')
            echo "  + ${name} → \"${display}\""
        fi
        (( count++ )) || true
    done < <(_litellm_pairs)

    while IFS= read -r name; do
        if ! jq -e --arg n "$name" '.[$n]' <<< "$new_models" > /dev/null 2>&1; then
            echo "  - removed ${name}"
        fi
    done < <(jq -r '.provider.litellm.models | keys[]' "$OPENCODE")

    local tmp
    tmp=$(mktemp)
    jq --argjson m "$new_models" '.provider.litellm.models = $m' "$OPENCODE" > "$tmp"
    mv "$tmp" "$OPENCODE"
    echo "synced: ${count} model(s) in opencode.json"
}

_check() {
    local litellm_models opencode_models missing stale ok=true

    litellm_models=$(_litellm_pairs | awk -F'\t' '{print $1}' | sort)
    opencode_models=$(jq -r '.provider.litellm.models | keys[]' "$OPENCODE" | sort)

    missing=$(comm -23 <(echo "$litellm_models") <(echo "$opencode_models"))
    stale=$(comm -13 <(echo "$litellm_models") <(echo "$opencode_models"))

    if [[ -n "$missing" ]]; then
        echo "error: in litellm but missing from opencode.json: ${missing//$'\n'/, }" >&2
        ok=false
    fi
    if [[ -n "$stale" ]]; then
        echo "error: in opencode.json but missing from litellm config: ${stale//$'\n'/, }" >&2
        ok=false
    fi

    if [[ "$ok" == false ]]; then
        echo "run 'make sync-models' to fix" >&2
        exit 1
    fi

    echo "ok: $(echo "$litellm_models" | wc -l | tr -d ' ') model(s) in sync"
}

if [[ "${1:-}" == "--check" ]]; then
    _check
else
    _sync
fi
