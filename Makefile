.PHONY: generate migrate migrate-down migrate-status run lint fmt test test-integration mutation

COVER_PROFILE := coverage.out
COVER_PACKAGES := ./internal/usecase/... ./internal/domain/...

generate:
	go tool oapi-codegen --config oapi-codegen.yaml contracts/openapi/trip-service.openapi.yaml
	go generate ./...

migrate:
	set -a; . ./.env; set +a; go tool goose -dir migrations postgres "$${DATABASE_URL}" up

migrate-down:
	set -a; . ./.env; set +a; go tool goose -dir migrations postgres "$${DATABASE_URL}" down

migrate-status:
	set -a; . ./.env; set +a; go tool goose -dir migrations postgres "$${DATABASE_URL}" status

run:
	set -a; . ./.env; set +a; go run ./cmd/trip-service

lint:
	golangci-lint run ./...

fmt:
	golangci-lint fmt ./...

test:
	go test -race -count=1 ./...
	go test -race -count=1 -cover -coverprofile=$(COVER_PROFILE) $(COVER_PACKAGES)
	@go tool cover -func=$(COVER_PROFILE) | tail -n 1

test-integration:
	set -a; . ./.env; set +a; go test -race -count=1 -tags=integration ./internal/repository/...

mutation:
	gremlins unleash ./internal/domain --invert-assignments --invert-bitwise --invert-logical --invert-loopctrl --remove-self-assignments --timeout-coefficient 30
	gremlins unleash ./internal/usecase --invert-assignments --invert-bitwise --invert-logical --invert-loopctrl --remove-self-assignments --timeout-coefficient 30
