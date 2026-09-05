# TunnelBox 分阶段改造实施计划

> 目标：在当前仓库上完成代码审查中确认的安全性、生命周期、数据模型、Cloudflare/API 可靠性和可维护性改造。
>
> 实施要求：Go 代码遵守标准设计规范；删除迁移链和历史兼容代码；功能文件不得超过 1000 行；每个阶段独立验证、独立 commit，最后推送远端。

## 0. 基本约束

- 当前分支：`refactor/latest-schema-hardening`
- Git 用户：`sparkyi <sparkyi@foxmail.com>`
- 远端：`origin`
- 推送目标：当前工作分支，不要直接向 `main` 推送。
- 不保留数据库迁移逻辑，不保留旧字段兼容逻辑，不保留旧 API 输入兼容逻辑。
- 不新增不必要的依赖；优先使用 Go 标准库和现有依赖。
- 不修改或提交本地数据、Token、SQLite 数据库和前端依赖目录。
- 每个功能文件保持小于 1000 行；如果修改前已接近上限，应在本阶段一并拆分。
- 修改后不要只运行局部测试，最终必须运行完整检查。

## 1. 当前实现基线和已发现问题

当前代码已经具备：

- `service / provision / operation / httpapi / cloudflare / connector / sqlite` 分层。
- 异步部署、停止、删除和重启恢复。
- SQLite、密码哈希、持久化 Session、Cloudflare Token 文件权限控制。
- 基本单元测试、Race、Vet 和前端构建。

需要完成的问题：

1. Operation 异步任务没有统一的应用生命周期，数据库关闭前没有等待所有任务退出。
2. 删除服务时没有删除 `data/cloudflared/tokens/<serviceID>.token`。
3. Session Cookie 缺少可配置的 `Secure` 属性。
4. JSON 请求体没有大小限制。
5. 登录接口没有最基本的限流和过期 Session 清理。
6. Request ID 可接受过宽字符集。
7. SQLite 仍保留多版本迁移链、旧字段和旧兼容逻辑。
8. Workspace 仍有无效的 `admin_token_path` 字段；服务仍使用 Quick hostname 占位符适配旧约束。
9. Cloudflare Token 非 `active` 时 Status 仍可能显示已配置。
10. Cloudflare Zone、Tunnel、Route、Access、Policy 列表没有统一完整分页。
11. Public hostname 未验证属于当前 Cloudflare Zone。
12. OpenAPI Schema 有错误引用、缩进问题，受保护接口缺少 Cookie security 声明。
13. `Deployer`、`App.tsx` 职责过重，需要拆分。
14. Docker 使用 `cloudflare/cloudflared:latest`。

---

# 阶段一：数据库模型清理与基础安全边界

## 1.1 SQLite 只保留当前 Schema

涉及文件：

- `internal/store/sqlite/sqlite.go`
- `internal/store/sqlite/sqlite_test.go`
- `internal/store/sqlite/workspace.go`
- `internal/store/sqlite/service_repository.go`
- `internal/service/workspace.go`
- `internal/service/service.go`
- 相关测试文件

要求：

- 删除 `migrate` 函数和 v1/v2/v3/v4 迁移分支。
- 只保留一份当前完整 Schema 初始化 SQL。
- 当前 Schema 必须直接包含最终字段：
  - `workspace.admin_password_hash`
  - `service.mode`
  - `service.private_route_id`
  - `service.public_url`
  - `auth_session`
- 删除 `admin_token_path` 字段及所有引用。
- Quick 服务的 `hostname` 使用空字符串，不再生成 `quick-*.invalid` 占位符。
- 用部分索引实现 Public/Private hostname 唯一约束：只对非空 hostname 建唯一索引，允许多个 Quick 服务共存。
- 删除 `storedMode`、`isQuickPlaceholder`、空 Mode 默认转换和旧 API 请求兼容逻辑。
- `mode` 必须由请求明确传入 `quick`、`private` 或 `public`；缺失直接返回校验错误。
- 数据库打开行为必须是“只支持当前 Schema”：旧版本数据库不得迁移。按已确认的 A 方案，旧数据库直接丢弃并重新创建当前 Schema；不得保留迁移或兼容分支。
- Schema 版本只用于判断是否是当前版本，不要建立迁移表，不要保存历史迁移代码。

测试：

- 当前 Schema 创建和重复打开。
- 所有表、字段、索引存在。
- 旧版本数据库启动后被丢弃并重新创建为空的当前 Schema。
- Quick 服务 hostname 为空，多个 Quick 服务可以同时创建。
- 缺失或非法 Mode 被拒绝。
- `admin_token_path` 不再存在。

建议 commit：

```text
refactor(storage): keep only the current sqlite schema
```

## 1.2 Cookie Secure 与 JSON 请求体限制

涉及文件：

- `internal/config/config.go`
- `internal/httpapi/router.go`
- `internal/httpapi/handlers.go`
- `internal/httpapi/router_test.go`

要求：

- 增加配置项 `TUNNELBOX_COOKIE_SECURE`，解析为布尔值。
- 默认值可保持 `false`，用于本地 HTTP；生产 HTTPS/反向代理环境由配置开启。
- 登录、首次设置、退出登录的 Cookie 属性保持一致，包含：
  - `HttpOnly: true`
  - `SameSite: Lax`
  - `Secure: 配置值`
  - `Path: /`
- 不要根据未经验证的 `X-Forwarded-Proto` 自动决定 Secure。
- `decodeJSON` 使用 `http.MaxBytesReader` 限制请求体，建议上限 1 MiB；登录请求可使用更小的上限。
- 保持 `DisallowUnknownFields` 和单一 JSON 对象校验。
- 增加 Cookie Secure、超大 JSON、额外 JSON 对象和未知字段测试。

建议 commit：

```text
fix(http): enforce secure sessions and request limits
```

---

# 阶段二：Operation 和 Connector 生命周期

## 2.1 Operation Manager 生命周期

涉及文件：

- `internal/operation/operation.go`
- `internal/operation/operation_test.go`
- `internal/bootstrap/bootstrap.go`

要求：

- `operation.Manager` 持有应用级 `context.Context`、取消函数和 `sync.WaitGroup`。
- `Start` 创建任务后使用 Manager 的应用 Context，不要继续使用 HTTP 请求 Context 执行异步任务。
- `Recover` 也使用同一个应用 Context。
- Manager 增加 `Shutdown(ctx)`：
  1. 标记 Manager 已关闭，禁止新的 Operation；
  2. 取消应用 Context；
  3. 等待所有已启动任务退出；
  4. 超时返回 `ctx.Err()`。
- `Start`、`Recover`、`Shutdown` 之间必须无数据竞争。
- 正确处理 WaitGroup：所有启动的 goroutine 都必须 `Add(1)`，在任务退出时 `Done()`。
- 任务因取消退出时，Operation 状态应保持 `unknown`，并写入安全错误码，不暴露底层错误文本。
- 不要让 Operation goroutine 在数据库关闭后继续写数据库。

Bootstrap 停机顺序：

1. HTTP Server 停止接收新请求并完成已有请求；
2. Operation Manager 停止接收新任务并等待任务退出；
3. Connector Runtime 停止并等待进程退出；
4. 最后关闭 SQLite。

测试：

- Operation 执行期间调用 `Shutdown` 会等待任务退出。
- Shutdown 后新的 `Start` 返回明确错误。
- Shutdown 后数据库仍可安全读取最终 Operation 状态。
- HTTP 客户端断开不会取消已接受的 Operation。
- Race 测试覆盖 Start/Shutdown 并发场景。

## 2.2 Connector Secret 文件清理

涉及文件：

- `internal/connector/cloudflared.go`
- `internal/connector/cloudflared_test.go`
- `internal/provision/ports.go`
- `internal/provision/deployer.go`
- `internal/provision/deployer_test.go`

要求：

- Connector Runtime 增加明确的凭据删除方法，例如：

```go
DeleteCredentials(context.Context, string) error
```

- 该方法只删除当前服务对应的：

```text
<dataDir>/tokens/<serviceID>.token
```

- 删除服务流程顺序：
  1. 停止 Connector；
  2. 删除已记录的 Cloudflare DNS、Access Policy、Access Application、Private Route、Tunnel；
  3. 删除 Connector Token 文件；
  4. 删除 SQLite 服务记录。
- Token 文件删除失败必须让 Operation 失败，不得静默忽略；服务记录和剩余远端引用必须保留以便重试。
- 删除不存在的 Token 文件视为成功，保证删除流程幂等。
- Quick 服务没有 Token 文件，删除时应正常成功。
- 服务 ID 必须继续经过路径安全校验，不能产生路径穿越。

测试：

- Token 文件被创建且权限为 `0600`。
- 删除服务后 Token 文件不存在。
- Token 文件删除失败会保留服务状态并返回安全错误码。
- 删除流程重试幂等。
- Quick 服务删除不要求 Token 文件。

建议 commit：

```text
fix(lifecycle): stop operations before closing resources
fix(connector): remove credentials with deleted services
```

---

# 阶段三：认证、Cloudflare 和 API 可靠性

## 3.1 登录保护和 Session 清理

涉及文件：

- `internal/httpapi/handlers.go`
- `internal/httpapi/middleware.go`
- `internal/auth/auth.go`
- `internal/store/sqlite/auth_repository.go`
- 相关测试

要求：

- 登录和首次设置接口增加最小限流，优先标准库实现，不引入 Redis 或外部服务。
- 以来源 IP + 时间窗口限流；如果部署在反向代理后，不要盲目信任任意 Forwarded Header，真实 IP 获取策略必须明确且简单。
- 连续失败时返回统一的认证错误，不泄露“用户不存在/未初始化”等内部状态。
- 日志可以记录请求 ID、来源和结果，但不能记录密码、Session Token 或 Cloudflare Token。
- 增加过期 Session 清理方法，在认证请求或定时任务中触发即可；不需要复杂后台调度器。
- 清理应使用参数化 SQL：

```sql
DELETE FROM auth_session WHERE expires_at <= ?
```

- 增加限流和过期 Session 测试。

## 3.2 Cloudflare Token 状态语义

涉及文件：

- `internal/cloudflare/integration.go`
- `internal/cloudflare/client.go`
- `internal/cloudflare/client_test.go`

要求：

- `Status` 只有在 Token 验证成功且 `TokenState == active` 时返回 `Configured: true`。
- `inactive`、`expired` 或其他非 active 状态必须返回 `Configured: false`，同时保留 TokenState。
- API 验证请求失败时返回安全的 `LastError`，不能返回底层 Token 或 SDK 错误文本。
- `Configure` 和 `Status` 的 active 判断必须一致。

## 3.3 Cloudflare 列表完整分页

涉及文件：

- `internal/cloudflare/client.go`
- `internal/cloudflare/client_test.go`

要求：

- 统一封装分页逻辑，至少覆盖：
  - Zones
  - Tunnels
  - Private Routes
  - Access Applications
  - Access Policies
- 不要依赖固定 `per_page=100` 或 `per_page=1000` 认为结果完整。
- 保证每页都推进 page 参数，并在没有更多结果时停止。
- 遇到上下文取消、HTTP 错误或无效分页信息时立即返回错误。
- 对超过一页的数据增加 HTTP mock 测试，验证请求页码和最终结果数量。

## 3.4 Public hostname 与 Zone 校验

涉及文件：

- `internal/service/service.go`
- `internal/cloudflare/integration.go`
- `internal/cloudflare/client.go`
- `internal/provision/deployer.go`
- 相关测试

要求：

- 配置 Public Zone 时保存并可读取 Zone Name；当前 Workspace 模型需要增加最终需要的 `zone_name` 字段，或者采用不增加字段但在部署前查询 Zone 的最简方案。
- Public 服务部署前必须验证 hostname 属于目标 Zone：
  - `hostname == zone`
  - 或 hostname 以 `.` + zone 结尾
- 必须防止 `notexample.com` 被错误判断为属于 `example.com`。
- Zone 名称和 hostname 比较统一小写、去掉无意义空格。
- 验证失败要在创建远程 Tunnel/Access/DNS 之前发生，避免留下孤立资源。
- 增加根域名、子域名、相似后缀域名、大小写和尾点测试。

## 3.5 Request ID 和 API 错误

涉及文件：

- `internal/httpapi/middleware.go`
- `internal/httpapi/handlers.go`
- 相关测试

要求：

- 外部 `X-Request-ID` 只接受固定安全格式，例如 `[A-Za-z0-9._-]{1,96}`。
- 非法值由服务端重新生成。
- Request ID 写入响应头、日志和错误 JSON 时保持一致。
- 不允许空格、引号、控制字符或任意 Unicode。

建议 commit：

```text
fix(cloudflare): make integration state and listings reliable
fix(api): validate hostnames and request identifiers
```

---

# 阶段四：OpenAPI、部署配置和代码拆分

## 4.1 修复 OpenAPI

涉及文件：

- `docs/openapi.yaml`
- 可新增 API 校验脚本或 CI 配置

要求：

- 将 `CloudflareConfigureRequest` 放到 `components.schemas` 的正确位置。
- 修复 `AuthRequest` 被错误嵌套或覆盖的问题。
- 检查所有 `$ref` 都能解析。
- 所有需要登录的 API 增加：

```yaml
security:
  - sessionCookie: []
```

- 公开接口只保留健康检查、静态控制台和认证状态/设置/登录所需路径。
- 对请求体中 Token 标记为 `writeOnly: true`，不要出现在响应 schema。
- 将 OpenAPI 校验加入可执行检查；优先使用仓库已有工具，没有时使用轻量 CLI，不要引入运行时依赖。

测试/验证：

```sh
# 使用环境中可用的 OpenAPI validator
redocly lint docs/openapi.yaml
# 或 swagger-cli validate docs/openapi.yaml
```

## 4.2 固定 cloudflared 镜像版本

涉及文件：

- `deploy/Dockerfile`

要求：

- 把：

```dockerfile
FROM cloudflare/cloudflared:latest AS cloudflared
```

改成固定版本或 digest。
- 不要使用 `latest`。
- Docker 构建必须仍能完成，最终镜像中的 `cloudflared` 可执行文件可运行。

## 4.3 拆分 Deployer

涉及文件：

- `internal/provision/deployer.go`
- 可新增 `deploy_quick.go`
- 可新增 `deploy_managed.go`
- 可新增 `delete.go`
- 可新增 `failure.go`
- 相关测试

要求：

- 每个功能文件小于 1000 行，目标是每个文件只承担一个流程职责。
- 保持 `Deployer` 对外 API 不变：`Deploy`、`Stop`、`Delete`、`Resume`。
- 推荐拆分：
  - Quick Tunnel 部署流程；
  - Private/Public Managed Tunnel 部署流程；
  - Stop 流程；
  - Delete/远端清理流程；
  - Connector 健康检查和错误转换。
- 不要引入 Saga 框架、通用工作流框架或多余接口。
- 状态更新、Operation step 更新和安全错误转换保持统一。
- 拆分后现有调用者和测试行为必须保持一致。

## 4.4 拆分前端 App

涉及文件：

- `web/src/App.tsx`
- 可新增：
  - `web/src/api/client.ts`
  - `web/src/api/types.ts`
  - `web/src/components/AuthScreen.tsx`
  - `web/src/components/ServiceTable.tsx`
  - `web/src/components/ServiceDialog.tsx`
  - `web/src/components/IntegrationDialog.tsx`
  - `web/src/components/OperationPanel.tsx`
  - `web/src/hooks/useOperationPolling.ts`
  - `web/src/hooks/useConsoleData.ts`

要求：

- `App.tsx` 只保留页面编排和顶层状态。
- API 请求和 TypeScript 类型移出组件。
- 至少拆出认证页面、服务列表、服务编辑弹窗、Cloudflare 配置弹窗、Operation 轮询。
- 不新增状态管理库。
- 保持现有 UI 和行为，移动端布局不得回归。
- 每个文件小于 1000 行。

建议 commit：

```text
fix(api): repair the openapi contract
build(deploy): pin the cloudflared image
refactor(provision): split deployment workflows
refactor(web): split the console into focused modules
```

---

# 阶段五：测试、文档和最终验证

## 5.1 必须补充的测试

Go：

- Operation Manager Shutdown、取消和并发 Start。
- Connector Token 创建、权限和删除。
- Cookie Secure 属性。
- JSON 请求体大小限制。
- 登录限流和过期 Session 清理。
- Request ID 非法字符。
- Cloudflare Token inactive 状态。
- Cloudflare 多页列表。
- Public hostname/Zone 归属校验。
- OpenAPI Schema 引用和 security 声明。
- 删除失败后的状态与引用保留。
- 数据库旧版本直接重建当前 Schema。

前端：

- 至少执行 TypeScript 编译和 Vite 构建。
- 如果仓库已有浏览器测试能力，补一条主流程：

```text
首次设置密码 -> 登录 -> 创建 Quick 服务 -> 部署 -> 轮询 Operation -> 显示 Quick URL
```

- 没有必要为纯展示组件引入完整 E2E 测试框架；优先保证主流程和构建通过。

## 5.2 文档更新

同步更新：

- `README.md`
- `README.zh-CN.md`
- `docs/openapi.yaml`
- `deploy/docker-compose.yml`（如新增配置项需要展示）

文档必须反映：

- `TUNNELBOX_COOKIE_SECURE` 配置项。
- Quick 服务不使用 hostname。
- 当前数据库只支持最新 Schema，启动旧数据库时直接重建；不要描述自动迁移。
- 删除服务会删除本地 Connector Token 文件。
- Docker 使用固定版本的 cloudflared。

## 5.3 最终检查

执行：

```sh
gofmt -w ./cmd ./internal
go test ./...
go test -race ./...
go vet ./...
git diff --check

cd web
npm run build
cd ..

# 检查功能文件行数
find internal cmd web/src -type f \( -name '*.go' -o -name '*.tsx' -o -name '*.ts' \) -print0 \
  | xargs -0 wc -l | sort -nr

# 检查迁移和兼容代码是否清除
rg -n 'migrat|schema_migrations|admin_token_path|AdminTokenPath|isQuickPlaceholder|storedMode|legacy|compatib|latest' \
  internal cmd docs README* deploy
```

最终要求：

- `rg` 结果中不能有数据库迁移链、旧字段、Quick 占位符和兼容逻辑。
- 所有功能文件小于 1000 行。
- Go、Race、Vet、前端构建全部通过。
- OpenAPI 可被标准工具解析。
- 工作区只包含预期源码、测试、文档和部署配置变更。

---

# Commit 和推送规则

每个阶段先验证，再提交，不要把所有修改压成一个 commit。建议顺序：

```text
refactor(storage): keep only the current sqlite schema
fix(http): enforce secure sessions and request limits
fix(lifecycle): stop operations before closing resources
fix(connector): remove credentials with deleted services
fix(cloudflare): make integration state and listings reliable
fix(api): validate hostnames and request identifiers
fix(api): repair the openapi contract
build(deploy): pin the cloudflared image
refactor(provision): split deployment workflows
refactor(web): split the console into focused modules
test: cover hardening and lifecycle paths
docs: update deployment and runtime behavior
```

每次 commit：

```sh
git status --short
git diff --check
git add <本阶段明确文件>
git commit -m "<message>"
git status --short
```

全部完成后：

```sh
git config user.name sparkyi
git config user.email sparkyi@foxmail.com
git push -u origin refactor/latest-schema-hardening
```

推送前确认：

- 没有 Token、`.env`、数据库和构建缓存被加入 commit。
- commit 历史按阶段可读。
- 分支不是 `main`。
- 最终测试全部通过。
