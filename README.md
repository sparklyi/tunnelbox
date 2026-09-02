# TunnelBox

TunnelBox is a small local control plane for publishing one or more internal
HTTP services through Cloudflare Tunnel and Cloudflare Access.

## v1 scope

- Single-node, single workspace, one Cloudflare account and zone.
- One service owns one remotely managed Tunnel and one `cloudflared` process.
- HTTP and HTTPS origins only.
- One explicit email or email-domain Allow condition per service.
- Cloudflare full DNS setup only.

Team membership, imported or shared tunnels, non-HTTP services, and partial
CNAME setup are deliberately outside the first release.

## Development

The repository targets Go 1.26. On hosts with another Go version installed,
use the toolchain directive supported by Go:

```sh
GOTOOLCHAIN=go1.26.0 go test ./...
```

The API listens on `127.0.0.1:8080` by default. Configuration is documented in
`internal/config` and the Compose example in `deploy/docker-compose.yml`.
