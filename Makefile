.PHONY: run migrate-up migrate-down build test test-required test-race lint tidy compose-up compose-infra compose-monitoring compose-monitoring-stop compose-down

run:
	go run ./cmd/wms

migrate-up:
	go run ./cmd/migrate -seed up

migrate-down:
	go run ./cmd/migrate -steps 1 down

build:
	go build -o bin/wms.exe ./cmd/wms

test:
	go test ./... -v

test-required:
	WMS_TEST_REQUIRED=1 go test ./internal/... -v -count=1

test-race:
	CGO_ENABLED=1 go test -race ./internal/... -count=1

lint:
	gofmt -l .
	go vet ./...

tidy:
	go mod tidy

compose-up:
	docker compose -f deploy/docker-compose.yaml up -d --build

compose-infra:
	docker compose -f deploy/docker-compose.yaml -f deploy/docker-compose.dev.yaml up -d mysql redis

compose-monitoring:
	docker compose -f deploy/docker-compose.yaml --profile monitoring up -d prometheus grafana

compose-monitoring-stop:
	docker compose -f deploy/docker-compose.yaml --profile monitoring stop prometheus grafana

compose-down:
	docker compose -f deploy/docker-compose.yaml down
