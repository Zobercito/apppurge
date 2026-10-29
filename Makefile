.PHONY: build dist test lint fmt install clean

BINARY_NAME=apppurge
BUILD_DIR=build
VERSION ?= dev
LDFLAGS=-X main.version=$(VERSION)

build:
	mkdir -p $(BUILD_DIR)
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) .

dist:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w $(LDFLAGS)" -o dist/$(BINARY_NAME)-$(VERSION)-linux-amd64 .
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w $(LDFLAGS)" -o dist/$(BINARY_NAME)-$(VERSION)-linux-arm64 .
	cd dist && sha256sum $(BINARY_NAME)-$(VERSION)-linux-* > SHA256SUMS

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
	rm -rf $(BUILD_DIR) dist coverage.out
