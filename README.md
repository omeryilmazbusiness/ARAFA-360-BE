# ARAFA-360

Multi-tenant Command Center backend for Mount Arafat logistics. Go 1.26, Fiber, PostgreSQL (pgx pool), Redis Pub/Sub, Sentry. Deployed on Railway.

```
domain/              entities, value objects, errors, ports (interfaces). Imports nothing from the app.
usecase/             application rules; depend on domain ports only
repository/          port implementations: postgres/ (pgx), redis/ (Pub/Sub broker)
delivery/http/       Fiber REST API, middleware, error mapping
delivery/websocket/  tenant-aware hub + upgrade handler
infrastructure/      config, pgx pool + migrator, redis client, Sentry, worker pool
migrations/          embedded SQL (applied at startup under an advisory lock)
main.go              composition root
```

`arch_test.go` fails the build if a layer imports something it must not (e.g. `usecase` importing `pgx`).

## Local development (macOS)

```bash
make setup      # .env + Postgres:5435 + Redis:6381 via docker compose
make run        # API on :8080 (migrates on boot)
make seed       # demo tenant 1111… and zone 2222… (run after the first boot)
make test-race
make help
```

Ports 5435/6381 avoid clashing with Homebrew's Postgres/Redis. If `:8080` is taken, `PORT=18080 make run`.

## API (all `/api/v1` routes need `X-Tenant-ID`)

| | |
|---|---|
| `POST /zones` | `{name, coordinates_polygon:[{lat,lng}…]}` |
| `POST /tents` | `{zone_id, type, capacity}` |
| `POST /tents/:id/beds` | `{count}` provisions beds with unique QR hashes (capacity-checked) |
| `POST /beds/scan` | `{qr_hash}` lookup |
| `POST /beds/check-in` | `{qr_hash, pilgrim_id}` atomic; 409 if occupied or pilgrim already has a bed |
| `POST /beds/check-out` | `{qr_hash}` |
| `POST /incidents` | `{tent_id, category, severity, message}` published to the dashboard |
| `GET /ws?tenant_id=…` | live event feed (`incident.reported`, `bed.checked_in`, `bed.checked_out`) |
| `GET /healthz`, `/readyz` | liveness / dependency readiness |

## Deploy on Railway

1. Add Postgres and Redis plugins; Railway injects `DATABASE_URL` and `REDIS_URL`.
2. Set `APP_ENV=production`, `ALLOWED_ORIGINS=https://your-dashboard`, `SENTRY_DSN`, `SENTRY_TRACES_SAMPLE_RATE=0.1`.
3. Keep `DB_MAX_CONNS` x replicas below the Postgres connection limit (~100).
4. Set `RAILWAY_DEPLOYMENT_DRAINING_SECONDS` to match `HTTP_SHUTDOWN_TIMEOUT` (default 20s) so shutdown can drain.
5. `railway.json` selects the Dockerfile and `/healthz` as the health check.

## Before production

- **Authentication.** The tenant comes from the `X-Tenant-ID` header, which is spoofable. Replace the body of `delivery/http.Tenant` with JWT/API-key verification. Nothing else changes.
- **Incident durability.** Redis Pub/Sub is at-most-once with no history; an offline dashboard misses alerts. Persist incidents (table) and/or use a Redis Stream so clients can replay on reconnect.
- **Rate limiting** per tenant / per device on the scan endpoints.
