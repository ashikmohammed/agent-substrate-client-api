.PHONY: api ui test

api:
	cd api && go run ./cmd/agentsubstrate-api

ui:
	cd ui && npm run dev

test:
	cd api && go test ./...
