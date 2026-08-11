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
	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/infra/config"
	"github.com/huipay/huipay-backend/infra/db"
	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/infra/idem"
	"github.com/huipay/huipay-backend/infra/obs"
	"github.com/huipay/huipay-backend/infra/prom"

	orderhandler "github.com/huipay/huipay-backend/internal/order/handler"
	orderservice "github.com/huipay/huipay-backend/internal/order/service"
	orderscheduler "github.com/huipay/huipay-backend/internal/order/scheduler"

	oauthhandler "github.com/huipay/huipay-backend/internal/payment/oauth"

	accounthandler "github.com/huipay/huipay-backend/internal/account/handler"
	accountrepo "github.com/huipay/huipay-backend/internal/account/repository"
	accountservice "github.com/huipay/huipay-backend/internal/account/service"
	"github.com/huipay/huipay-backend/internal/account/bootstrap"
	"github.com/huipay/huipay-backend/internal/account/ledger"

	merchantrepo "github.com/huipay/huipay-backend/internal/merchant/repository"
	merchantservice "github.com/huipay/huipay-backend/internal/merchant/service"
	merchanthandler "github.com/huipay/huipay-backend/internal/merchant/handler"

	paymentcoderepo "github.com/huipay/huipay-backend/internal/paymentcode/repository"
	paymentcodeservice "github.com/huipay/huipay-backend/internal/paymentcode/service"
	paymentcodehandler "github.com/huipay/huipay-backend/internal/paymentcode/handler"

	"github.com/huipay/huipay-backend/internal/payment/channel"
	notifyhandler "github.com/huipay/huipay-backend/internal/payment/notify"
	paymentrouter "github.com/huipay/huipay-backend/internal/payment/router"
	"github.com/huipay/huipay-backend/internal/payment/channel/wechat"
	reconcilesvc "github.com/huipay/huipay-backend/internal/payment/reconcile"
	reconcilesched "github.com/huipay/huipay-backend/internal/payment/reconcile/scheduler"
	"github.com/huipay/huipay-backend/internal/middleware"

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
	paymentCodeRepo := paymentcoderepo.NewPaymentCodeRepo(dbConn.Master)
	orderSvc := orderservice.NewService(dbConn.Master, logger, paymentRouter, paymentCodeRepo)

	// 微信通道适配器（enabled 时初始化；失败仅告警，不阻断服务启动）
	var wxAdapter channel.Adapter
	if cfg.WeChat.Enabled {
		wx, err := wechat.New(cfg.WeChat)
		if err != nil {
			logger.Error("wechat channel init fail", zap.Error(err))
		} else {
			wxAdapter = wx
		}
	}
	if wxAdapter != nil {
		paymentRouter.Register(wxAdapter)
	}

	// 对账下载器（微信通道启用且配置完整时初始化；失败仅告警）
	var billDownloader *reconcilesvc.Downloader
	if cfg.WeChat.Enabled {
		bd, bdErr := reconcilesvc.NewDownloader(cfg.WeChat)
		if bdErr != nil {
			logger.Error("reconcile downloader init fail", zap.Error(bdErr))
		} else {
			billDownloader = bd
		}
	}

	idemStore := idem.NewMySQLStore(dbConn.Master)

	// 启动时初始化通道在途资金户（entity + wallet），返回微信结算户 wallet_id 供回调入账
	settlementWalletID := uint64(0)
	if dbConn.Master != nil {
		wid, seedErr := bootstrap.SeedChannelSettlementWallets(context.Background(), dbConn.Master, accountSvc, logger)
		if seedErr != nil {
			logger.Warn("seed channel settlement wallets fail", zap.Error(seedErr))
		} else {
			settlementWalletID = wid
		}
	}

	ruleEngine := splitrule.NewEngine()
	splitExec := splitexec.NewExecutor(walletRepo, journalRepo, logger)
	splitSvc := splitservice.NewService(ruleEngine, splitExec, accountSvc, logger)

	// 商户进件服务（依赖 entity 仓储 + 账户服务）
	entityRepo := merchantrepo.NewEntityRepo(dbConn.Master)
	merchantSvc := merchantservice.NewService(dbConn.Master, entityRepo, accountSvc, logger)

	// 收款码牌服务
	paymentCodeSvc := paymentcodeservice.NewService(paymentCodeRepo, logger)

	// 6. 装配 Gin
	gin.SetMode(cfg.GinMode)
	r := gin.New()
	r.Use(obs.GinRecovery(logger))
	r.Use(obs.GinTrace())
	r.Use(obs.GinAccessLog(logger))
	r.Use(errs.GinErrorHandler(logger))
	r.Use(middleware.CORS())
	r.Use(middleware.MerchantID())

	// 7. 注册路由
	orderH := orderhandler.New(orderSvc, logger)
	accountH := accounthandler.New(accountSvc, logger)
	splitH := splithandler.New(splitSvc, logger)
	merchantH := merchanthandler.New(merchantSvc, logger)
	paymentCodeH := paymentcodehandler.New(paymentCodeSvc, logger)
	notifyH := notifyhandler.New(wxAdapter, orderSvc, accountSvc, ledgerSvc, idemStore, settlementWalletID, logger)

	// 微信 OAuth（公众号网页授权，用于 JSAPI 场景获取 openid）
	var oauthH *oauthhandler.Handler
	if cfg.WeChat.Enabled && cfg.WeChat.AppID != "" && cfg.WeChat.AppSecret != "" {
		oauthH = oauthhandler.New(wechat.NewOAuthClient(cfg.WeChat), cfg.WeChat.NotifyBaseURL, logger)
	}

	v1 := r.Group("/v1")
	{
		v1.POST("/checkout/precreate", orderH.Precreate)
		v1.POST("/checkout/precreate-by-code", orderH.PrecreateByCode)
		v1.GET("/checkout/list", orderH.List)
		v1.POST("/checkout/embed-info", orderH.EmbedInfo)
		v1.GET("/checkout/:order_no", orderH.Get)
		v1.GET("/checkout/:order_no/query", orderH.Query)
		v1.POST("/checkout/:order_no/refund", orderH.Refund)
		v1.POST("/checkout/:order_no/pay", orderH.Pay)

		v1.POST("/notify/wechat", notifyH.HandleWechat)
		v1.POST("/notify/wechat/refund", notifyH.HandleWechatRefund)

		if oauthH != nil {
			v1.GET("/oauth/wechat/authorize", oauthH.Authorize)
			v1.GET("/oauth/wechat/callback", oauthH.Callback)
		}

		v1.GET("/wallets/:entity_id", accountH.GetWallet)
		v1.GET("/wallets/:entity_id/entries", accountH.ListEntries)

		v1.POST("/split/execute", splitH.Execute)
		v1.GET("/split/:order_no", splitH.Get)

		// 管理后台：商户进件、列表、详情、更新、状态、概览
		v1.POST("/admin/merchants", merchantH.Onboard)
		v1.GET("/admin/merchants", merchantH.List)
		v1.GET("/admin/merchants/:id", merchantH.Get)
		v1.PUT("/admin/merchants/:id", merchantH.Update)
		v1.POST("/admin/merchants/:id/status", merchantH.SetStatus)
		v1.GET("/admin/merchants/:id/overview", merchantH.Overview)
		v1.GET("/admin/merchants/:id/wechat-config", merchantH.GetWechatConfig)
		v1.PUT("/admin/merchants/:id/wechat-config", merchantH.UpdateWechatConfig)

		// 收款码牌：商户侧自助管理
		v1.POST("/merchant/codes", paymentCodeH.Create)
		v1.GET("/merchant/codes", paymentCodeH.List)
		v1.POST("/merchant/codes/:id/disable", paymentCodeH.Disable)

		// 商户自助：当前商户资料与经营概览（读 X-Merchant-Id 中间件）
		v1.GET("/merchant/profile", merchantH.SelfProfile)
		v1.GET("/merchant/overview", merchantH.SelfOverview)
	}

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/metrics", prom.Handler())

	// 8. 启动定时任务（超时关单 + 幂等键清理 + 每日对账）
	if dbConn.Master != nil {
		go orderscheduler.NewCloseExpiredScheduler(dbConn.Master, paymentRouter, 30*time.Second, logger).Start(context.Background())
		go orderscheduler.StartIdempotencyCleanup(context.Background(), dbConn.Master, 1*time.Hour, logger)
		if billDownloader != nil {
			go reconcilesched.StartDailyReconcile(context.Background(), billDownloader, dbConn.Master, logger)
		}
	}

	// 9. 启动服务并优雅退出
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