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
mysql -uroot -e "CREATE DATABASE huipay_main DEFAULT CHARSET utf8mb4;"

# 2. 跑迁移
make migrate-up

# 3. 编译 & 启动
make build
./bin/huipay-server
```

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