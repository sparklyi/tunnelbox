# TunnelBox

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

- [中文](#中文)
- [English](#english)

## 中文

### 功能范围

- 管理 Web/HTTP/HTTPS Origin 和 `cloudflared` Connector
- `quick` 临时公开、`private` 私网受控、`public` 自有域名三种模式
- Cloudflare Access Allow 策略（邮箱或邮箱域名）
- 异步部署、操作进度查询，以及控制面重启后的未完成操作恢复
- SQLite 保存配置和状态；Cloudflare Token 只写入权限为 `0600` 的 Secret 文件

当前 MVP 只支持 Web Origin，不支持 SSH、TCP、RDP 或独立的远程 Connector，
也不导入或删除未由 TunnelBox 明确绑定的远端资源。

### 快速开始

#### Docker Compose（推荐）

```sh
docker compose -f deploy/docker-compose.yml up --build -d
docker compose -f deploy/docker-compose.yml exec tunnelbox cat /app/data/admin.token
```

打开 <http://127.0.0.1:8080>，将输出的管理员令牌粘贴到页面中。示例 Compose
把控制面绑定到本机，并把数据库、Connector 状态和 Secret 保存在
`tunnelbox-data` 卷中；镜像已经包含 `cloudflared`。

#### 从源码运行

需要 Go 1.26、Node.js 22+ 和 PATH 中的 `cloudflared`：

```sh
cd web && npm ci && npm run build
cd ..
go run ./cmd/tunnelbox
```

打开 <http://127.0.0.1:8080>。默认回环监听不要求控制面管理员令牌；监听局域网
或公网地址时，必须设置 `TUNNELBOX_ADMIN_TOKEN_FILE`，且文件权限应为 `0600`。

### 第一次部署

1. 确认运行 TunnelBox 的机器或容器可以访问 Origin。Origin 不需要公网 IP，
   `localhost` 指的是运行 `cloudflared` 的环境。
2. 在控制台点击“新建服务”，选择模式：
   - **Quick**：不需要 Cloudflare 账号、域名或 API Token。部署后得到临时的
     `https://*.trycloudflare.com` 地址，不支持标准 Access 策略。
   - **Private**：不需要公网域名，但需要 Cloudflare Account ID、API Token，
     以及已加入同一 Zero Trust 组织的 Cloudflare One Client/WARP。访问者使用
     私网 IP，且必须在 Split Tunnels 中让目标 IP/CIDR 经过 WARP。
   - **Public**：需要自己拥有的 Zone、Account ID 和 API Token。TunnelBox 会
     创建 Access 和 DNS CNAME，访问者可以用普通浏览器打开域名并登录 Access。
3. Private 在“配置连接”中留空 Zone ID；Public 填写目标 Zone ID；Quick 可以
   跳过 Cloudflare 配置。保存时 TunnelBox 会先验证 Token。
4. 填写服务名称和 Origin URL。Private 的“私网 IP”应与 Origin 主机一致；Public
   的“公网域名”必须属于目标 Zone；Quick 不需要填写主机名和 Allow 条件。
5. 点击“部署”，在操作面板等待异步操作完成。重复部署会复用已保存的远端资源。
6. 按模式验证入口：打开 Quick 地址；通过 WARP 访问 Private 私网 IP；或在普通
   浏览器打开 Public 域名并完成 Access 登录。

Private 不是普通公网 DNS 入口。需要先完成 Zero Trust 组织和设备注册，再配置
[Split Tunnels](https://developers.cloudflare.com/cloudflare-one/team-and-resources/devices/cloudflare-one-client/configure/route-traffic/split-tunnels/)，否则资源即使部署成功，访问者也可能无法连到私网 IP。

### 获取 Cloudflare 配置

| 信息 | 获取位置 |
| --- | --- |
| Account ID | Cloudflare Dashboard 的 Account Overview，或[官方说明](https://developers.cloudflare.com/fundamentals/account/find-account-and-zone-ids/) |
| Zone ID（仅 Public） | 目标 Zone 的 Overview 页面；Private 和 Quick 不需要 |
| API Token（Private/Public） | 按[官方创建说明](https://developers.cloudflare.com/fundamentals/api/get-started/create-token/)在 Profile → API Tokens 创建 Custom Token |
| `cloudflared` | [安装 Cloudflare Tunnel Connector](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/downloads/) |
| WARP / One Client（Private） | [Cloudflare One Client 文档](https://developers.cloudflare.com/cloudflare-one/connections/connect-devices/warp/) |

Token 至少需要目标 Account 的 Tunnel、Access/Zero Trust 写权限；Public 还需要
目标 Zone 的 DNS 编辑和 Zone 读取权限。只授权目标 Account/Zone，并设置过期时间，
不要使用 Global API Key。TunnelBox 不把 Token 写入 SQLite、日志或 API 响应。

### 配置项

所有配置通过环境变量提供，默认值适合本地运行：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `TUNNELBOX_LISTEN` | `127.0.0.1:8080` | HTTP 监听地址 |
| `TUNNELBOX_DATABASE` | `data/tunnelbox.db` | SQLite 文件 |
| `TUNNELBOX_ADMIN_TOKEN_FILE` | 空 | 非回环监听时必填的管理员令牌文件 |
| `TUNNELBOX_CLOUDFLARE_TOKEN_FILE` | `data/cloudflare.token` | Cloudflare Token 文件 |
| `TUNNELBOX_CLOUDFLARED_BINARY` | `cloudflared` | Connector 可执行文件 |
| `TUNNELBOX_CLOUDFLARED_DATA_DIR` | `data/cloudflared` | Connector 状态目录 |
| `TUNNELBOX_WEB_DIR` | `web/dist` | 管理界面静态文件目录 |

`TUNNELBOX_WORKSPACE_ID` 和 `TUNNELBOX_WORKSPACE_NAME` 可用于修改默认
Workspace（分别为 `default` 和 `Default`）。

### API 和开发

完整 API 契约见 [`docs/openapi.yaml`](docs/openapi.yaml)。部署接口返回 `202`，
客户端通过 `/api/v1/operations/:id` 轮询进度。

```sh
# Quick：不需要域名或 Cloudflare 配置
curl -X POST http://127.0.0.1:8080/api/v1/services \
  -H 'Content-Type: application/json' \
  -d '{"mode":"quick","name":"本地预览","origin_url":"http://127.0.0.1:3000"}'

curl -X POST http://127.0.0.1:8080/api/v1/services/SERVICE_ID/deploy
curl http://127.0.0.1:8080/api/v1/operations/OPERATION_ID
```

如果配置了 `TUNNELBOX_ADMIN_TOKEN_FILE`，在上述请求中加入
`Authorization: Bearer <admin-token>`。健康检查为 `/healthz`，就绪检查为
`/readyz`。

本地开发检查：

```sh
gofmt -w ./cmd ./internal
go test ./...
go test -race ./...
go vet ./...
cd web && npm run build
```

## English

### What it does

TunnelBox is a local control plane for publishing internal HTTP/HTTPS origins through
Cloudflare Tunnel. It manages `cloudflared`, provisions the required resources, and
shows asynchronous deployment progress in a small web console.

The three modes are deliberately different:

| Mode | Domain needed | How visitors connect | Access |
| --- | --- | --- | --- |
| `quick` | No | Temporary `trycloudflare.com` URL | No standard Access policy |
| `private` | No | Private IP through Cloudflare One Client/WARP | Email/email-domain Allow |
| `public` | Yes | Hostname in a zone you own | Email/email-domain Allow |

The MVP supports Web origins only. SSH, TCP, RDP, arbitrary remote connectors, and
unmanaged remote resources are out of scope.

### Quick start

#### Docker Compose (recommended)

```sh
docker compose -f deploy/docker-compose.yml up --build -d
docker compose -f deploy/docker-compose.yml exec tunnelbox cat /app/data/admin.token
```

Open <http://127.0.0.1:8080> and paste the printed admin token into the console. The
example binds the control plane to loopback, persists data in the `tunnelbox-data`
volume, and includes `cloudflared` in the image.

#### From source

Install Go 1.26, Node.js 22+, and `cloudflared` on `PATH`:

```sh
cd web && npm ci && npm run build
cd ..
go run ./cmd/tunnelbox
```

Open <http://127.0.0.1:8080>. A loopback bind does not require an admin token. Any LAN
or public bind requires `TUNNELBOX_ADMIN_TOKEN_FILE` pointing to a `0600` file.

### First deployment

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

Private is not a public DNS entry. Complete Zero Trust enrollment and configure
[Split Tunnels](https://developers.cloudflare.com/cloudflare-one/team-and-resources/devices/cloudflare-one-client/configure/route-traffic/split-tunnels/) so the target private IP is sent through WARP.

### Cloudflare values

- Find the Account ID and Zone ID in the [Cloudflare dashboard](https://developers.cloudflare.com/fundamentals/account/find-account-and-zone-ids/).
- Create a scoped API Token from [Profile → API Tokens](https://dash.cloudflare.com/profile/api-tokens), following the [official token guide](https://developers.cloudflare.com/fundamentals/api/get-started/create-token/).
- Install [`cloudflared`](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/downloads/).
- Install [Cloudflare One Client/WARP](https://developers.cloudflare.com/cloudflare-one/connections/connect-devices/warp/) for Private visitors.

Grant only the permissions required by the selected mode. Private needs Account-level
Tunnel, Access/Zero Trust write permissions; Public also needs Zone DNS edit and Zone
read permissions. Do not use a Global API Key. Tokens are stored only in the configured
owner-only Secret file, never in SQLite, logs, or API responses.

### API and development

See [`docs/openapi.yaml`](docs/openapi.yaml) for the complete API contract. Deployment
returns `202 Accepted`; poll `/api/v1/operations/:id` for progress. If admin
authentication is enabled, send `Authorization: Bearer <admin-token>`.

```sh
curl -X POST http://127.0.0.1:8080/api/v1/services \
  -H 'Content-Type: application/json' \
  -d '{"mode":"quick","name":"Local preview","origin_url":"http://127.0.0.1:3000"}'
curl -X POST http://127.0.0.1:8080/api/v1/services/SERVICE_ID/deploy
curl http://127.0.0.1:8080/api/v1/operations/OPERATION_ID
```

Run the checks before submitting changes:

```sh
gofmt -w ./cmd ./internal
go test ./...
go test -race ./...
go vet ./...
cd web && npm run build
```
