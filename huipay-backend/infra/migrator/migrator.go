// 包 migrator 将数据库迁移内置到应用中，启动时自动执行。
//
// 迁移文件通过 embed 打包进二进制，随版本分发，解决服务器部署时
// "忘了跑迁移" 或 "代码有列、库无列" 的漂移问题。
//
// 复用 golang-migrate 的 schema_migrations 表追踪版本，等价于 CLI 的
// `migrate -path migrations up`，但无需外部安装 migrate 工具。
package migrator

import (
	"embed"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/mysql" // 注册 mysql 驱动
	"github.com/golang-migrate/migrate/v4/source/iofs"     // 注册 embed FS 源
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Run 对给定 MySQL DSN 执行全部未应用的迁移（migrate up）。
// 幂等：已应用则返回 nil；迁移失败返回错误（应用可据此决定是否回滚/退出）。
func Run(dsn string, logger *zap.Logger) error {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("migrate source init: %w", err)
	}

	m, err := migrate.NewWithSourceInstance(
		"iofs", src,
		"mysql://"+dsn,
	)
	if err != nil {
		return fmt.Errorf("migrate init: %w", err)
	}
	defer m.Close()

	logger.Info("running db migrations", zap.String("dsn_masked", maskDSN(dsn)))

	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			logger.Info("db migrations: no change")
			return nil
		}
		// 打印当前版本与脏标记，便于排查
		if v, dirty, verr := m.Version(); verr == nil {
			logger.Error("db migrations failed",
				zap.Uint("version", v), zap.Bool("dirty", dirty), zap.Error(err))
		}
		return fmt.Errorf("migrate up: %w", err)
	}
	logger.Info("db migrations applied successfully")
	return nil
}

// maskDSN 隐藏密码，避免日志泄露敏感信息。
func maskDSN(dsn string) string {
	// 取 @ 之前作为 user 部分，密码可能含 @，简单处理：仅保留 host 与库名
	// 这里只用于日志，不参与实际连接。
	end := len(dsn)
	if i := indexOf(dsn, "?charset"); i > 0 {
		end = i
	}
	return "***@" + tailHost(dsn[:end])
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// tailHost 提取 @ 之后的部分（host:port/db）。
func tailHost(s string) string {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '@' {
			return s[i+1:]
		}
	}
	return s
}