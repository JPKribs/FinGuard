.PHONY: build test clean run lint install-tools keys test-proxy

BINARY_NAME=finguard
BUILD_DIR=bin
MAIN_PATH=./cmd/finguard
VERSION=1.0.0b2

build:
	mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)

test:
	go test -v ./...

test-race:
	go test -race -v ./...

clean:
	rm -rf $(BUILD_DIR)
	go clean

run: build
	./$(BUILD_DIR)/$(BINARY_NAME) --config config.yaml

test-proxy: build
	./$(BUILD_DIR)/$(BINARY_NAME) --config config-test.yaml

lint:
	golangci-lint run

fmt:
	go fmt ./...

vet:
	go vet ./...

tidy:
	go mod tidy

install-tools:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

deps:
	go mod download

release:
	mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64-${VERSION}.exe $(MAIN_PATH)
	GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64-${VERSION} $(MAIN_PATH)
	GOOS=linux GOARCH=arm64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64-${VERSION} $(MAIN_PATH)
	GOOS=darwin GOARCH=arm64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64-${VERSION} $(MAIN_PATH)

all: fmt vet test build