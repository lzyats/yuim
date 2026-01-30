# im-ai（Logic / API）

- 落库：`chat_msg`（兼容现有字段）
- conv 级序号：`chat_msg.sync_id`（由 `im_conv_seq` 生成）
- Outbox：`im_outbox`（DB + MQ 最终一致，且入队幂等）

## 发送消息
POST /send

### 单聊
```json
{"user_id":1001,"receive_id":2002,"talk_type":"1","client_msg_id":"c-1","msg_type":"TEXT","content":{"text":"hi"}}
```

### 群聊（下游 im-job 负责 fanout）
```json
{"user_id":1001,"group_id":123,"talk_type":"2","client_msg_id":"c-2","msg_type":"TEXT","content":{"text":"hello"}}
```

## 增量同步 /sync
- 指定 conv_id：
`GET /sync?uid=1001&conv_id=p2p:1001:2002&after_sync_id=0&limit=50`
- 单聊：
`GET /sync?uid=1001&talk_type=1&peer_id=2002&after_sync_id=0`
- 群聊：
`GET /sync?uid=1001&talk_type=2&group_id=123&after_sync_id=0`
