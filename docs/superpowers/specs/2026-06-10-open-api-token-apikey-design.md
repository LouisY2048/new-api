# Open API — Token / APIKey 开放接口 设计文档

**版本：** V1.1
**日期：** 2026-06-10
**参考文档：** `docs/fangan/注册+查询接口规范.md`

## 1. 概述

在现有 API 网关平台基础上，新增一套面向第三方合作方的开放接口（`/open/v1/`），支持 ICCID + 邮箱 + 套餐 ID 签发 APIKey，以及通过 ICCID 查询 Token 使用量。接口采用与现有 `/api/`、`/v1/` 独立的鉴权体系和返回格式。

### 接口清单

| 接口 | 路径 | 鉴权 | 说明 |
|------|------|------|------|
| 签发 APIKey | `POST /open/v1/apikey/issue` | OpenAuth + 限流 | ICCID + email + planid → APIKey |
| 查询使用量 | `POST /open/v1/token/usage` | OpenAuth | ICCID → 配额 + 明细 |

## 2. 返回格式

Open API 使用独立的响应辅助函数，不复用 `common.ApiSuccess`/`common.ApiError`。

### 统一格式

```json
// 成功
{"code": "00000", "message": "success", "data": {...}}

// 失败
{"code": "10001", "message": "iccid 不存在或未开通", "data": null}
```

- `code` 为字符串类型
- 成功时 `data` 携带业务数据
- 失败时 `data` 固定为 `null`

### 错误码

| code | 释义 |
|------|------|
| 00000 | 业务处理成功 |
| 10001 | ICCID 不存在、未入库或未开户 |
| 10002 | 邮箱格式不符合规范 |
| 10003 | ICCID 与绑定邮箱信息不匹配 |
| 10004 | APIKey 无效、已过期或已作废 |
| 10005 | 该 ICCID 已申领过 APIKey，禁止重复签发 |
| 10008 | 触发接口限流，请求频次超限 |
| 10009 | 签名错误、AppId 非法或时间戳过期 |
| 10010 | 疑似爬虫批量遍历连续 ICCID |
| 99999 | 平台系统内部异常 |

> 10006、10007 预留，当前版本不实现。

## 3. 数据模型与数据库变更

### 3.1 新建表：`open_apps`

| 字段 | 类型 | 说明 |
|------|------|------|
| id | int, PK, auto_increment | 主键 |
| app_id | varchar(64), uniqueIndex | 应用标识，平台线下分配 |
| app_secret | varchar(128) | 密钥（32 位随机字符串，crypto/rand 生成） |
| name | varchar(128) | 应用名称 |
| status | int, default 1 | 状态：1=启用, 0=禁用 |
| ip_whitelist_enabled | bool, default false | IP 白名单开关 |
| allow_ips | text, default '' | IP 白名单，逗号分隔 |
| created_at | bigint | 创建时间戳 |
| updated_at | bigint | 更新时间戳 |

### 3.2 现有表变更：`users`

新增字段：

```go
Iccid string `json:"iccid" gorm:"type:varchar(32);uniqueIndex;default:''"`
```

### 3.3 缓存

- OpenApp：Redis Hash + 内存热缓存，TTL 默认 300 秒
- 查询路径：Redis → DB 回填

## 4. 鉴权中间件 — OpenAuth

### 4.1 请求头

| Header | 必填 | 说明 |
|--------|------|------|
| X-AppId | 是 | 平台分配的 AppId |
| X-Timestamp | 是 | Unix 秒级时间戳 |
| X-Sign | 是 | MD5(AppId+AppSecret+X-Timestamp+Body原文) |

### 4.2 校验流程

```
1. 读取 X-AppId / X-Timestamp / X-Sign → 缺任一 → 10009
2. 根据 AppId 查缓存/DB → 不存在或禁用 → 10009
3. 若 ip_whitelist_enabled=true，检查来源 IP 是否在 allow_ips 中 → 不在 → 10009
4. |now - X-Timestamp| > 300s → 10009
5. 读 Body 原文，计算 MD5(AppId + AppSecret + X-Timestamp + Body 原文)
   → 与 X-Sign 对比 → 不一致 → 10009
6. 通过：openApp 信息注入 Context，放行
```

### 4.3 签名计算注意

- Body 原文通过 `GetBodyStorage().Bytes()` 获取
- 签名校验后 `Seek(0)` 重置，确保后续 Controller 可正常 Unmarshal

## 5. 限流中间件

### 5.1 ICCID 申领频次

- Redis Key: `openapi:ratelimit:iccid:<iccid>:1h`
- 限制：1 小时内最多 20 次
- 超限：返回 10008

### 5.2 邮箱绑定 ICCID 数

- Redis Key: `openapi:ratelimit:email:<email>:1d`
- 限制：同一邮箱单日最多 5 个不同 ICCID（已存在的不重复计数）
- 超限：返回 10008

### 5.3 连续 ICCID 风控

- Redis Key: `openapi:iccid_history:<AppId>`，存储最近 5 个 ICCID 的最后 6 位数字
- 限制：当前 ICCID 与列表中已有 ICCID 形成步进+1 连续序列（≥5 个）
- 超限：返回 10010
- TTL：60 秒

## 6. 接口流程

### 6.1 签发 APIKey — `POST /open/v1/apikey/issue`

**请求：**

```json
{"iccid": "89860012345678901234", "email": "user@example.com", "planid": 1}
```

**流程：**

```
OpenAuth 鉴权 → 限流检查
→ Controller:
  1. 校验邮箱长度（≤50 字符）→ 不合法返回 10002
  2. 查 ICCID（users.iccid 唯一索引）是否已有用户：
     a. 已有用户 且 邮箱与请求一致 且 已有有效套餐 且 已有有效 APIKey
        → 返回 10005，data 携带已有 apiKey、tokenTotal、validEndDate
     b. 已有用户 且 邮箱与请求一致 但 无有效套餐（或套餐过期/无有效 APIKey）
        → 返回 10004
     c. 已有用户 且 邮箱不一致
        → 返回 10005，data 为 null
  3. ICCID 未绑定用户：
     - 创建新用户（自动生成用户名），填入 email + iccid
     - Redis 计数器 openapi:ratelimit:email:<email>:1d 计数 +1
  4. 查 planid 对应的订阅套餐（SubscriptionPlan）
  5. 为该用户创建真实订阅记录（UserSubscription，source="openapi"），
     用户划入套餐对应 UpgradeGroup
  6. 生成 APIKey（model.Token）：
     - UnlimitedQuota = true（不限额度，消费时扣用户账户配额）
     - Group = 套餐 UpgradeGroup
  7. 返回：
     - apiKey: 生成的密钥
     - tokenTotal: 套餐 TotalAmount
     - validEndDate: 订阅 EndTime，格式 yyyy-MM-dd
```

**成功返回：**

```json
{
  "code": "00000",
  "message": "success",
  "data": {
    "apiKey": "sk-xxxxxxxx",
    "tokenTotal": 100000,
    "validEndDate": "2026-12-05"
  }
}
```

**已有 APIKey 返回：**

```json
// 邮箱一致 + 已有有效 APIKey
{
  "code": "10005",
  "message": "该ICCID已完成APIKey申领，不可重复签发",
  "data": {
    "apiKey": "sk-xxxxxxxx",
    "tokenTotal": 100000,
    "validEndDate": "2026-12-05"
  }
}

// 邮箱不一致
{
  "code": "10005",
  "message": "该ICCID已完成APIKey申领，不可重复签发",
  "data": null
}
```

### 6.2 查询 Token 使用量 — `POST /open/v1/token/usage`

**请求：**

```json
{"iccid": "89860012345678901234"}
```

**流程：**

```
OpenAuth 鉴权
→ Controller:
  1. 根据 ICCID 查用户 → 不存在返回 10001
  2. 查用户是否有额度（Quota > 0）→ 无额度返回 10004
  3. 查用户是否有有效套餐（活跃 UserSubscription）→ 无返回 10004
  4. 从订阅记录获取：
     - tokenTotal = AmountTotal
     - tokenUsed = AmountUsed
     - remainToken = tokenTotal - tokenUsed
     - validEndDate = EndTime，格式 yyyy-MM-dd
  5. 查该用户所有 Log（Type=LogTypeConsume），逐条映射为 usageDetails：
     - usedAt    → CreatedAt 格式化为 yyyy-MM-dd HH:mm:ss
     - tokenCount → Quota
     - scene      → "chat"
     - requestId  → RequestId
     - remark     → ""
  6. 返回
```

**成功返回：**

```json
{
  "code": "00000",
  "message": "success",
  "data": {
    "tokenTotal": 100000,
    "tokenUsed": 2350,
    "remainToken": 97650,
    "validEndDate": "2026-12-02",
    "usageDetails": [
      {
        "usedAt": "2026-05-18 14:32:01",
        "tokenCount": 1200,
        "scene": "chat",
        "requestId": "REQ-20260518-001",
        "remark": ""
      }
    ]
  }
}
```

## 7. 后台管理（/api/openapp/）

### 7.1 后端路由

| 方法 | 路径 | 鉴权 | 说明 |
|------|------|------|------|
| GET | `/api/openapp/` | AdminAuth | 获取开放应用列表 |
| POST | `/api/openapp/` | AdminAuth | 新增（自动生成 AppSecret） |
| PUT | `/api/openapp/` | AdminAuth | 编辑 |
| DELETE | `/api/openapp/:id` | AdminAuth | 删除 |
| POST | `/api/openapp/:id/key` | AdminAuth + CriticalRateLimit + SecureVerification | 查看/刷新 AppSecret |

### 7.2 前端（Classic 主题）

在 `/console/setting` 新增 **"开放应用"** 标签（itemKey: `openapp`）。

**新增文件：**
- `web/classic/src/components/settings/OpenAppSetting.jsx` — 顶层容器
- `web/classic/src/pages/Setting/OpenApp/OpenAppList.jsx` — 列表
- `web/classic/src/pages/Setting/OpenApp/AddEditOpenAppModal.jsx` — 新增/编辑弹窗

**修改文件：**
- `web/classic/src/pages/Setting/index.jsx` — 添加 Tab 项
- `web/classic/src/App.jsx` — 添加后端 API 路由

**列表字段：** AppId、名称、状态、IP 白名单开关、IP 白名单、创建时间、操作。

**弹窗字段：** AppId、AppSecret（查看需安全验证）、名称、状态开关、IP 白名单开关、IP 白名单列表。

Default 主题（React 19 + Base UI）本次暂不实现。

## 8. 业务约束

1. APIKey 与 ICCID 绑定，单个 ICCID 终身仅可申领 1 次 APIKey
2. Token 及 APIKey 有效期由订阅套餐决定，到期自动失效
3. APIKey 消费时从用户账户（User.Quota）扣减额度
4. APIKey 丢失需走人工工单，开放接口不支持重置
5. 开放接口不支持动态修改配额，需在后台人工操作

## 9. 新增文件清单

### 后端

| 文件 | 职责 |
|------|------|
| `model/open_app.go` | OpenApp 模型、缓存、CRUD |
| `controller/openapi.go` | 签发 + 查询两个接口 |
| `controller/openapp.go` | 后台管理 CRUD |
| `service/openapi.go` | 业务逻辑 |
| `dto/openapi.go` | 请求/响应结构体 |
| `middleware/open_auth.go` | OpenAuth 签名鉴权中间件 |
| `router/openapi-router.go` | /open/v1/* 路由注册 |
| `common/openapi_response.go` | 统一返回格式辅助函数 |

### 前端（Classic）

| 文件 | 职责 |
|------|------|
| `web/classic/src/components/settings/OpenAppSetting.jsx` | 容器 |
| `web/classic/src/pages/Setting/OpenApp/OpenAppList.jsx` | 列表 |
| `web/classic/src/pages/Setting/OpenApp/AddEditOpenAppModal.jsx` | 弹窗 |
