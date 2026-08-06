// 包 main 是 HuiPay 后端服务的启动入口。
// 装配顺序：配置 → 日志 → DB → 业务服务 → HTTP 路由。
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/huipay/huipay-backend/infra/config"
	"github.com/huipay/huipay-backend/infra/db"
	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/infra/obs"
	"github.com/huipay/huipay-backend/infra/prom"

	orderhandler "github.com/huipay/huipay-backend/internal/order/handler"
	orderservice "github.com/huipay/huipay-backend/internal/order/service"

	accounthandler "github.com/huipay/huipay-backend/internal/account/handler"
	accountrepo "github.com/huipay/huipay-backend/internal/account/repository"
	accountservice "github.com/huipay/huipay-backend/internal/account/service"
	"github.com/huipay/huipay-backend/internal/account/ledger"

	paymentrouter "github.com/huipay/huipay-backend/internal/payment/router"

	splithandler "github.com/huipay/huipay-backend/internal/split/handler"
	splitservice "github.com/huipay/huipay-backend/internal/split/service"
	splitrule "github.com/huipay/huipay-backend/internal/split/rule"
	splitexec "github.com/huipay/huipay-backend/internal/split/executor"
)

func main() {
	// 1. 加载配置
	cfg := config.Load()

	// 2. 初始化日志
	logger := obs.NewZapLogger(cfg.LogLevel)

	// 3. 初始化 MySQL 主从（HUIPAY_SKIP_DB=true 时跳过，便于本地冒烟）
	dbConn, err := db.MustOpen(cfg, nil)
	if err != nil {
		if os.Getenv("HUIPAY_SKIP_DB") != "true" {
			logger.Fatal(fmt.Sprintf("db open failed: %v", err))
		}
		logger.Warn("HUIPAY_SKIP_DB=true: 跳过 DB 初始化，所有依赖 DB 的接口将不可用")
		dbConn = &db.DB{}
	}

	// 4. 初始化 Prometheus
	prom.MustRegister()

	// 5. 装配业务服务（手工注入；后续可改为 Wire）
	walletRepo := accountrepo.NewWalletRepo(dbConn.Master)
	journalRepo := accountrepo.NewJournalRepo(dbConn.Master)
	ledgerSvc := ledger.NewService(walletRepo, journalRepo, logger)
	accountSvc := accountservice.NewService(ledgerSvc, walletRepo, journalRepo, logger)

	paymentRouter := paymentrouter.NewDefaultRouter()
	orderSvc := orderservice.NewService(dbConn.Master, logger, paymentRouter)

	ruleEngine := splitrule.NewEngine()
	splitExec := splitexec.NewExecutor(walletRepo, journalRepo, logger)
	splitSvc := splitservice.NewService(ruleEngine, splitExec, accountSvc, logger)

	// 6. 装配 Gin
	gin.SetMode(cfg.GinMode)
	r := gin.New()
	r.Use(obs.GinRecovery(logger))
	r.Use(obs.GinTrace())
	r.Use(obs.GinAccessLog(logger))
	r.Use(errs.GinErrorHandler(logger))

	// 7. 注册路由
	orderH := orderhandler.New(orderSvc, logger)
	accountH := accounthandler.New(accountSvc, logger)
	splitH := splithandler.New(splitSvc, logger)

	v1 := r.Group("/v1")
	{
		v1.POST("/checkout/precreate", orderH.Precreate)
		v1.GET("/checkout/:order_no", orderH.Get)
		v1.POST("/checkout/:order_no/refund", orderH.Refund)

		v1.GET("/wallets/:entity_id", accountH.GetWallet)
		v1.GET("/wallets/:entity_id/entries", accountH.ListEntries)

		v1.POST("/split/execute", splitH.Execute)
		v1.GET("/split/:order_no", splitH.Get)
	}

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/metrics", prom.Handler())

	// 8. 启动服务并优雅退出
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	go func() {
		logger.Info(fmt.Sprintf("huipay-backend listening on %s", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal(fmt.Sprintf("listen failed: %v", err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("server forced shutdown: %v", err)
	}
}