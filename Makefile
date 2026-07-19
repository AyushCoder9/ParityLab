.PHONY: dev test build verify e2e infra-up infra-down

dev:
	@echo "Web: pnpm dev"
	@echo "API: go run ./services/api/cmd/paritylab"
	@echo "Worker: go run ./services/api/cmd/worker"

test:
	pnpm test
	go test ./...

build:
	pnpm build
	go build ./services/api/cmd/...

verify:
	pnpm lint
	pnpm typecheck
	pnpm test
	go test ./...
	pnpm build
	go build ./services/api/cmd/...

e2e:
	pnpm e2e

infra-up:
	docker compose -f infra/compose.yaml up -d

infra-down:
	docker compose -f infra/compose.yaml down
