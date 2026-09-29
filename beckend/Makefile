.PHONY: run test vet build up down
run:
	go run ./cmd/api
test:
	go test -race ./...
vet:
	go vet ./...
build:
	go build -o bin/api ./cmd/api
	go build -o bin/import-track ./cmd/import-track
up:
	docker compose up -d --build
down:
	docker compose down
