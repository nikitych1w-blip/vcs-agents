# vcs-agents — local control-plane

Docker Compose for the **inference control-plane** of `vcs-agents`. First run brings up the
spine; observability and the worker are opt-in so you don't pay for the whole stack on day one.

What this is **not**: it doesn't run your product (prod SourceControl is an external hub the
worker talks to), and it doesn't run `vcs-sandbox` (that's a separate ephemeral, per-run compose).

## Layout

| File | What |
|---|---|
| `docker-compose.yml` | Core: Temporal dev server + LiteLLM (+ Postgres). Plus a `worker` profile. |
| `docker-compose.observability.yml` | Optional Langfuse v3 overlay (+6 containers). |
| `litellm/config.yaml` | Model aliases, routing, fallbacks. |
| `.env.example` | All secrets/config. Copy to `.env`. |
| `Makefile` | `up` / `down` / `logs` / `obs-up` / `worker-up` / `clean`. |

## First run

```bash
cp ..env.example .env        # then edit: set LITELLM_MASTER_KEY, LITELLM_SALT_KEY,
                            # LITELLM_DB_PASSWORD, and at least one provider key
make up                     # or: docker compose up -d
```

Brings up:
- **Temporal** — UI at http://localhost:8233, gRPC at `localhost:7233` (workers connect here)
- **LiteLLM** — OpenAI-compatible endpoint at http://localhost:4000, dashboard at `/ui`

Verify:

```bash
docker compose ps                                   # both healthy
curl http://localhost:4000/health/liveliness        # -> alive
# a real model call through the gateway (use a model_name from litellm/config.yml):
curl http://localhost:4000/v1/chat/completions \
  -H "Authorization: Bearer $LITELLM_MASTER_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-main","messages":[{"role":"user","content":"ping"}]}'
```

Open http://localhost:8233 — Temporal UI, empty namespace `default` ready for workflows.

## Add the worker (when you have code)

The `worker` service builds from a `Dockerfile` in the repo (should build `cmd/worker`). Once
that exists:

```bash
make worker-up        # docker compose --profile worker up -d --build
```

It connects to `temporal:7233` and calls models via `http://litellm:4000`. Env contract is in
`docker-compose.yml` (Temporal address, LiteLLM base/key, Gitea hub + token, pinned vault/api refs).

## Add observability (Langfuse) — heavy, opt-in

~8 GB RAM, 6 extra containers. Start with LiteLLM's own spend/metrics first; add Langfuse when
you need step-level tracing of pipeline runs.

```bash
make obs-up           # core + Langfuse
```

Then: open http://localhost:3000 → create org/project → copy the API keys into `.env`
(`LANGFUSE_PUBLIC_KEY`, `LANGFUSE_SECRET_KEY`), uncomment `success_callback: ["langfuse"]` in
`litellm/config.yaml`, and `docker compose restart litellm`. Point the worker's
`OTEL_EXPORTER_OTLP_ENDPOINT` at Langfuse's OTLP ingest to get task→step→call spans.

> The overlay mirrors Langfuse's official self-host compose; image tags are loose on purpose.
> Pin them against the current file at https://langfuse.com/self-hosting before relying on it.

## Notes that will bite later if ignored

- **Pin image tags.** `temporalio/temporal:latest`, `litellm:main-stable`, `langfuse:3` are fine
  for first run; pin exact versions for reproducibility. For LiteLLM, avoid `1.82.7`/`1.82.8`
  (Mar-2026 supply-chain); use a clean tag.
- **Temporal here is a dev server** (in-process SQLite). Great for local. For a persistent /
  prod-shaped control-plane, swap to `temporalio/auto-setup` + a dedicated Postgres.
- **Bot identity.** `GITEA_TOKEN` is a service account on prod SourceControl: read `vcs-vault` /
  `vcs-api`, write-PR to the product repo, **no** merge to main, no admin. Don't use your
  personal token — runs and permissions get tangled.
- **Reproducibility.** Pin `VAULT_REF` / `API_REF` to commit SHAs and record them per run, so a
  run = (vcs-agents version) × (vault ref) × (api ref) × (sandbox image) × (prod-hub version).
- **`host.docker.internal`** (for a local vLLM/Ollama later) works on Docker Desktop; on plain
  Linux add `extra_hosts: ["host.docker.internal:host-gateway"]` to the litellm service.
