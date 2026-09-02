# TunnelBox

TunnelBox is a small local control plane for publishing HTTP and HTTPS origins
through Cloudflare Tunnel and Cloudflare Access. It keeps one workspace and
one independently managed `cloudflared` process per service.

The first release intentionally stays narrow: one Cloudflare account and zone,
Web origins only, and one explicit email or email-domain Allow condition per
service. SSH, TCP, WARP and shared/imported resources are out of scope.

## Run locally

Requirements: Go 1.26, Node.js 22+, and a `cloudflared` binary on `PATH`.

```sh
cd web && npm install && npm run build && cd ..
GOTOOLCHAIN=go1.26.0 go run ./cmd/tunnelbox
```

Open <http://127.0.0.1:8080>. The API listens on loopback by default, so a
local admin token is optional. For a LAN/public bind, set
`TUNNELBOX_ADMIN_TOKEN_FILE` to an owner-only (`0600`) file.

## Configuration

All settings are environment variables; the defaults are suitable for a local
checkout.

| Variable | Default | Purpose |
| --- | --- | --- |
| `TUNNELBOX_LISTEN` | `127.0.0.1:8080` | HTTP bind address |
| `TUNNELBOX_DATABASE` | `data/tunnelbox.db` | SQLite file |
| `TUNNELBOX_WORKSPACE_ID` | `default` | Workspace key |
| `TUNNELBOX_WORKSPACE_NAME` | `Default` | Workspace label |
| `TUNNELBOX_ADMIN_TOKEN_FILE` | empty | Local admin bearer token file |
| `TUNNELBOX_CLOUDFLARE_TOKEN_FILE` | `data/cloudflare.token` | Owner-only Cloudflare token file |
| `TUNNELBOX_CLOUDFLARED_BINARY` | `cloudflared` | Connector executable |
| `TUNNELBOX_CLOUDFLARED_DATA_DIR` | `data/cloudflared` | Connector token files |
| `TUNNELBOX_WEB_DIR` | `web/dist` | Built console assets |

Cloudflare API tokens are never written to SQLite, logs, or API responses.
The UI sends a token only to `PUT /api/v1/integrations/cloudflare`; the control
plane validates it and stores it in the configured `0600` file.

## Docker Compose

```sh
docker compose -f deploy/docker-compose.yml up --build
```

The example keeps the control plane bound to loopback and stores SQLite,
connector state, and the Cloudflare token in the named `tunnelbox-data` volume.
On first start it creates an owner-only administrator token in that volume.
Read it once with:

```sh
docker compose -f deploy/docker-compose.yml exec tunnelbox cat /app/data/admin.token
```

Paste that token into the console's administrator token field. Keep it private;
rotating it means replacing the file in the volume with another owner-only file.

## API

The complete contract is in [`docs/openapi.yaml`](docs/openapi.yaml). The usual
flow is:

```sh
curl -X PUT http://127.0.0.1:8080/api/v1/integrations/cloudflare \
  -H 'Content-Type: application/json' \
  -d '{"account_id":"...","zone_id":"...","token":"..."}'

curl -X POST http://127.0.0.1:8080/api/v1/services \
  -H 'Content-Type: application/json' \
  -d '{"name":"Docs","hostname":"docs.example.com","origin_url":"http://127.0.0.1:3000","allow_type":"email","allow_value":"you@example.com"}'

curl -X POST http://127.0.0.1:8080/api/v1/services/{id}/deploy
curl http://127.0.0.1:8080/api/v1/operations/{operation_id}
```

Deployment is asynchronous and records the current step in the operation row.
On restart, incomplete deployment operations are loaded from SQLite and
retried using the remote IDs already saved on the service. SQLite uses the
single built-in `PRAGMA user_version` value for its schema version; a separate
`schema_migrations` table is deliberately not needed for this single-process
MVP.

## Checks

```sh
GOTOOLCHAIN=go1.26.0 gofmt -w ./cmd ./internal
GOTOOLCHAIN=go1.26.0 go test ./...
GOTOOLCHAIN=go1.26.0 go test -race ./...
GOTOOLCHAIN=go1.26.0 go vet ./...
cd web && npm run build
```
