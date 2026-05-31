.PHONY: dev-api dev-web build test tidy

tidy:
	go mod tidy

test:
	go test ./...

# Build frontend first, then embed into the Go binary.
build:
	cd web && npm install && npm run build
	go build -o bin/github-stats ./cmd/server

dev-api:
	go run ./cmd/server

dev-web:
	cd web && npm run dev
