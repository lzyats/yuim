# core-push-go

`core-push-go` 是 Java `core-push` 的 Go 版落地实现（按 **Git 仓库**方式使用，不是父项目的子 module）。

当前版本重点实现：

1. **个推（GeTui RestAPI v2）推送**：从 Redis 队列取出消息后，按 CID 推送。
2. **RocketMQ**：同一条消息同时写入 RocketMQ（可作为后续异步/审计/分发）。
3. **Redis 队列消费**：通过 `BRPOP` 从指定 key（list）阻塞获取消息。

> 你提到的示例 key：`push:user:20260129:2000814672382668802`，在本项目里可直接作为 `redis.queue-key` 配置使用。

---

## 目录结构

```
core-push-go/
  cmd/
    worker/               # 独立 worker，可部署为 systemd 服务
  pkg/
    push/                 # 配置、类型、错误定义
    provider/
      getui/              # 个推 RestAPI v2（获取 token、推送）
      rocketmq/           # RocketMQ producer（发布消息）
    store/
      redis/              # Redis 队列（BRPOP） + 消息解析
    runner/               # Worker：串联 Redis -> Getui -> RocketMQ
```

---

## 消息格式

Redis 队列（list）里每个元素建议放 JSON：

```json
{
  "title": "标题",
  "body": "内容",
  "targets": ["CID1", "CID2"],
  "data": {
    "type": "chat",
    "id": "123"
  }
}
```

兼容模式：如果队列里放的不是 JSON（例如直接是一个 CID 字符串），则会当成：

- `targets=[payload]`
- `title/body` 为空

---

## 配置（YML）

示例配置（对应你给的父项目 YML 结构）：

```yml
# MQ配置
rocketmq:
  enabled: Y
  name-server: 110.42.56.25:9876
  producer:
    access-key: admin123
    secret-key: admin123admin123
    group: testim
  topic: testim
  # tag: 可选

# 推送配置（个推）
push:
  enabled: Y
  appId: 0RbcZP7qpW85LeTKqhtDw1
  appKey: Tscywps0VSAKcjI4B1rEr3
  appSecret: m0wOSAPWOp6F7ormKOvD08
  masterSecret: WXc25xbCrrAsS4ZILOl8y7
  # baseUrl: https://restapi.getui.com/v2   # 可不填
  # ttl: 7200000                           # 可不填，毫秒

# redis 配置
redis:
  enabled: Y
  host: 110.42.56.66
  port: 6379
  database: 0
  password: 555
  timeout: 5s

  # 重点：这里填你实际队列 key
  queue-key: push:user:20260129:2000814672382668802

  lettuce:
    pool:
      max-idle: 128
      max-active: 128
      max-wait: 5s
```

---

## 运行方式

### 1）作为 Worker 运行

```bash
go run ./cmd/worker -config /path/to/config.yml
```

或使用环境变量：

```bash
set CORE_PUSH_CONFIG=C:\path\to\config.yml
core-push-worker.exe
```

（Linux）

```bash
CORE_PUSH_CONFIG=/etc/core-push.yml ./core-push-worker
```

---

## 行为说明

- Redis：使用 `BRPOP(queue-key, 5s)` 阻塞取任务，支持多实例水平扩展。
- 个推：
  - 通过 `BaseUrl/{appId}/auth` 获取 token（sign=SHA256(appkey+timestamp+mastersecret)）。citeturn4search0
  - 通过 `BaseUrl/{appId}/push/single/cid` 推送，Header `token` 带上鉴权 token。citeturn3view0
- RocketMQ：把同一条消息 JSON 序列化后写入配置的 `topic`。

---

## 依赖与 Go 版本

- Go 1.22+
- RocketMQ：`github.com/apache/rocketmq-client-go/v2`（官方 Go 客户端）citeturn0search15
- Redis：`github.com/redis/go-redis/v9`（go.mod 最低 Go 1.21，本项目 Go 1.22 OK）citeturn0search3

---

## 下一步（按你父项目的真实消息结构再对齐）

你只要告诉我 Java 版往这个 key 里 `LPUSH/RPUSH` 的 payload 结构（或者贴一条样例），我就可以把：

- payload -> `title/body/targets/data` 的映射规则
- 对接 RocketMQ 的消息结构（topic/tag/keys）
- 推送失败重试 / dead letter 队列

全部按父项目的逻辑对齐。


## Refactor Notes (Delivery Engine 방향)

- 新增 `pkg/delivery`：投递引擎（路由、离线、ACK、可选第三方推送异步队列）。
- 新增 `pkg/event`：IM 内部事件契约（Logic -> MQ -> Job）。
- 新增 `pkg/producer`：仅负责 RocketMQ 生产（不消费）。
- 新增 `pkg/store/storeiface`：投递状态存储接口。
- Redis Store 扩展：route/offline/idem 三类能力（ZSET 离线队列）。

下一步建议：把 `cmd/worker` 改名为 `im-job` 并迁移 MQ 消费到那里，`core-push-go` 本身不再承担消费职责。
