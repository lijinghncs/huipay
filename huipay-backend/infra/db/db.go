// 包 db 提供 MySQL 主从连接初始化。
package db

import (
	"context"
	"fmt"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/huipay/huipay-backend/infra/config"
)

// DB 封装主从连接（主库可写，从库读）。
type DB struct {
	Master *gorm.DB
	Slave  *gorm.DB
}

// MustOpen 初始化主库连接（从库可选）。
func MustOpen(cfg *config.Config, log logger.Interface) (*DB, error) {
	master, err := open(cfg.MySQLMaster, log)
	if err != nil {
		return nil, fmt.Errorf("open master failed: %w", err)
	}
	out := &DB{Master: master}
	if cfg.MySQLSlave != "" {
		slave, err := open(cfg.MySQLSlave, log)
		if err == nil {
			out.Slave = slave
		}
	}
	return out, nil
}

func open(dsn string, log logger.Interface) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: log,
		NowFunc: func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)
	return db, nil
}

// HealthCheck 用于 /healthz 探活。
func (d *DB) HealthCheck(ctx context.Context) error {
	sqlDB, err := d.Master.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}