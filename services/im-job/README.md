# im-job（MQ Consumer / Delivery）

## v9 新增：群聊 fanout（成员展开）
当收到事件：
- `evt.ConvID` 形如 `g:<group_id>`
- 且 `evt.ToUIDs` 为空（im-ai 群聊发送默认空）

im-job 会：
1. 查询 MySQL `im_group_member`（status=1）得到群成员
2. 过滤掉发送者 `FromUID`
3. 将成员列表写入 `evt.ToUIDs`，走统一投递/离线流程

### 缓存
- 进程内 TTL 缓存（默认 30s）减少 DB 压力
- 超大群建议后续：分页、Redis set 缓存、或专门的群成员服务
