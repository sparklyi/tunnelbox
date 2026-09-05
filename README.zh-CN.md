# TunnelBox

<p><img src="web/public/assets/logo-wordmark.svg" alt="TunnelBox" width="420" /></p>

[English](README.md)

TunnelBox 是一个轻量的本地控制面，用来把内网 HTTP/HTTPS 服务通过 Cloudflare
Tunnel 发布出去。它以受控方式运行 `cloudflared`，创建所需的 Cloudflare 资源，
并在一个管理界面中展示部署进度。

请按访问方式选择模式：

| 模式 | 需要公网域名 | 访问方式 | Cloudflare Access |
| --- | --- | --- | --- |
| `quick` | 不需要 | 临时 `trycloudflare.com` 地址 | 不支持标准策略 |
| `private` | 不需要 | 通过 Cloudflare One Client/WARP 访问私网 IP | 支持邮箱或邮箱域名策略 |
| `public` | 需要 | 你拥有的 Zone 下的域名 | 支持邮箱或邮箱域名策略 |

没有公网域名也可以使用 TunnelBox：临时分享选 `quick`；需要身份和权限控制、且
访问者可以使用 WARP 时选 `private`；只有在拥有目标 Cloudflare Zone 时才选
`public`。

## 功能范围

- 管理 Web/HTTP/HTTPS Origin 和 `cloudflared` Connector
- 支持 `quick`、`private`、`public` 三种明确的发布模式
- 为指定邮箱或邮箱域名创建 Cloudflare Access Allow 策略
- 支持异步部署、进度查询，以及控制面重启后的未完成操作恢复
- 可停止运行中的 Connector，但保留 Cloudflare 资源，之后可以再次部署
- SQLite 保存本地状态；Cloudflare API Token 只写入权限为 `0600` 的 Secret 文件

当前 MVP 只支持 Web Origin，不支持 SSH、TCP、RDP 或独立的远程 Connector，也不导入
或删除未由 TunnelBox 明确绑定的远端资源。

## 快速开始

### Docker Compose（推荐）

```sh
docker compose -f deploy/docker-compose.yml up --build -d
```

打开 <http://127.0.0.1:8080>，首次使用时直接创建管理员密码。登录后控制台会使用安全的
持久化会话 Cookie；用户不需要查找或复制任何令牌。示例 Compose 把控制面映射到本机，并把
SQLite、Connector 状态和 Secret 保存在 `tunnelbox-data` 卷中；镜像已经包含 `cloudflared`。

### 从源码运行

需要 Go 1.26、Node.js 22+ 和 PATH 中的 `cloudflared`：

```sh
cd web && npm ci && npm run build
cd ..
go run ./cmd/tunnelbox
```

打开 <http://127.0.0.1:8080>，首次使用时创建管理员密码。之后访问会自动使用安全的持久化会话
Cookie。局域网或公网监听时，请额外使用反向代理或网络策略限制访问范围；应用本身统一使用密码登录。

## 第一次部署

1. 确认运行 TunnelBox 的机器或容器可以访问 Origin。Origin 不需要公网 IP，`localhost`
   指的是运行 `cloudflared` 的环境。
2. 在控制台点击“新建服务”，选择模式：
   - **Quick**：不需要 Cloudflare 账号、域名或 API Token。部署后得到临时的
     `https://*.trycloudflare.com` 地址，不支持标准 Access 策略。
   - **Private**：不需要公网域名，但需要 Cloudflare Account ID、API Token，以及已
     加入同一 Zero Trust 组织的 Cloudflare One Client/WARP。访问者使用私网 IP，并且
     必须在 Split Tunnels 中让目标 IP/CIDR 经过 WARP。
   - **Public**：需要自己拥有的 Zone、Account ID 和 API Token。TunnelBox 会创建 Access
     和 DNS CNAME，访问者可以用普通浏览器打开域名并登录 Access。
3. Private 在“配置连接”中留空 Zone ID；Public 填写目标 Zone ID；Quick 可以跳过
   Cloudflare 配置。保存时 TunnelBox 会先验证 Token。
4. 填写服务名称和 Origin URL。Private 的“私网 IP”应与 Origin 主机一致；Public 的
   “公网域名”必须属于目标 Zone；Quick 不需要填写主机名和 Allow 条件。
5. 点击“部署”，在操作面板等待异步操作完成。重复部署会复用已保存的远端资源。
6. 按模式验证入口：打开 Quick 地址；通过 WARP 访问 Private 私网 IP；或在普通浏览器
   打开 Public 域名并完成 Access 登录。

需要临时下线服务时，点击服务行中的“停止”。这只会停止本地 Connector，并保留 Tunnel、
Access 策略、DNS 记录和服务配置；之后点击“部署”即可恢复，不会删除远端资源。

需要移除服务时，点击服务行中的“删除”并确认。没有远端引用的草稿或已停止服务会立即
删除；已部署服务会先停止 Connector，再按本服务记录的 ID 删除 TunnelBox 创建的 DNS、
Access、私网路由和 Tunnel，最后删除本地记录。操作面板会显示每一步；如果某一步失败，
服务和剩余远端 ID 会保留，修复 Token 或权限后可以再次点击“删除”重试。

Private 不是普通公网 DNS 入口。需要先完成 Zero Trust 组织和设备注册，再配置
[Split Tunnels](https://developers.cloudflare.com/cloudflare-one/team-and-resources/devices/cloudflare-one-client/configure/route-traffic/split-tunnels/)，否则资源即使部署成功，访问者也可能无法连到私网 IP。

## 获取 Cloudflare 配置

| 信息 | 获取位置 |
| --- | --- |
| Account ID | Cloudflare Dashboard 的 Account Overview，或[官方说明](https://developers.cloudflare.com/fundamentals/account/find-account-and-zone-ids/) |
| Zone ID（仅 Public） | 目标 Zone 的 Overview 页面；Private 和 Quick 不需要 |
| API Token（Private/Public） | 按[官方创建说明](https://developers.cloudflare.com/fundamentals/api/get-started/create-token/)在 Profile → API Tokens 创建 Custom Token |
| `cloudflared` | [安装 Cloudflare Tunnel Connector](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/downloads/) |
| WARP / One Client（Private） | [Cloudflare One Client 文档](https://developers.cloudflare.com/cloudflare-one/connections/connect-devices/warp/) |

Token 至少需要目标 Account 的 Tunnel、Access/Zero Trust 写权限；Public 还需要目标 Zone
的 DNS 编辑和 Zone 读取权限。只授权目标 Account/Zone，并设置过期时间，不要使用
Global API Key。TunnelBox 不把 Token 写入 SQLite、日志或 API 响应。

## 配置项

所有配置通过环境变量提供，默认值适合本地运行：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `TUNNELBOX_LISTEN` | `127.0.0.1:8080` | HTTP 监听地址 |
| `TUNNELBOX_DATABASE` | `data/tunnelbox.db` | SQLite 文件 |
| `TUNNELBOX_CLOUDFLARE_TOKEN_FILE` | `data/cloudflare.token` | Cloudflare Token 文件 |
| `TUNNELBOX_CLOUDFLARED_BINARY` | `cloudflared` | Connector 可执行文件 |
| `TUNNELBOX_CLOUDFLARED_DATA_DIR` | `data/cloudflared` | Connector 状态目录 |
| `TUNNELBOX_WEB_DIR` | `web/dist` | 管理界面静态文件目录 |

`TUNNELBOX_WORKSPACE_ID` 和 `TUNNELBOX_WORKSPACE_NAME` 可用于修改默认 Workspace
（分别为 `default` 和 `Default`）。

## API 和开发

完整 API 契约见 [`docs/openapi.yaml`](docs/openapi.yaml)。部署接口返回 `202`，客户端通过
`/api/v1/operations/:id` 轮询进度。首次使用调用 `/api/v1/auth/setup` 创建密码，之后调用
`/api/v1/auth/login` 登录；服务端会设置 HttpOnly 会话 Cookie。

```sh
# Quick：不需要域名或 Cloudflare 配置
curl -X POST http://127.0.0.1:8080/api/v1/services \
  -H 'Content-Type: application/json' \
  -d '{"mode":"quick","name":"本地预览","origin_url":"http://127.0.0.1:3000"}'
curl -X POST http://127.0.0.1:8080/api/v1/services/SERVICE_ID/deploy
curl http://127.0.0.1:8080/api/v1/operations/OPERATION_ID
# 之后停止 Connector，不删除服务或 Cloudflare 资源
curl -X POST http://127.0.0.1:8080/api/v1/services/SERVICE_ID/stop
# 删除服务（本地删除返回 204，远端清理返回 202 和操作 ID）
curl -X DELETE http://127.0.0.1:8080/api/v1/services/SERVICE_ID
```

健康检查为 `/healthz`，就绪检查为 `/readyz`。

提交前运行：

```sh
gofmt -w ./cmd ./internal
go test ./...
go test -race ./...
go vet ./...
cd web && npm run build
```
