# TunnelBox

TunnelBox 是一个轻量的本地控制面，用来把 HTTP/HTTPS Origin 发布到
Cloudflare Tunnel，并用 Cloudflare Access 做访问授权。每个服务由一个受控的
`cloudflared` 进程负责连接。

第一版只支持一个 Cloudflare Account/Zone、Web Origin，以及一个明确的邮箱或
邮箱域名 Allow 条件。SSH、TCP、WARP、共享或导入的远端资源暂不支持。

## 中文

### 先决条件

- Go 1.26
- Node.js 22 或更高版本（只在从源码构建管理界面时需要）
- 已安装并位于 `PATH` 的 `cloudflared`
- 一个拥有目标 Zone 的 Cloudflare 账号

### 本地启动

```sh
cd web
npm install
npm run build
cd ..
GOTOOLCHAIN=go1.26.0 go run ./cmd/tunnelbox
```

打开 <http://127.0.0.1:8080>。默认只监听本机回环地址，因此不需要本地管理员
令牌。若要监听局域网或公网地址，必须把 `TUNNELBOX_ADMIN_TOKEN_FILE` 指向权限
为 `0600` 的文件。

### 运行参数

所有配置都通过环境变量提供，默认值适合本地试用：

| 变量 | 默认值 | 用途 |
| --- | --- | --- |
| `TUNNELBOX_LISTEN` | `127.0.0.1:8080` | HTTP 监听地址 |
| `TUNNELBOX_DATABASE` | `data/tunnelbox.db` | SQLite 文件 |
| `TUNNELBOX_WORKSPACE_ID` | `default` | Workspace 标识 |
| `TUNNELBOX_WORKSPACE_NAME` | `Default` | Workspace 显示名称 |
| `TUNNELBOX_ADMIN_TOKEN_FILE` | 空 | 本地管理员 Bearer Token 文件；非回环监听时必填 |
| `TUNNELBOX_CLOUDFLARE_TOKEN_FILE` | `data/cloudflare.token` | 权限为 `0600` 的 Cloudflare Token 文件 |
| `TUNNELBOX_CLOUDFLARED_BINARY` | `cloudflared` | Connector 可执行文件 |
| `TUNNELBOX_CLOUDFLARED_DATA_DIR` | `data/cloudflared` | Connector 状态和 Token 文件目录 |
| `TUNNELBOX_WEB_DIR` | `web/dist` | 管理界面静态文件目录 |

### Docker Compose

```sh
docker compose -f deploy/docker-compose.yml up --build
```

示例 Compose 默认仍只暴露 `127.0.0.1:8080`，并把 SQLite、Connector 状态和
Cloudflare Token 保存在 `tunnelbox-data` 卷中。首次启动会在卷中生成本地管理员
令牌，读取一次并粘贴到页面右上角：

```sh
docker compose -f deploy/docker-compose.yml exec tunnelbox cat /app/data/admin.token
```

管理员令牌只用于保护 TunnelBox 控制面；它和下面的 Cloudflare API Token 不是同一
个东西。

### 第一次使用

1. 在 Cloudflare 准备 Account ID、Zone ID 和自定义 API Token（获取位置见下表）。
2. 打开页面右上角的「使用指南」，或点击 Cloudflare 区域的「配置连接」。
3. 填入 Account ID、Zone ID 和 API Token，点击「保存并验证」。验证成功后，页面
   会加载该 Token 可见的 Zone。
4. 点击「新建服务」，填写服务名称、公网域名、Origin URL 和 Allow 条件。
5. 点击服务行的「部署」。TunnelBox 会依次创建或复用 Tunnel、配置 Web Route、
   启动 Connector、创建 Access Application/Allow Policy，最后创建 DNS CNAME。
6. 在操作面板等待完成，然后访问公网域名。Cloudflare Access 会先要求访问者
   登录，并只允许填写的邮箱或邮箱域名。

### 去哪里获取信息

| 信息 | 获取位置 |
| --- | --- |
| Account ID、Zone ID | Cloudflare Dashboard 的 Account/Zone 概览；也可参考官方说明：[Find your account and zone IDs](https://developers.cloudflare.com/fundamentals/account/find-account-and-zone-ids/) |
| API Token | [官方创建说明](https://developers.cloudflare.com/fundamentals/api/get-started/create-token/)，然后在 [Profile → API Tokens](https://dash.cloudflare.com/profile/api-tokens) → Create Token → Create Custom Token |
| `cloudflared` | [安装 Cloudflare Tunnel connector](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/downloads/) |

自定义 API Token 至少需要以下权限：

```text
Account / Cloudflare Tunnel / Edit
Account / Access: Apps and Policies / Edit
Zone / DNS / Edit
Zone / Zone / Read
```

Token 建议只授权目标 Account/Zone，并设置合理的过期时间。不要使用 Global API
Key。TunnelBox 会在保存前调用 Cloudflare 验证接口，Token 不会写入 SQLite、日志
或 API 响应；仅保存到配置的权限为 `0600` 的 Secret 文件。

### 创建服务时怎么填

| 字段 | 说明 | 示例 |
| --- | --- | --- |
| 名称 | 仅用于控制台识别 | `内网文档` |
| 公网域名 | 目标 Zone 下尚未被其他记录占用的完整域名 | `docs.example.com` |
| Origin URL | Connector 所在机器可以访问的 HTTP/HTTPS 地址；不能带用户名密码 | `http://192.168.1.20:8080` |
| 允许条件 | Access 的 Allow 条件类型 | 指定邮箱或邮箱域名 |
| 条件值 | 与类型对应的邮箱地址或域名 | `you@example.com` / `example.com` |

Origin 必须从运行 `cloudflared` 的机器可达。`localhost` 指 Connector 容器或主机
自身，不是访问者的电脑；Docker 部署时通常要填写同一网络中 Origin 的服务名或
宿主机可达地址。

### API 快速示例

完整契约见 [`docs/openapi.yaml`](docs/openapi.yaml)。控制面令牌启用时，在受保护
请求中加入 `Authorization: Bearer <admin-token>`：

```sh
curl -X PUT http://127.0.0.1:8080/api/v1/integrations/cloudflare \
  -H 'Authorization: Bearer <admin-token>' \
  -H 'Content-Type: application/json' \
  -d '{"account_id":"<account-id>","zone_id":"<zone-id>","token":"<cloudflare-api-token>"}'

curl -X POST http://127.0.0.1:8080/api/v1/services \
  -H 'Authorization: Bearer <admin-token>' \
  -H 'Content-Type: application/json' \
  -d '{"name":"Docs","hostname":"docs.example.com","origin_url":"http://127.0.0.1:3000","allow_type":"email","allow_value":"you@example.com"}'

curl -X POST http://127.0.0.1:8080/api/v1/services/SERVICE_ID/deploy \
  -H 'Authorization: Bearer <admin-token>'
curl http://127.0.0.1:8080/api/v1/operations/OPERATION_ID \
  -H 'Authorization: Bearer <admin-token>'
```

部署接口返回 `202 Accepted`，客户端轮询操作资源获取进度。控制面重启后会从
SQLite 恢复未完成操作，并使用已保存的远端资源 ID 继续执行；重复部署会复用已有
资源，而不是无条件创建新的 Tunnel、Access 或 DNS 记录。

### 常见问题

- **页面提示需要管理员令牌**：本地启动默认没有令牌；Compose 启动时读取
  `/app/data/admin.token`，粘贴到右上角输入框。
- **Token 验证失败**：确认使用的是 API Token 而不是 Global API Key，并检查四项
  权限、Account/Zone 范围和 Token 是否已过期或被撤销。
- **Origin 不可达**：从运行 `cloudflared` 的同一环境测试 Origin URL；检查协议、
  端口、防火墙和容器网络。
- **Access 拒绝访问**：确认登录邮箱与 Allow 条件完全匹配；域名条件只匹配邮箱
  域名，不会自动允许所有 Cloudflare 用户。
- **DNS 尚未生效**：等待 DNS 缓存刷新，并在部署操作面板确认最后一步已完成。

### 安全边界与当前范围

控制面默认绑定 `127.0.0.1:8080`。非回环监听必须启用管理员认证；服务访问者由
Cloudflare Access 单独授权。Origin 只接受 `http`/`https`，不接受 shell、文件或
脚本协议。删除、解绑和 Token 轮换应在受控环境中执行并保管备份。

当前 MVP 使用单个 SQLite `PRAGMA user_version` 保存 schema 版本，不建立额外的
`schema_migrations` 表；数据表和远端资源引用均服务于单进程控制面。

### 开发检查

```sh
GOTOOLCHAIN=go1.26.0 gofmt -w ./cmd ./internal
GOTOOLCHAIN=go1.26.0 go test ./...
GOTOOLCHAIN=go1.26.0 go test -race ./...
GOTOOLCHAIN=go1.26.0 go vet ./...
cd web && npm run build
```

## English

### What it does

TunnelBox is a small local control plane for publishing HTTP/HTTPS origins through
Cloudflare Tunnel and protecting them with Cloudflare Access. Each service owns a
managed `cloudflared` connector process.

The MVP supports one Cloudflare account and zone, Web origins only, and one explicit
email or email-domain Allow condition per service. SSH, TCP, WARP, and shared/imported
remote resources are out of scope.

### Prerequisites and local run

You need Go 1.26, Node.js 22+ for building the console, a `cloudflared` binary on
`PATH`, and a Cloudflare account that owns the target zone.

```sh
cd web
npm install
npm run build
cd ..
GOTOOLCHAIN=go1.26.0 go run ./cmd/tunnelbox
```

Open <http://127.0.0.1:8080>. Loopback is the default, so a local admin token is not
required. For a LAN or public bind, set `TUNNELBOX_ADMIN_TOKEN_FILE` to an owner-only
(`0600`) file.

### Configuration

All settings are environment variables; the defaults are suitable for a local checkout.

| Variable | Default | Purpose |
| --- | --- | --- |
| `TUNNELBOX_LISTEN` | `127.0.0.1:8080` | HTTP bind address |
| `TUNNELBOX_DATABASE` | `data/tunnelbox.db` | SQLite file |
| `TUNNELBOX_WORKSPACE_ID` | `default` | Workspace key |
| `TUNNELBOX_WORKSPACE_NAME` | `Default` | Workspace label |
| `TUNNELBOX_ADMIN_TOKEN_FILE` | empty | Local admin bearer-token file; required for non-loopback binds |
| `TUNNELBOX_CLOUDFLARE_TOKEN_FILE` | `data/cloudflare.token` | Owner-only Cloudflare API Token file |
| `TUNNELBOX_CLOUDFLARED_BINARY` | `cloudflared` | Connector executable |
| `TUNNELBOX_CLOUDFLARED_DATA_DIR` | `data/cloudflared` | Connector state and token files |
| `TUNNELBOX_WEB_DIR` | `web/dist` | Built console assets |

### Docker Compose

```sh
docker compose -f deploy/docker-compose.yml up --build
```

The example binds the control plane to loopback and stores SQLite, connector state, and
the Cloudflare token in the named `tunnelbox-data` volume. On first start it creates a
local admin token:

```sh
docker compose -f deploy/docker-compose.yml exec tunnelbox cat /app/data/admin.token
```

Paste that value into the administrator-token field. It protects the TunnelBox control
plane and is separate from the Cloudflare API Token used for provisioning.

### First-run checklist

1. Prepare the Account ID, Zone ID, and a custom Cloudflare API Token (see below).
2. Open **使用指南** in the top-right corner, or choose **配置连接** in the Cloudflare
   section.
3. Enter the three values and choose **保存并验证**. After validation, TunnelBox loads
   the zones visible to the token.
4. Choose **新建服务** and enter a service name, public hostname, Origin URL, and an
   Allow condition.
5. Choose **部署**. TunnelBox creates or reuses the Tunnel, applies the Web route,
   starts the connector, creates the Access application and Allow policy, and creates
   the DNS CNAME last.
6. Wait for the operation panel to finish, then open the public hostname. Cloudflare
   Access will authenticate the visitor and enforce the configured email condition.

### Where to find the values

| Value | Where to get it |
| --- | --- |
| Account ID and Zone ID | Cloudflare Dashboard account/zone overview, or [Find your account and zone IDs](https://developers.cloudflare.com/fundamentals/account/find-account-and-zone-ids/) |
| API Token | Follow the [official token guide](https://developers.cloudflare.com/fundamentals/api/get-started/create-token/), then open [Profile → API Tokens](https://dash.cloudflare.com/profile/api-tokens) → Create Token → Create Custom Token |
| `cloudflared` | [Cloudflare Tunnel connector downloads](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/downloads/) |

The custom token needs at least:

```text
Account / Cloudflare Tunnel / Edit
Account / Access: Apps and Policies / Edit
Zone / DNS / Edit
Zone / Zone / Read
```

Scope the token to the target account and zone and set an expiration date. Do not use a
Global API Key. TunnelBox verifies the token before saving it; the token is never stored
in SQLite, logs, or API responses and is written only to the configured `0600` secret
file.

### Service fields

| Field | Meaning | Example |
| --- | --- | --- |
| Name | Console label | `Internal docs` |
| Public hostname | Full hostname in the selected zone | `docs.example.com` |
| Origin URL | HTTP/HTTPS address reachable from the connector, without credentials | `http://192.168.1.20:8080` |
| Allow condition | Access condition type | Email or email domain |
| Condition value | Matching address or domain | `you@example.com` / `example.com` |

The Origin must be reachable from the machine or container running `cloudflared`.
`localhost` means that connector environment, not the visitor's computer. With Docker,
use an Origin service name on the same network or another address reachable from the
container.

### API and operations

The full contract is in [`docs/openapi.yaml`](docs/openapi.yaml). Add
`Authorization: Bearer <admin-token>` when admin authentication is enabled:

```sh
curl -X PUT http://127.0.0.1:8080/api/v1/integrations/cloudflare \
  -H 'Authorization: Bearer <admin-token>' \
  -H 'Content-Type: application/json' \
  -d '{"account_id":"<account-id>","zone_id":"<zone-id>","token":"<cloudflare-api-token>"}'

curl -X POST http://127.0.0.1:8080/api/v1/services \
  -H 'Authorization: Bearer <admin-token>' \
  -H 'Content-Type: application/json' \
  -d '{"name":"Docs","hostname":"docs.example.com","origin_url":"http://127.0.0.1:3000","allow_type":"email","allow_value":"you@example.com"}'

curl -X POST http://127.0.0.1:8080/api/v1/services/SERVICE_ID/deploy \
  -H 'Authorization: Bearer <admin-token>'
curl http://127.0.0.1:8080/api/v1/operations/OPERATION_ID \
  -H 'Authorization: Bearer <admin-token>'
```

Deployment is asynchronous and returns `202 Accepted`; poll the operation resource for
progress. Incomplete operations are resumed after a restart, and repeated deployments
reuse stored remote IDs instead of blindly creating duplicate resources.

### Troubleshooting and scope

- **Admin token required**: read `/app/data/admin.token` from the Compose volume and
  paste it into the top-right field.
- **Token validation failed**: use an API Token, not a Global API Key; check all four
  permissions, account/zone scope, expiration, and revocation state.
- **Origin unreachable**: test the URL from the connector environment and check protocol,
  port, firewall, and container networking.
- **Access denied**: verify that the signed-in email exactly matches the Allow condition.
- **DNS not ready**: allow DNS caches to refresh and confirm the final deployment step.

The control plane defaults to `127.0.0.1:8080`; non-loopback binds require admin
authentication. Access visitors and control-plane administrators are separate trust
boundaries. Origins accept only `http` and `https`.

The single-process MVP stores schema version in SQLite `PRAGMA user_version` and does not
create a separate `schema_migrations` table.

Run the development checks:

```sh
GOTOOLCHAIN=go1.26.0 gofmt -w ./cmd ./internal
GOTOOLCHAIN=go1.26.0 go test ./...
GOTOOLCHAIN=go1.26.0 go test -race ./...
GOTOOLCHAIN=go1.26.0 go vet ./...
cd web && npm run build
```
