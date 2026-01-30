# yuim 配置叠加规范

yuim 支持配置文件叠加：

```bash
-c common.yml,service.yml
```

按顺序加载，后者覆盖前者（同名字段以最后一次为准）。

## Redis 字段统一

所有服务统一使用：

```yaml
redis:
  addr: "127.0.0.1:6379"
  password: ""
  database: 0
```

## 建议目录

```text
configs/
  common.yml
  im-ai.yml
  im-job.yml
  im-push.yml
```
