# TunnelBox

[简体中文](README.zh-CN.md)

TunnelBox is a small local control plane for exposing an internal HTTP/HTTPS
service through Cloudflare Tunnel. It runs `cloudflared` as a managed process,
creates the required Cloudflare resources, and reports deployment progress from
one web console.

Choose the mode that matches the way people should reach the service:

| Mode | Public domain | Visitor access | Cloudflare Access |
| --- | --- | --- | --- |
| `quick` | Not required | Temporary `trycloudflare.com` URL | Not available |
| `private` | Not required | Private IP through Cloudflare One Client/WARP | Email or email-domain policy |
| `public` | Required | Hostname in a zone you own | Email or email-domain policy |

No public domain is required for TunnelBox. Use `quick` for a temporary browser
link, or `private` when visitors can use WARP and you need identity-based access
control. Use `public` only when you own the target Cloudflare zone.

## Features

- Manage Web/HTTP/HTTPS origins and `cloudflared` Connector processes.
- Support three explicit modes: `quick`, `private`, and `public`.
- Create Cloudflare Access Allow policies for an email or email domain.
- Track asynchronous deployments and resume incomplete operations after a restart.
- Stop a running Connector without deleting its Cloudflare resources, then deploy it again later.
- Store local state in SQLite; Cloudflare API Tokens are written only to an owner-only
  (`0600`) Secret file.

The MVP supports Web origins only. SSH, TCP, RDP, independent remote Connectors, and
unmanaged remote resources are out of scope.

## Quick start

### Docker Compose (recommended)

```sh
docker compose -f deploy/docker-compose.yml up --build -d
```

Open <http://127.0.0.1:8080> and create an administrator password on first use. The
console keeps a secure, persistent session cookie after login. The example exposes the
control plane on loopback, persists SQLite, Connector state, and Secrets in the
`tunnelbox-data` volume, and includes `cloudflared` in the image.

### From source

Install Go 1.26, Node.js 22+, and `cloudflared` on `PATH`:

```sh
cd web && npm ci && npm run build
cd ..
go run ./cmd/tunnelbox
```

Open <http://127.0.0.1:8080> and create an administrator password on first use. Subsequent
visits use a secure, persistent session cookie. For a LAN or public bind, protect the
endpoint with a reverse proxy or network policy; the application itself uses the same
password login.

## First deployment

1. Make sure the machine or container running TunnelBox can reach the Origin. The
   Origin does not need a public IP; `localhost` means the Connector environment.
2. In **New service**, choose one mode:
   - **Quick** needs no Cloudflare account, domain, or token. It returns a temporary
     `https://*.trycloudflare.com` URL and does not support standard Access policies.
   - **Private** needs an Account ID and API Token, but no public domain. Visitors must
     join the same Zero Trust organization, use One Client/WARP, and route the target
     IP/CIDR through WARP with Split Tunnels.
   - **Public** needs a zone you own, an Account ID, and an API Token. TunnelBox creates
     the Access application and DNS CNAME for a normal browser hostname.
3. Leave Zone ID empty for Private; enter the target Zone ID for Public. Quick can skip
   Cloudflare configuration. Saving the connection validates the token first.
4. Enter the service name and Origin URL. Private uses the Origin host's private IP;
   Public uses a full hostname in the selected zone; Quick needs neither hostname nor
   Allow condition.
5. Deploy and follow the operation panel. Repeated deployments reuse stored remote IDs.
6. Verify the mode-specific entry: open the Quick URL, use WARP for the Private IP, or
   open the Public hostname and complete the Access login.

To temporarily take a service offline, click **Stop** in its row. This stops the local
Connector and keeps the Tunnel, Access policy, DNS record, and service settings. Click
**Deploy** later to bring it back.

To remove a service, click **Delete** and confirm. A draft or stopped service with no
remote references is removed immediately. A managed service first stops its Connector,
then removes only the Cloudflare resources recorded for that service, and finally removes
the local record. The operation panel reports each cleanup step; failed cleanup keeps the
service and remaining IDs so you can retry after fixing the credentials or permissions.

Private is not a public DNS entry. Complete Zero Trust enrollment and configure
[Split Tunnels](https://developers.cloudflare.com/cloudflare-one/team-and-resources/devices/cloudflare-one-client/configure/route-traffic/split-tunnels/) so the target private IP is sent through WARP.

## Cloudflare values

- Find the Account ID and Zone ID in the [Cloudflare dashboard](https://developers.cloudflare.com/fundamentals/account/find-account-and-zone-ids/).
- Create a scoped API Token from [Profile → API Tokens](https://dash.cloudflare.com/profile/api-tokens), following the [official token guide](https://developers.cloudflare.com/fundamentals/api/get-started/create-token/).
- Install [`cloudflared`](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/downloads/).
- Install [Cloudflare One Client/WARP](https://developers.cloudflare.com/cloudflare-one/connections/connect-devices/warp/) for Private visitors.

Grant only the permissions required by the selected mode. Private needs Account-level
Tunnel, Access/Zero Trust write permissions; Public also needs Zone DNS edit and Zone
read permissions. Do not use a Global API Key. Tokens are stored only in the configured
owner-only Secret file, never in SQLite, logs, or API responses.

## Configuration

All settings are environment variables; the defaults are suitable for a local checkout:

| Variable | Default | Purpose |
| --- | --- | --- |
| `TUNNELBOX_LISTEN` | `127.0.0.1:8080` | HTTP bind address |
| `TUNNELBOX_DATABASE` | `data/tunnelbox.db` | SQLite file |
| `TUNNELBOX_CLOUDFLARE_TOKEN_FILE` | `data/cloudflare.token` | Cloudflare API Token file |
| `TUNNELBOX_CLOUDFLARED_BINARY` | `cloudflared` | Connector executable |
| `TUNNELBOX_CLOUDFLARED_DATA_DIR` | `data/cloudflared` | Connector state directory |
| `TUNNELBOX_WEB_DIR` | `web/dist` | Built console assets |

`TUNNELBOX_WORKSPACE_ID` and `TUNNELBOX_WORKSPACE_NAME` can override the default
Workspace (`default` and `Default`).

## API and development

See [`docs/openapi.yaml`](docs/openapi.yaml) for the complete API contract. Deployment
returns `202 Accepted`; poll `/api/v1/operations/:id` for progress. Authenticate by first
calling `/api/v1/auth/setup` (first run) or `/api/v1/auth/login`; the server sets an
HttpOnly session cookie.

```sh
curl -X POST http://127.0.0.1:8080/api/v1/services \
  -H 'Content-Type: application/json' \
  -d '{"mode":"quick","name":"Local preview","origin_url":"http://127.0.0.1:3000"}'
curl -X POST http://127.0.0.1:8080/api/v1/services/SERVICE_ID/deploy
curl http://127.0.0.1:8080/api/v1/operations/OPERATION_ID
# Stop the Connector later without deleting the service or Cloudflare resources
curl -X POST http://127.0.0.1:8080/api/v1/services/SERVICE_ID/stop
# Delete a service (returns 204 for a local delete or 202 with an operation)
curl -X DELETE http://127.0.0.1:8080/api/v1/services/SERVICE_ID
```

Health checks are available at `/healthz` and `/readyz`.

Run the checks before submitting changes:

```sh
gofmt -w ./cmd ./internal
go test ./...
go test -race ./...
go vet ./...
cd web && npm run build
```
