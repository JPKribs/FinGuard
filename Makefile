.PHONY: build test clean run lint install-tools version release

BINARY_NAME=finguard
BUILD_DIR=bin
VERSION=1.0.0-dev

build:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build -ldflags "-X main.Version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/finguard

test:
	go test -v ./...

clean:
	rm -rf $(BUILD_DIR)
	go clean

run: build
	./$(BUILD_DIR)/$(BINARY_NAME) --config config.yaml

fmt:
	go fmt ./...

vet:
	go vet ./...

tidy:
	go mod tidy

version:
	@echo $(VERSION)

release:
	mkdir -p $(BUILD_DIR)
	@echo "Building release binaries for version $(VERSION)..."
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-X main.Version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64-$(VERSION).exe ./cmd/finguard
	GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "-X main.Version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-arm64-$(VERSION).exe ./cmd/finguard
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-X main.Version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64-$(VERSION) ./cmd/finguard
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "-X main.Version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64-$(VERSION) ./cmd/finguard
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-X main.Version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64-$(VERSION) ./cmd/finguard
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "-X main.Version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64-$(VERSION) ./cmd/finguard
	@echo "Release binaries built in $(BUILD_DIR)/"
	@ls -la $(BUILD_DIR)/*-$(VERSION)*

all: fmt vet test build