.PHONY: build test lint fmt install clean

BINARY_NAME=appurge
BUILD_DIR=build

build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) .

test:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

lint:
	golangci-lint run

fmt:
	go fmt ./...

install: build
	sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/

clean:
	rm -rf $(BUILD_DIR) coverage.out
