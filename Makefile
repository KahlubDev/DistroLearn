.PHONY: check lint test build

check: lint test build

lint:
	pnpm lint
	pnpm typecheck
	cd services && gofmt -l . && go vet ./...

test:
	pnpm test
	cd services && go test ./...

build:
	pnpm build
	cd services && go build ./...
