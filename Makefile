.PHONY: all build run test clean

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
