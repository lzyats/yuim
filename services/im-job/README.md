# im-job（RocketMQ Consumer / 投递服务）

角色：订阅 RocketMQ（**CLUSTERING**）→ 查 Redis 路由 → **按 Comet 节点批量定向投递** → 失败则入离线（可选第三方推送）。

## 关键优化（为 50W 在线准备）
- **Batching**：同一 Comet 节点的投递合并成批量请求，减少 RPC/HTTP 开销
- **Retry**：批量投递失败会进行一次快速重试（默认 20ms backoff）
- **Circuit Breaker**：某个 Comet 节点连续失败达到阈值后短暂熔断，直接降级到离线，避免雪崩

## 运行
```bash
go run ./cmd/im-job -c ./config.yml
```

## 依赖
- RocketMQ
- Redis
- Comet（im-push）需要提供内部投递接口：单条 + 批量

## Comet 内部投递接口约定

### 1) 单条投递（可选）
`POST /internal/push`
```json
{ "uid": 1001, "packet_json": "{...}" }
```

### 2) 批量投递（推荐）
`POST /internal/push/batch`
```json
{
  "items": [
    { "uid": 1001, "packet_json": "{...}" },
    { "uid": 1002, "packet_json": "{...}" }
  ]
}
```
返回 2xx 表示整批投递成功；非 2xx 视为失败，将触发重试/离线降级。

## Metrics
- `/metrics` 暴露 Prometheus 指标：消费量、解码失败、批量投递、熔断次数、离线入队等。
