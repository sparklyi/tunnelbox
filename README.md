# TunnelBox

TunnelBox 是一个轻量的本地控制面，用来把内网 HTTP/HTTPS Origin 快速发布到外网。
每个服务由一个受控的 `cloudflared` 进程负责连接，并按选择的模式决定访问边界：

| 模式 | 需要公网域名 | 访问方式 | Cloudflare Access |
| --- | --- | --- | --- |
| `quick` 临时公开 | 否 | 随机 `trycloudflare.com` 地址，浏览器可直接打开 | 不支持标准 Access 策略 |
| `private` 私网受控 | 否 | Cloudflare One Client/WARP 访问私网 IP | 支持邮箱/域名 Allow |
| `public` 自有域名 | 是 | 你的 Zone 下的公网主机名 | 支持邮箱/域名 Allow |

因此，**没有公网域名也可以使用 TunnelBox**：只想临时分享就选 Quick；需要
Cloudflare 身份和权限控制就选 Private，并让访问者安装 WARP。公网域名只在
Public 模式需要。第一版只支持 Web Origin，不支持 SSH、TCP 和共享/导入的远端资源。

## 中文

### 先决条件

- Go 1.26
- Node.js 22 或更高版本（只在从源码构建管理界面时需要）
- 已安装并位于 `PATH` 的 `cloudflared`
- Quick 模式不需要 Cloudflare 账号；Private/Public 模式需要 Cloudflare 账号和 API Token

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

1. 先安装 `cloudflared`，确认运行 TunnelBox 的机器可以访问你的 Origin。Origin
   不需要公网 IP；`localhost` 指 Connector 所在环境。
2. 在「新建服务」里选择一种模式：
   - **Quick / 临时公开**：不填 Cloudflare 信息，保存后直接部署；完成会得到随机
     `https://*.trycloudflare.com` 地址。它适合测试和临时分享，不提供标准 Access。
   - **Private / 私网受控**：不需要公网域名，但要先配置 Account ID 和 API Token；访问者
     需要加入同一个 Cloudflare Zero Trust 组织并使用 Cloudflare One Client/WARP。
     WARP 默认排除 RFC1918 私网段，还要在 Split Tunnels 中把目标 IP/CIDR 路由到 WARP，
     Access 才会按邮箱条件授权私网 IP。
   - **Public / 自有域名**：需要你拥有的 Zone ID、Account ID 和 API Token；TunnelBox
     会创建 Access 和 DNS，普通浏览器可访问你的域名。
3. 打开右上角「使用指南」或「配置连接」。Quick 可以跳过这一步；Private 留空
   Zone ID，Public 填写目标 Zone ID，点击「保存并验证」。
4. 填写服务名称和 Origin URL。Private 还要填写与 Origin 主机一致的私网 IP；Public
   填写完整公网主机名和 Allow 条件；Quick 不需要域名或 Allow 条件。
5. 点击「部署」，在操作面板等待完成。重复部署会复用已保存的远端资源，不会无条件
   创建重复 Tunnel、Access 或 DNS。
6. 按模式验证入口：打开 Quick 返回的临时地址；用 WARP 访问 Private 的私网 IP；或
   用普通浏览器打开 Public 域名并完成 Access 登录。

### 去哪里获取信息

| 信息 | 获取位置 |
| --- | --- |
| Account ID | Cloudflare Dashboard 的 Account 概览；也可参考官方说明：[Find your account and zone IDs](https://developers.cloudflare.com/fundamentals/account/find-account-and-zone-ids/) |
| Zone ID（仅 Public） | 目标 Zone 的 Overview 页面；Private/Quick 不需要 |
| API Token（Private/Public） | [官方创建说明](https://developers.cloudflare.com/fundamentals/api/get-started/create-token/)，然后在 [Profile → API Tokens](https://dash.cloudflare.com/profile/api-tokens) → Create Token → Create Custom Token |
| `cloudflared` | [安装 Cloudflare Tunnel connector](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/downloads/) |
| WARP / One Client（Private 访问者） | [Cloudflare One Client 下载与说明](https://developers.cloudflare.com/cloudflare-one/connections/connect-devices/warp/) |
| Zero Trust 组织、设备注册和 Split Tunnels（Private） | [Connect an IP/CIDR](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/private-net/cloudflared/connect-cidr/) |

按模式授予最小权限：

```text
Private:
Account / Cloudflare Tunnel / Edit
Account / Access: Apps and Policies / Edit
Account / Zero Trust / Write

Public（在 Private 基础上增加）：
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
| 模式 | `quick`、`private` 或 `public` | `quick` |
| 私网 IP（Private） | 与 Origin 主机一致的 RFC1918/IPv6 私网地址 | `192.168.1.20` |
| 公网域名（Public） | 目标 Zone 下尚未被其他记录占用的完整域名 | `docs.example.com` |
| Origin URL | Connector 所在机器可以访问的 HTTP/HTTPS 地址；不能带用户名密码 | `http://192.168.1.20:8080` |
| 允许条件（Private/Public） | Access 的 Allow 条件类型 | 指定邮箱或邮箱域名 |
| 条件值（Private/Public） | 与类型对应的邮箱地址或域名 | `you@example.com` / `example.com` |

Origin 必须从运行 `cloudflared` 的机器可达。Quick 不会创建 Access；Private 的
入口只对已连接 WARP 的设备开放；Public 才会创建 DNS CNAME。不要为了通过校验而
填写假的 Zone ID 或公网域名。

Private 的访问链路不是普通公网 DNS：先在 Zero Trust 中完成组织和设备 enrollment，
再按 [Split Tunnels 配置说明](https://developers.cloudflare.com/cloudflare-one/team-and-resources/devices/cloudflare-one-client/configure/route-traffic/split-tunnels/) 让目标私网 IP 经过 WARP。否则 Tunnel 和 Access 都可能已创建，但访问者仍然无法连到私网地址。

### API 快速示例

完整契约见 [`docs/openapi.yaml`](docs/openapi.yaml)。控制面令牌启用时，在受保护
请求中加入 `Authorization: Bearer <admin-token>`：

```sh
# Quick：无域名、无 Cloudflare 配置即可创建
curl -X POST http://127.0.0.1:8080/api/v1/services \
  -H 'Authorization: Bearer <admin-token>' \
  -H 'Content-Type: application/json' \
  -d '{"mode":"quick","name":"本地预览","origin_url":"http://127.0.0.1:3000"}'

# Private：无域名，但需要 Account ID/Token，访问者使用 WARP
curl -X POST http://127.0.0.1:8080/api/v1/services \
  -H 'Authorization: Bearer <admin-token>' \
  -H 'Content-Type: application/json' \
  -d '{"mode":"private","name":"内网文档","hostname":"192.168.1.20","origin_url":"http://192.168.1.20:8080","allow_type":"email","allow_value":"you@example.com"}'

# Public：只有在拥有 Zone 时才填写 zone_id 和公网域名
curl -X PUT http://127.0.0.1:8080/api/v1/integrations/cloudflare \
  -H 'Authorization: Bearer <admin-token>' \
  -H 'Content-Type: application/json' \
  -d '{"account_id":"<account-id>","zone_id":"<zone-id>","token":"<cloudflare-api-token>"}'

curl -X POST http://127.0.0.1:8080/api/v1/services \
  -H 'Authorization: Bearer <admin-token>' \
  -H 'Content-Type: application/json' \
  -d '{"mode":"public","name":"Docs","hostname":"docs.example.com","origin_url":"http://127.0.0.1:3000","allow_type":"email","allow_value":"you@example.com"}'

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
- **Token 验证失败**：确认使用的是 API Token 而不是 Global API Key，并检查按模式
  配置的权限、Account/Zone 范围和 Token 是否已过期或被撤销。
- **没有公网域名**：选择 Quick 可直接得到临时地址；若必须使用 Access 身份策略，
  选择 Private，并让访问者安装并登录 Cloudflare One Client/WARP。Quick 地址不能
  绑定标准 Access 应用。
- **Origin 不可达**：从运行 `cloudflared` 的同一环境测试 Origin URL；检查协议、
  端口、防火墙和容器网络。
- **Access 拒绝访问**：确认登录邮箱与 Allow 条件完全匹配；域名条件只匹配邮箱
  域名，不会自动允许所有 Cloudflare 用户。
- **Private 无法访问**：确认设备已连接 WARP，私网 IP 与 Origin 主机一致；Private
  不是普通公网 DNS 入口。
- **DNS 尚未生效**：等待 DNS 缓存刷新，并在部署操作面板确认最后一步已完成。

### 安全边界与当前范围

控制面默认绑定 `127.0.0.1:8080`。非回环监听必须启用管理员认证；Private/Public
服务访问者由 Cloudflare Access 授权，Quick 使用临时地址边界。Origin 只接受
`http`/`https`，不接受 shell、文件或脚本协议。删除、解绑和 Token 轮换应在受控环境
中执行并保管备份。

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

TunnelBox is a small local control plane for quickly publishing internal HTTP/HTTPS
origins to the Internet. Each service owns a managed `cloudflared` connector process.

It has three explicit exposure modes:

| Mode | Public domain required | Visitor path | Cloudflare Access |
| --- | --- | --- | --- |
| `quick` | No | Random `trycloudflare.com` URL in any browser | No standard Access policy |
| `private` | No | Private IP through Cloudflare One Client/WARP | Email/email-domain Allow |
| `public` | Yes | Hostname in a zone you own | Email/email-domain Allow |

You can therefore use TunnelBox without owning a domain. Choose Quick for a temporary
share, or Private when identity and policy control matter and visitors can use WARP.
The MVP supports Web origins only; SSH, TCP, and imported remote resources are out of scope.

### Prerequisites and local run

You need Go 1.26, Node.js 22+ for building the console, and a `cloudflared` binary on
`PATH`. Quick mode needs no Cloudflare account. Private and Public modes need a
Cloudflare account and API Token.

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

1. Install `cloudflared` and make sure the Connector environment can reach the Origin.
   The Origin does not need a public IP; `localhost` means the Connector environment.
2. In **New service**, choose a mode:
   - **Quick**: no domain, account, or token. Deployment returns a random
     `https://*.trycloudflare.com` URL for temporary sharing; standard Access policies do
     not apply.
   - **Private**: no public domain, but configure an Account ID and API Token. Visitors must
     enroll in the same Cloudflare Zero Trust organization and use Cloudflare One Client/WARP.
     Because WARP excludes RFC1918 ranges by default, add the target IP/CIDR to Split Tunnels
     so Access can apply the email condition to the private IP.
   - **Public**: configure Account ID, Zone ID, and API Token. TunnelBox creates Access
     and DNS so a normal browser can use your hostname.
3. Open **使用指南** or **配置连接**. Quick can skip it; leave Zone ID empty for Private
   and enter the target Zone ID for Public, then choose **保存并验证**.
4. Enter the service name and Origin URL. Private also needs the matching private IP;
   Public needs a full hostname and Allow condition; Quick needs neither.
5. Choose **Deploy** and wait for the operation panel. Repeated deployments reuse stored
   remote IDs instead of blindly creating duplicate resources.
6. Verify the mode-specific entry: open the Quick URL, use WARP for the Private IP, or
   open the Public hostname and complete the Access login.

### Where to find the values

| Value | Where to get it |
| --- | --- |
| Account ID | Cloudflare Dashboard account overview, or [Find your account and zone IDs](https://developers.cloudflare.com/fundamentals/account/find-account-and-zone-ids/) |
| Zone ID (Public only) | The target zone's Overview page; not needed for Private/Quick |
| API Token (Private/Public) | Follow the [official token guide](https://developers.cloudflare.com/fundamentals/api/get-started/create-token/), then open [Profile → API Tokens](https://dash.cloudflare.com/profile/api-tokens) → Create Token → Create Custom Token |
| `cloudflared` | [Cloudflare Tunnel connector downloads](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/downloads/) |
| WARP / One Client (Private visitors) | [Cloudflare One Client documentation](https://developers.cloudflare.com/cloudflare-one/connections/connect-devices/warp/) |
| Zero Trust organization, device enrollment, and Split Tunnels (Private) | [Connect an IP/CIDR](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/private-net/cloudflared/connect-cidr/) |

Grant the minimum permissions for the selected mode:

```text
Private:
Account / Cloudflare Tunnel / Edit
Account / Access: Apps and Policies / Edit
Account / Zero Trust / Write

Public (add these to Private):
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
| Mode | `quick`, `private`, or `public` | `quick` |
| Private IP (Private) | RFC1918/IPv6 private address matching the Origin host | `192.168.1.20` |
| Public hostname (Public) | Full hostname in the selected zone | `docs.example.com` |
| Origin URL | HTTP/HTTPS address reachable from the connector, without credentials | `http://192.168.1.20:8080` |
| Allow condition (Private/Public) | Access condition type | Email or email domain |
| Condition value (Private/Public) | Matching address or domain | `you@example.com` / `example.com` |

The Origin must be reachable from the machine or container running `cloudflared`.
Quick creates no Access policy; Private exposes only to WARP-connected devices; Public
creates the DNS CNAME. Do not enter a fake Zone ID or public hostname just to pass validation.

Private is not a normal public DNS entry. First finish Zero Trust organization and device
enrollment, then configure [Split Tunnels](https://developers.cloudflare.com/cloudflare-one/team-and-resources/devices/cloudflare-one-client/configure/route-traffic/split-tunnels/) so the target private IP is sent through WARP. Without that route, the Tunnel and Access resources may exist while the visitor still cannot connect.

### API and operations

The full contract is in [`docs/openapi.yaml`](docs/openapi.yaml). Add
`Authorization: Bearer <admin-token>` when admin authentication is enabled:

```sh
# Quick: no domain or Cloudflare configuration
curl -X POST http://127.0.0.1:8080/api/v1/services \
  -H 'Authorization: Bearer <admin-token>' \
  -H 'Content-Type: application/json' \
  -d '{"mode":"quick","name":"Local preview","origin_url":"http://127.0.0.1:3000"}'

# Private: no domain, but visitors use WARP
curl -X POST http://127.0.0.1:8080/api/v1/services \
  -H 'Authorization: Bearer <admin-token>' \
  -H 'Content-Type: application/json' \
  -d '{"mode":"private","name":"Internal docs","hostname":"192.168.1.20","origin_url":"http://192.168.1.20:8080","allow_type":"email","allow_value":"you@example.com"}'

# Public: only use this when you own the target zone
curl -X PUT http://127.0.0.1:8080/api/v1/integrations/cloudflare \
  -H 'Authorization: Bearer <admin-token>' \
  -H 'Content-Type: application/json' \
  -d '{"account_id":"<account-id>","zone_id":"<zone-id>","token":"<cloudflare-api-token>"}'

curl -X POST http://127.0.0.1:8080/api/v1/services \
  -H 'Authorization: Bearer <admin-token>' \
  -H 'Content-Type: application/json' \
  -d '{"mode":"public","name":"Docs","hostname":"docs.example.com","origin_url":"http://127.0.0.1:3000","allow_type":"email","allow_value":"you@example.com"}'

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
- **Token validation failed**: use an API Token, not a Global API Key; check the
  mode-specific permissions, account/zone scope, expiration, and revocation state.
- **No public domain**: choose Quick for a temporary browser URL. If identity policy is
  required, choose Private and have visitors install and sign in to Cloudflare One
  Client/WARP. A Quick URL cannot be attached to a standard Access application.
- **Origin unreachable**: test the URL from the connector environment and check protocol,
  port, firewall, and container networking.
- **Access denied**: verify that the signed-in email exactly matches the Allow condition.
- **Private cannot be reached**: confirm the device is connected to WARP and the private IP
  matches the Origin host. Private is not a public DNS entry.
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
