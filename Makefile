.PHONY: dev build run lint fmt migrate-up migrate-down migrate-create test setup clean genkey

include .env
export

dev:
	air

build:
	go build -o ./tmp/main.exe ./cmd/cage

run:
	go run ./cmd/cage

lint:
	golangci-lint run

fmt:
	gofmt -w .
	goimports -w .

migrate-up:
	migrate -database "$(DATABASE_URL)" -path migrations up

migrate-down:
	migrate -database "$(DATABASE_URL)" -path migrations down 1

migrate-create:
	migrate create -ext sql -dir migrations -seq $(name)

test:
	go test ./... -v

test-short:
	go test ./... -short -v

test-converge:
	go test ./... -coverprofile=coverage.out
	go toll cover -html=coverage.out -o coverage.html

setup:
	lefthook install
	@echo "Setup complete. Copy .env.example to .env and fill in your values."

clean:
	rm -rf tmp

genkey:
	go run ./cmd/genkey -name=$(name)