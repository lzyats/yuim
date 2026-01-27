# im-ai（Logic / API）

## 与现有 `chat_msg` 结合
沿用你的 `chat_msg` 表结构，并扩展两列（推荐）：
- `conv_id`：会话ID（单聊建议用 `p2p:uidSmall:uidBig`，群聊建议用 `g:groupId`）
- `client_msg_id`：客户端幂等ID

`sync_id` 作为 **conv 级 seq** 使用（同一会话内自增）。

表结构见 `schema.sql`（同时包含 outbox / conv_seq）。

## Outbox 最终一致（已补齐）
发送时在 **同一 MySQL 事务**中：
1) 生成 `sync_id`
2) 插入 `chat_msg`
3) 插入 `im_outbox`（返回 outbox_id）

事务提交后：
- inline publish 尝试发布 MQ
- inline publish 成功：`MarkSent(outbox_id)`
- 失败：后台 outbox worker 扫描补偿重发（直到成功）

## 离线拉取（按 sync_id 增量）
`GET /sync?uid=1001&conv_id=p2p:1001:2002&after_sync_id=0&limit=50`

## 启动（支持配置叠加）
```bash
go run ./cmd/im-ai -c ./config.common.yml,./config.yml
```
