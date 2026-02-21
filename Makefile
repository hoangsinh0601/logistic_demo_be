.PHONY: all build run test clean docker-up docker-down swagger

APP_NAME=backend
MAIN_FILE=cmd/api/main.go

all: build

build:
	@echo "Building..."
	@go build -o bin/$(APP_NAME) $(MAIN_FILE)

run:
	@echo "Running..."
	@go run $(MAIN_FILE)

test:
	@echo "Testing..."
	@go test ./... -v

clean:
	@echo "Cleaning..."
	@rm -rf bin

docker-up:
	@echo "Starting Docker containers with build..."
	docker compose -f deployments/docker-compose.yml up -d --build

docker-down:
	@echo "Stopping Docker containers..."
	docker compose -f deployments/docker-compose.yml down

swagger:
	@echo "Generating Swagger docs..."
	$$(go env GOPATH)/bin/swag init -g $(MAIN_FILE) --output api/swagger
