APP_VERSION ?= dev

.PHONY: build tidy test

build:
	@mkdir -p bin
	@echo ">> build im-ai"
	@cd services/im-ai && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.Version=$(APP_VERSION)" -o ../../bin/im-ai ./cmd/im-ai
	@echo ">> build im-job"
	@cd services/im-job && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.Version=$(APP_VERSION)" -o ../../bin/im-job ./cmd/im-job
	@echo ">> build im-push"
	@cd services/im-push && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.Version=$(APP_VERSION)" -o ../../bin/im-push ./cmd/im-push
	@echo ">> done. bin/ contains: im-ai im-job im-push"

tidy:
	@cd libs/core-push-go && go mod tidy
	@cd services/im-ai && go mod tidy
	@cd services/im-job && go mod tidy
	@cd services/im-push && go mod tidy

test:
	@cd libs/core-push-go && go test ./...
	@cd services/im-ai && go test ./...
	@cd services/im-job && go test ./...
	@cd services/im-push && go test ./...
