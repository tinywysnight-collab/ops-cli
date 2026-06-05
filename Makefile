BINARY := opsx
PKG := ./cmd/opsx
BIN_DIR := bin

# Build a single static binary (no CGO) for the host platform.
.PHONY: build
build:
	CGO_ENABLED=0 go build -o $(BIN_DIR)/$(BINARY) $(PKG)

# Cross-compile the primary (darwin), secondary (linux), and Windows targets.
.PHONY: cross
cross:
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o $(BIN_DIR)/$(BINARY)-darwin-arm64 $(PKG)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o $(BIN_DIR)/$(BINARY)-darwin-amd64 $(PKG)
	CGO_ENABLED=0 GOOS=linux  GOARCH=arm64 go build -o $(BIN_DIR)/$(BINARY)-linux-arm64 $(PKG)
	CGO_ENABLED=0 GOOS=linux  GOARCH=amd64 go build -o $(BIN_DIR)/$(BINARY)-linux-amd64 $(PKG)
	$(MAKE) windows

.PHONY: windows
windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o $(BIN_DIR)/$(BINARY)-windows-amd64.exe $(PKG)

.PHONY: test
test:
	go test -race -cover ./...

.PHONY: lint
lint:
	test -z "$$(gofmt -l .)"
	go vet ./...
	golangci-lint run ./...

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: clean
clean:
	rm -rf $(BIN_DIR)
