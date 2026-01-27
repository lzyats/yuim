module yuim/services/im-ai

go 1.22

require (
	github.com/go-sql-driver/mysql v1.8.1
	github.com/sony/sonyflake v1.3.0
	go.uber.org/zap v1.27.0
	gopkg.in/yaml.v3 v3.0.1

	github.com/lzyats/core-push-go v0.0.0
)

replace github.com/lzyats/core-push-go => ../../libs/core-push-go
