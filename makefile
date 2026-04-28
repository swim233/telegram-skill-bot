.PHONY: build build-all clean run test push

# 版本号：优先用 git tag，否则用短哈希
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BINARY_NAME := telegram-chat-skill-bot
LDFLAGS := -s -w -X main.Version=$(VERSION)

# 默认目标平台
GOOS ?= linux
GOARCH ?= amd64

OUTPUT := ./bin/$(BINARY_NAME)-$(VERSION)-$(GOOS)-$(GOARCH)

build:
	mkdir -p ./bin
	cd src && CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags "$(LDFLAGS)" -o ../$(OUTPUT) .
	upx --best --lzma $(OUTPUT) -o $(OUTPUT)-upx || true
	@echo "Built: $(OUTPUT)"

build-all:
	@$(MAKE) build GOOS=linux GOARCH=amd64
	@$(MAKE) build GOOS=linux GOARCH=arm64
	@$(MAKE) build GOOS=darwin GOARCH=amd64
	@$(MAKE) build GOOS=darwin GOARCH=arm64
	@$(MAKE) build GOOS=windows GOARCH=amd64

run:
	cd src && go run .

test:
	cd src && go test ./...


clean:
	rm -rf ./bin

-include makefile.local
