.PHONY: generate migrate migrate-down migrate-status run

generate:
	go tool oapi-codegen --config oapi-codegen.yaml contracts/openapi/trip-service.openapi.yaml

migrate:
	set -a; . ./.env; set +a; go tool goose -dir migrations postgres "$${DATABASE_URL}" up

migrate-down:
	set -a; . ./.env; set +a; go tool goose -dir migrations postgres "$${DATABASE_URL}" down

migrate-status:
	set -a; . ./.env; set +a; go tool goose -dir migrations postgres "$${DATABASE_URL}" status

run:
	set -a; . ./.env; set +a; go run ./cmd/trip-service
