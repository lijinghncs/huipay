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

// silentLogger 不打 SQL 日志（GORM logger.Interface 用 interface{} 实现）；
// 真实项目应将 GORM 事件转发到 zap，此处先静默避免编译失败。
type silentLogger struct{}

func (silentLogger) LogMode(logger.LogLevel) logger.Interface { return silentLogger{} }
func (silentLogger) Info(_ context.Context, _ string, _ ...interface{})    {}
func (silentLogger) Warn(_ context.Context, _ string, _ ...interface{})    {}
func (silentLogger) Error(_ context.Context, _ string, _ ...interface{})   {}
func (silentLogger) Trace(_ context.Context, _ time.Time, _ func() (string, int64), _ error) {}

func open(dsn string, log logger.Interface) (*gorm.DB, error) {
	if log == nil {
		log = silentLogger{}
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger:  log,
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