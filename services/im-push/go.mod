module yuim/services/im-push

go 1.22

require (
	github.com/prometheus/client_golang v1.19.0
	github.com/gorilla/websocket v1.5.1
	go.uber.org/zap v1.27.0
	gopkg.in/yaml.v3 v3.0.1

	github.com/lzyats/core-push-go v0.0.0
)

replace github.com/lzyats/core-push-go => ../../libs/core-push-go
