# HuiPay 后端 Makefile

GO         ?= go
BIN_DIR    ?= bin
APP_NAME   ?= huipay-server
MAIN_PKG   ?= ./cmd/server

MYSQL_DSN  ?= huipay:huipay@tcp(127.0.0.1:3306)/huipay_main?charset=utf8mb4&parseTime=True&loc=Local

.PHONY: help run build test lint migrate-up migrate-down tidy clean

help:
	@echo "Targets:"
	@echo "  run              - run app in foreground"
	@echo "  build            - build binary to bin/$(APP_NAME)"
	@echo "  test             - run unit tests"
	@echo "  lint             - run golangci-lint"
	@echo "  migrate-up       - run DB migrations up"
	@echo "  migrate-down     - rollback last migration"
	@echo "  tidy             - go mod tidy"
	@echo "  clean            - remove bin/"

run:
	$(GO) run $(MAIN_PKG)

build:
	mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/$(APP_NAME) $(MAIN_PKG)

test:
	$(GO) test ./... -count=1

lint:
	golangci-lint run ./...

migrate-up:
	migrate -path migrations -database "mysql://$(MYSQL_DSN)" up

migrate-down:
	migrate -path migrations -database "mysql://$(MYSQL_DSN)" down 1

tidy:
	$(GO) mod tidy

clean:
	rm -rf $(BIN_DIR)