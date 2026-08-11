# HuiPay 后端 Makefile

GO         ?= go
BIN_DIR    ?= bin
APP_NAME   ?= huipay-server
MAIN_PKG   ?= ./cmd/server

# 数据库连接 DSN（不含 mysql:// 前缀，migrate-up/down 会拼接）。
# 数据库名默认 huipay；golang-migrate 用 schema_migrations 表追踪版本。
# 密码含特殊字符（如 lijing123!@#）时，须 URL 编码（@ -> %40, # -> %23），
# 或通过环境变量注入：MYSQL_DSN="root:xxx@tcp(127.0.0.1:3306)/huipay?charset=utf8mb4&parseTime=True&loc=Local" make migrate-up
MYSQL_DSN  ?= root:CHANGE_ME@tcp(127.0.0.1:3306)/huipay?charset=utf8mb4&parseTime=True&loc=Local

MIGRATE_DIR ?= infra/migrator/migrations

.PHONY: help run build test lint migrate-up migrate-down migrate-create tidy clean

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
	migrate -path $(MIGRATE_DIR) -database "mysql://$(MYSQL_DSN)" up

migrate-down:
	migrate -path $(MIGRATE_DIR) -database "mysql://$(MYSQL_DSN)" down 1

migrate-create:
	@test -n "$(NAME)" || (echo "usage: make migrate-create NAME=desc"; exit 1)
	migrate create -ext sql -dir $(MIGRATE_DIR) -seq $(NAME)

tidy:
	$(GO) mod tidy

clean:
	rm -rf $(BIN_DIR)