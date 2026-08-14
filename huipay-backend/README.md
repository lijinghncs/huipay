# huipay-backend

HuiPay 后端单体（Go 1.22 + Gin + GORM + MySQL 8.0）。

## 目录结构

```
cmd/server/                启动入口
infra/                     基础设施（config/db/obs/idem/errs/lock/outbox/prom/cache）
internal/domain/           领域层（entity / vo，不依赖任何业务包）
internal/order/            订单服务
internal/payment/          支付服务（channel/router/pricer）
internal/account/          账户服务（ledger/repository/service/handler）
internal/split/            分账服务（rule/executor/service/handler）
migrations/                golang-migrate SQL 文件
Makefile                   构建脚本
```

## 快速开始

```bash
# 1. 准备 MySQL
mysql -uroot -e "CREATE DATABASE huipay DEFAULT CHARSET utf8mb4;"

# 2. 安装 golang-migrate（带 mysql 驱动）
go install -tags 'mysql' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
# 确保 $(go env GOPATH)/bin 在 PATH 中

# 3. 跑迁移（DSN 默认 root:CHANGE_ME@.../huipay，密码含特殊字符须 URL 编码或用环境变量覆盖）
MYSQL_DSN="root:xxx@tcp(127.0.0.1:3306)/huipay?charset=utf8mb4&parseTime=True&loc=Local" make migrate-up

# 4. 编译 & 启动
make build
./bin/huipay-server

# 或者直接 go run 启动（开发期推荐）
go run ./cmd/server
```

## 数据库迁移（golang-migrate）

本项目用 [golang-migrate](https://github.com/golang-migrate/migrate) 统一管理 schema（`schema_migrations` 表追踪版本），避免"代码有列、库无列"的漂移。

- **迁移文件**：`infra/migrator/migrations/NNNN_*.{up,down}.sql`，按数字前缀递增。
- **新增迁移**：`make migrate-create NAME=描述`（或 CLI `migrate create -ext sql -dir infra/migrator/migrations -seq 描述`），然后编辑 up/down SQL。
- **执行迁移**：`make migrate-up`。应用启动时也会自动执行（`infra/migrator` 内嵌，见下）。
- **回滚一个**：`make migrate-down`。
- **查看版本**：`migrate -path infra/migrator/migrations -database "mysql://$(MYSQL_DSN)" version`。

> DSN 密码若含特殊字符（`@` `#` 等），在 URL 中必须编码（`@`→`%40`，`#`→`%23`），否则 DSN 解析会错位。最稳妥是通过 `MYSQL_DSN` 环境变量注入。

### 迁移内嵌到应用（推荐用于部署）

迁移 SQL 通过 `//go:embed` 打进二进制，应用启动时经 [infra/migrator](file:///d:/code/汇聚付/huipay/huipay-backend/infra/migrator/migrator.go) 的 `migrator.Run()` **自动执行未应用的迁移**（幂等：已应用则跳过）。因此服务器部署时**无需**额外安装 migrate CLI 或手动跑迁移，直接启动新版本即可。

## 关键接口

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/healthz` | 健康检查 |
| GET | `/metrics` | Prometheus 指标 |
| POST | `/v1/checkout/precreate` | 预下单 |
| GET | `/v1/checkout/:order_no` | 查询订单 |
| POST | `/v1/checkout/:order_no/refund` | 退款（骨架） |
| GET | `/v1/wallets/:entity_id` | 查询钱包 |
| GET | `/v1/wallets/:entity_id/entries` | 账本流水 |
| POST | `/v1/split/execute` | 触发分账 |
| GET | `/v1/split/:order_no` | 查询分账结果 |

## 环境变量

| 变量 | 默认 | 说明 |
|---|---|---|
| `HUIPAY_HTTP_PORT` | 8080 | 监听端口 |
| `HUIPAY_GIN_MODE` | release | Gin 模式 |
| `HUIPAY_LOG_LEVEL` | info | 日志级别 |
| `HUIPAY_MYSQL_MASTER` | - | 主库 DSN |
| `HUIPAY_MYSQL_SLAVE` | - | 从库 DSN（可选） |