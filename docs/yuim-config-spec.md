# yuim 配置规范（common.yml + service.yml）

目标：将 **跨服务通用配置** 与 **服务私有配置** 分离，使用 `-c common.yml,service.yml` 叠加加载，避免重复与遗漏。

> 叠加规则：按顺序加载，后者覆盖前者（同名字段以最后一次为准）。

---

## 1. 文件组织建议

推荐目录：
```text
yuim/
  configs/
    common.yml
    im-ai.yml
    im-job.yml
    im-push.yml
    # 可选：env 分层
    common.prod.yml
    im-ai.prod.yml
```

推荐启动方式：
```bash
./bin/im-ai   -c ./configs/common.yml,./configs/im-ai.yml
./bin/im-job  -c ./configs/common.yml,./configs/im-job.yml
./bin/im-push -c ./configs/common.yml,./configs/im-push.yml
```

如果按环境区分：
```bash
./bin/im-ai -c ./configs/common.yml,./configs/common.prod.yml,./configs/im-ai.yml,./configs/im-ai.prod.yml
```

---

## 2. 字段分层原则（强约束）

### 2.1 必须放到 common.yml 的字段（跨服务共用）
这些字段在 **所有服务** 一般一致（同一环境同一套基础设施）：

- `env`
- `timeout`
- `redis.*`（route/offline/idem/dedupe）
- `rocketmq.name_server`
- （可选）`log.*`（日志级别/格式）
- （可选）`metrics.*`（统一监控端口规划）

> 注意：RocketMQ 的 `topic/group` 通常是 **服务私有**（放 service.yml）。

### 2.2 必须放到 service.yml 的字段（服务私有）
这些字段与服务强绑定：

- im-ai：`http.*`、`mysql.*`、`rocketmq.topic/producer_group/tag`、`idempotency.*`、`outbox.*`、`sync.*`
- im-job：`metrics.*`、`rocketmq.topic/group/tag`、`delivery.*`、`comet.*`、`breaker.*`、`dedupe.*`
- im-push：`http.*`、`comet_addr`、`route_ttl`、`write_timeout`、（可选）`metrics.*`

---

## 3. 统一配置 Schema（建议长期维持）

下面是建议的“总 schema”（不同服务用到的子集不同）。

### 3.1 common.yml（推荐模板）
```yaml
env: dev

timeout: 3s

redis:
  addr: "127.0.0.1:6379"
  password: ""
  db: 0

rocketmq:
  name_server: "127.0.0.1:9876"

# 可选：统一日志配置（如未来加入）
# log:
#   level: "info"
#   encoding: "json"
```

---

## 4. 各服务 service.yml 模板

### 4.1 im-ai.yml（Logic/API）
```yaml
http:
  addr: ":8080"

mysql:
  dsn: "root:password@tcp(127.0.0.1:3306)/yuim?parseTime=true&loc=Local"
  max_open_conns: 50
  max_idle_conns: 25
  conn_max_life: 30m
  conn_max_idle: 5m

rocketmq:
  topic: "im_event"
  tag: "*"
  producer_group: "im-ai"

idempotency:
  ttl: 168h # 7d

outbox:
  tick: 1s
  batch: 200

sync:
  default_limit: 50
  max_limit: 500
```

**说明**
- `timeout` 建议放在 common；若 im-ai 需要更短/更长，可在 im-ai.yml 覆盖。
- outbox worker 的 `tick/batch` 与数据库压力、MQ 压力相关，需压测后调参。

---

### 4.2 im-job.yml（MQ Consumer / Delivery）
```yaml
metrics:
  addr: ":2112"

rocketmq:
  topic: "im_event"
  tag: "*"
  group: "im-job"

delivery:
  op_timeout: 3s
  offline_max_keep: 2000
  vendor_queue_size: 4096
  vendor_workers: 8

comet:
  push_path: "/internal/push"
  push_batch_path: "/internal/push/batch"
  timeout: 2s
  max_batch: 200
  flush_every: 5ms

breaker:
  enabled: true
  threshold: 5
  window: 10s
  open_for: 5s

dedupe:
  enabled: true
  ttl: 24h
```

**说明**
- `rocketmq.group` 必须确保同一套消费集群内一致（同服务多实例共享同 group）。
- `dedupe.ttl` 取决于你允许重复投递的窗口，一般 6h~72h 都可，按业务容忍度与 Redis 成本权衡。

---

### 4.3 im-push.yml（Comet / WebSocket）
```yaml
http:
  addr: ":7001"

# 分布式部署：必须写“im-job 可达”的地址（内网 IP / 服务发现域名）
comet_addr: "10.0.0.21:7001"

redis:
  # 如果你把 redis 放在 common，这里可以省略；需要覆盖时可写
  addr: "127.0.0.1:6379"
  password: ""
  db: 0

route_ttl: 60
write_timeout: 5s
```

**说明**
- `comet_addr` 是最容易出错的点：不能写 127.0.0.1，必须能被 im-job 访问。
- `route_ttl` 建议配合“续期/心跳”机制（后续会补）。

---

## 5. 环境分层规范（推荐）

推荐每个环境一套覆盖文件：
```text
configs/
  common.yml
  common.prod.yml
  im-ai.yml
  im-ai.prod.yml
  ...
```

示例 `common.prod.yml`：
```yaml
env: prod
redis:
  addr: "10.0.0.5:6379"
rocketmq:
  name_server: "10.0.0.6:9876"
timeout: 2s
```

---

## 6. 校验与防漏（建议强制）

### 6.1 启动时强校验（推荐）
每个服务启动时做：
- 必填项为空直接 fatal
- 关键字段打印一行（脱敏）便于排障

必填建议：
- common：`redis.addr`、`rocketmq.name_server`
- im-ai：`mysql.dsn`、`rocketmq.topic/producer_group`
- im-job：`rocketmq.topic/group`、`comet.push_batch_path`
- im-push：`comet_addr`、`http.addr`

### 6.2 变更流程建议
- common.yml 任何改动 → 必须走 code review
- service.yml 改动 → 必须在对应服务的 README 里更新参数说明

---

## 7. 常见坑清单（上线前自检）

- [ ] common.yml 中 redis/rocketmq 改了，但某个服务的 service.yml 里又重复定义了旧值（被覆盖了）
- [ ] im-push 的 comet_addr 写成 127.0.0.1 或公网域名（im-job 内网不可达）
- [ ] im-job 的 rocketmq.group 不一致，导致多实例消费错乱
- [ ] outbox tick 太小导致数据库压力异常
- [ ] dedupe ttl 太大导致 Redis key 量暴涨（需要估算：QPS * TTL）

---

## 8. 约定：conv_id 与 talk_type/group_id 的关系（建议）

为了兼容现有 `chat_msg` 字段，同时支持按会话拉取：

- 单聊：`talk_type = '1'`，`conv_id = "p2p:<min(uid1,uid2)>:<max(uid1,uid2)>"`，`group_id = NULL`
- 群聊：`talk_type = '2'`，`conv_id = "g:<group_id>"`，`receive_id = NULL`

这样：
- `/sync` 只需要 `uid + conv_id + after_sync_id`
- 存储结构仍与现有表字段一致

