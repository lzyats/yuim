# im-push（Comet / WebSocket）

骨架版本（用于联调）：提供
- `/ws?uid=...` WebSocket（demo）
- `/internal/push` 单条投递
- `/internal/push/batch` 批量投递（给 im-job 使用）
- `/metrics` Prometheus 指标（在线连接数/下发统计/背压/离线）

后续需要替换：
- uid 来源：token 鉴权
- 协议：protobuf
- 更完整的心跳/ACK/断线重连/路由续期
