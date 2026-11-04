# ChatGPT 账号池

管理员侧边栏的「ChatGPT 账号池」对应 `/admin/chatgpt-accounts`，管理 ChatGPT Web 和 Codex 来源的上游凭据。账号使用现有 `accounts` 表，平台标识为 `chatgpt`、认证类型为 `oauth`，继续复用 sub2api 的分组、代理、批量更新和管理员权限。

该账号池用于账号维护。它不注册网关协议，不会把网页账号按 Codex Responses 协议加入原有 OpenAI 调度池。原有 `/admin/accounts` 中的 OpenAI 账号继续使用原来的网关。

## 管理能力

- 列表、卡片视图；按名称、邮箱、账号 ID、套餐、来源、分组和状态筛选。
- 手动添加和编辑 AT、RT、来源、套餐、备注、代理以及多个分组。编辑时凭据留空保留原值。
- 显示 AT 签发/到期时间与 RT 存在性。JWT 解析仅用于展示生命周期，不等同于验证签名或上游可用性；未知有效期显示「待验证」。
- 同步账号身份、套餐和图片额度，刷新 AT 为单独操作。缺少图片额度字段时显示未知；Pro / ProLite 且未返回数值额度时显示无限额度。
- 本页选择、跨页全选筛选结果、批量同步、刷新 AT、启停、重置账号错误、替换分组与删除。操作结果逐项展示；停止后续处理会保留已经发出的请求结果。
- 支持每行一个 AT、Session JSON、CPA JSON、chatgpt2api 账号 JSON 和 sub2api 备份。单次最多 1000 个账号，浏览器导入总大小限制为 12 MiB。过期 AT 可以入库，供携带 RT 的账号续期。
- OAuth 登录复用 sub2api 的 OpenAI 授权流程，并保留返回的 OAuth client ID。
- 支持通过 HTTPS 地址和管理密钥从 CPA / Sub2API 导入。密钥不持久化；连接拒绝私有地址与重定向。要求二次验证的远程 Sub2API 会拒绝 API Key 导出，此时使用远程已验证的 JSON 备份。
- 导出选中或筛选结果为完整 JSON、每行一个 AT 的 TXT。完整 sub2api 备份保留代理、分组名称与启用状态；还原时按名称解析或创建 ChatGPT 分组。
- 复制 AT/RT、导出凭据沿用现有管理员二次验证。

## 同步和后台维护

上游同步读取 `/backend-api/me`、`/backend-api/conversation/init` 和 `/backend-api/accounts/check/v4-2023-04-27`，不会进行图片生成或消耗一次生成任务。

后台维护使用现有 TokenRefreshService 的启用开关和运行周期。启用中的 ChatGPT 账号在 AT 接近到期时续期，账号信息和额度检查间隔至少 15 分钟。暂停调度或停用的账号不会被后台维护。HTTP 403、超时等错误保留上一次成功的额度观察；只有明确的终止刷新错误才标记 RT 无效。

账号维护在单次请求内有界执行，并使用进程并发上限、数据库租约及凭据版本条件更新。并发编辑或删除胜过旧的上游响应。RT 刷新启动后有独立的 60 秒生命周期，即使浏览器断开，也继续尝试保存已经轮换的凭据。

不需要数据库结构迁移。凭据继续放在 `credentials`，上游观察和维护信息放在 `extra.chatgpt_*`；列表 DTO 返回脱敏凭据和状态。

## API

管理员鉴权下使用 `/api/v1/admin/accounts` 前缀：

| 路径 | 用途 |
| --- | --- |
| `GET /chatgpt` | 状态筛选、分页、计数；`ids_only=1` 返回匹配账号 ID |
| `POST /chatgpt/import` | 导入 Token / JSON |
| `POST /chatgpt/import-remote` | 从 CPA / Sub2API 拉取并导入 |
| `POST /chatgpt/maintain` | `action=sync` 同步账号/额度，`action=refresh` 刷新 AT |
| `GET /:id/chatgpt-credential/:kind` | 二次验证后读取 AT 或 RT，响应禁止缓存 |

编辑、启停、删除、分组、代理和备份继续使用已有管理 API。

## 验证

新增测试覆盖凭据生命周期、状态优先级、额度未知与失败区分、RT 轮换、日志/错误脱敏、并发更新条件、跨实例维护租约、导入兼容、备份分组还原和前端批量部分失败。页面还可使用模拟 API 在桌面和手机浏览器中检查布局。

真实上游连通性取决于账号凭据、代理和上游对请求的接受情况；本地模拟测试不代表真实账号已经通过验证。
