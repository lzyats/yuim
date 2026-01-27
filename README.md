# yuim

yuim 是一个 Go 单仓（monorepo）IM 平台工程，按 GOIM 思路拆分为三大服务：

- `services/im-ai`：Logic / API（鉴权、业务校验、落库、发布 MQ 事件）
- `services/im-job`：投递服务（RocketMQ CLUSTERING 消费 → Redis 路由 → 定向投递 Comet → 失败入离线）
- `services/im-push`：Comet（WebSocket 长连接与下发通道）

基础能力库：
- `libs/core-push-go`：Delivery Engine（路由/离线/ACK/幂等/第三方推送封装）

文档：
- `docs/IM-Platform-Architecture.md`

## 开发模式
本仓库使用 **go.work** 连接多模块，方便本地开发与独立打包。

## 快速启动（demo）
1) 启动 RocketMQ、Redis
2) 启动 Comet：
```bash
cd services/im-push
go run ./cmd/im-push -c ./config.yml
```
3) 启动 Job：
```bash
cd services/im-job
go run ./cmd/im-job -c ./config.yml
```
4) 启动 API：
```bash
cd services/im-ai
go run ./cmd/im-ai -c ./config.yml
```

## 构建二进制
在仓库根目录：
```bash
make build
```
输出到 `bin/`。


### im-ai 数据库
- `services/im-ai/schema.sql`：消息落库表结构（最小版）。


## 配置文件通用化建议
- 采用 `-c common.yml,service.yml` 叠加加载，避免重复与遗漏（已在 im-ai 实现）。
