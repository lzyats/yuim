module yuim/services/im-job

go 1.22

require (
	github.com/apache/rocketmq-client-go/v2 v2.1.2
	github.com/prometheus/client_golang v1.19.0
	go.uber.org/zap v1.27.0
	gopkg.in/yaml.v3 v3.0.1

	// core-push-go (your module path; adjust to your repo)
	github.com/lzyats/core-push-go v0.0.0
)

// If you develop in a mono-repo, uncomment and adjust:
// replace github.com/lzyats/core-push-go => ../../libs/core-push-go
