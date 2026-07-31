.PHONY: up down migrate-up migrate-down run test

up:
	docker compose up -d

down:
	docker compose down

migrate-up:
	go run ./cmd/migrate -direction up

migrate-down:
	go run ./cmd/migrate -direction down

run:
	go run ./cmd/kestrel

test:
	go test ./...
