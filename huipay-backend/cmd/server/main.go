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
	"github.com/huipay/huipay-backend/infra/notify"
	"github.com/huipay/huipay-backend/infra/idem"
	"github.com/huipay/huipay-backend/infra/obs"
	"github.com/huipay/huipay-backend/infra/prom"
	"github.com/huipay/huipay-backend/infra/migrator"

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

	storerepo "github.com/huipay/huipay-backend/internal/store/repository"
	storeservice "github.com/huipay/huipay-backend/internal/store/service"
	storehandler "github.com/huipay/huipay-backend/internal/store/handler"

	notifyhandler "github.com/huipay/huipay-backend/internal/payment/notify"
	paymentrouter "github.com/huipay/huipay-backend/internal/payment/router"
	"github.com/huipay/huipay-backend/internal/payment/channel/wechat"
	mockchannel "github.com/huipay/huipay-backend/internal/payment/channel/mock"
	reconcilesvc "github.com/huipay/huipay-backend/internal/payment/reconcile"
	reconcilesched "github.com/huipay/huipay-backend/internal/payment/reconcile/scheduler"
	"github.com/huipay/huipay-backend/internal/middleware"
	"github.com/huipay/huipay-backend/internal/domain/vo"

	splithandler "github.com/huipay/huipay-backend/internal/split/handler"
	splitservice "github.com/huipay/huipay-backend/internal/split/service"
	splitrule "github.com/huipay/huipay-backend/internal/split/rule"
	splitexec "github.com/huipay/huipay-backend/internal/split/executor"
	splitsched "github.com/huipay/huipay-backend/internal/split/scheduler"
	splitrepo "github.com/huipay/huipay-backend/internal/split/repository"
	recon "github.com/huipay/huipay-backend/internal/split/recon"

	statsrepo "github.com/huipay/huipay-backend/internal/stats/repository"
	statsservice "github.com/huipay/huipay-backend/internal/stats/service"
	statsscheduler "github.com/huipay/huipay-backend/internal/stats/scheduler"

	adminservice "github.com/huipay/huipay-backend/internal/admin/service"
	adminhandler "github.com/huipay/huipay-backend/internal/admin/handler"
)

// walletAdapter 适配 account.Service → ports.WalletResolver
type walletAdapter struct {
	svc *accountservice.Service
}

func (w *walletAdapter) GetWalletByEntityType(ctx context.Context, entityID uint64, entityType vo.EntityType) (uint64, error) {
	wallet, err := w.svc.GetWalletByEntityType(ctx, entityID, entityType)
	if err != nil {
		return 0, err
	}
	if wallet == nil {
		return 0, errs.New(errs.CodeInternalError, "wallet not found", 200)
	}
	return wallet.ID, nil
}


// toObsFileLog 将配置的日志文件配置转换为 obs 日志文件配置。
func toObsFileLog(c config.LogFileConfig) obs.FileLogConfig {
	return obs.FileLogConfig{
		Enabled:   c.Enabled,
		Path:      c.Path,
		MaxSizeMB: c.MaxSizeMB,
		MaxAgeDay: c.MaxAgeDay,
	}
}

func main() {
	// 1. 加载配置
	cfg := config.Load()

	// 2. 初始化日志
	logger := obs.NewZapLogger(cfg.LogLevel, toObsFileLog(cfg.LogFile))

	// 3. 初始化 MySQL 主从（HUIPAY_SKIP_DB=true 时跳过，便于本地冒烟）
	dbConn, err := db.MustOpen(cfg, nil)
	if err != nil {
		if os.Getenv("HUIPAY_SKIP_DB") != "true" {
			logger.Fatal(fmt.Sprintf("db open failed: %v", err))
		}
		logger.Warn("HUIPAY_SKIP_DB=true: 跳过 DB 初始化，所有依赖 DB 的接口将不可用")
		dbConn = &db.DB{}
	}

	// 4. 应用数据库迁移（迁移文件已内嵌进二进制；幂等，已应用则跳过）
	//    仅在真实连接数据库时执行；HUIPAY_SKIP_DB 冒烟模式 dbConn.Master 为 nil 跳过
	if dbConn.Master != nil {
		if err := migrator.Run(cfg.MySQLMaster, logger); err != nil {
			logger.Fatal(fmt.Sprintf("apply db migrations failed: %v", err))
		}
	}

	// 5. 初始化 Prometheus
	prom.MustRegister()

	// 6. 装配业务服务（手工注入；后续可改为 Wire）
	walletRepo := accountrepo.NewWalletRepo(dbConn.Master)
	journalRepo := accountrepo.NewJournalRepo(dbConn.Master)
	ledgerSvc := ledger.NewService(walletRepo, journalRepo, logger)
	accountSvc := accountservice.NewService(ledgerSvc, walletRepo, journalRepo, logger)

	paymentRouter := paymentrouter.NewDefaultRouter()
	paymentCodeRepo := paymentcoderepo.NewPaymentCodeRepo(dbConn.Master)
	storeRepo := storerepo.NewStoreRepo(dbConn.Master)

	// 商户进件服务（依赖实体仓储 + 账户服务；也是商户微信配置 provider，需在微信管理器前装配）
	entityRepo := merchantrepo.NewEntityRepo(dbConn.Master)
	merchantSvc := merchantservice.NewService(dbConn.Master, entityRepo, accountSvc, logger, cfg.AuthSecret)

	// 微信通道适配器（enabled 时初始化；失败仅告警，不阻断服务启动）
	var wxAdapter *wechat.Adapter
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

	// 挡板通道：本地未启用微信支付时，注册 mock 适配器模拟支付成功，便于联调。
	// 仅当微信通道未启用且 app_env 非 production 时生效。
	var (
		orderSvc          *orderservice.Service // 提前声明，供 mock 回调闭包引用，稍后赋值
		settlementWalletID uint64               // 通道在途资金户 wallet_id，供 mock 回调入账
	)
	if wxAdapter == nil && cfg.AppEnv != "production" {
		mockAdapter := mockchannel.New(func(ctx context.Context, orderNo string, amount int64) error {
			// 模拟微信回调成功：标记订单已支付（幂等安全）
			updated, err := orderSvc.MarkPaid(ctx, orderNo, amount, vo.ChannelWeChat, "mock_trade_"+orderNo)
			if err != nil {
				return err
			}
			if !updated {
				return nil
			}
			order, gErr := orderSvc.GetByOrderNo(ctx, orderNo)
			if gErr != nil || order == nil {
				return gErr
			}
			// 挡板：先给结算户预充高额资金（模拟渠道垫资），再完成 结算户→商户 入账，保证借贷平衡
			settle, wErr := walletRepo.GetByID(ctx, settlementWalletID)
			if wErr != nil {
				logger.Error("mock credit: get settlement wallet fail", zap.Uint64("settlement_wallet_id", settlementWalletID), zap.Error(wErr))
				return wErr
			}
			logger.Info("mock credit: settlement balance before", zap.Uint64("wallet_id", settle.ID), zap.Int64("balance", settle.Balance), zap.Int64("need", amount))
			if settle.Balance < amount {
				if uErr := walletRepo.UpdateBalance(ctx, settle.ID, settle.Version, amount); uErr != nil {
					logger.Error("mock credit: topup settlement fail", zap.Error(uErr))
					return uErr
				}
			}
			if cErr := ledgerSvc.CreditFromSettlement(ctx, &ledger.CreditFromSettlementRequest{
				SettlementWalletID: settlementWalletID,
				ToEntityID:         order.MerchantID,
				ToEntityType:       vo.EntityMerchant,
				Amount:             amount,
				BizType:            "PAYMENT",
				BizID:              orderNo,
			}); cErr != nil {
				logger.Error("mock credit: credit from settlement fail", zap.Error(cErr))
				return cErr
			}
			logger.Info("mock credit: success", zap.String("order_no", orderNo), zap.Int64("amount", amount))
			return nil
		})
		paymentRouter.Register(mockAdapter)
		logger.Warn("mock payment channel enabled (wechat disabled)")
	}

	// 按商户微信配置的适配器管理器：商户已配置则用商户参数下单/查单/回调，否则回退平台通道
	var wxManager *wechat.Manager
	if wxAdapter != nil {
		wxManager = wechat.NewManager(wxAdapter, merchantSvc, logger)
	}

	// 微信未启用时 wxManager 为 nil：显式传 nil 接口，避免 Go 的 nil 指针放入接口导致 merchantAdapters != nil
	var merchantAdapters orderservice.MerchantAdapterResolver
	if wxManager != nil {
		merchantAdapters = wxManager
	}
	orderSvc = orderservice.NewService(dbConn.Master, logger, paymentRouter, paymentCodeRepo, merchantAdapters)

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
	if dbConn.Master != nil {
		wid, seedErr := bootstrap.SeedChannelSettlementWallets(context.Background(), dbConn.Master, accountSvc, logger)
		if seedErr != nil {
			logger.Warn("seed channel settlement wallets fail", zap.Error(seedErr))
		} else {
			settlementWalletID = wid
		}
	}

	ruleEngine := splitrule.NewEngine()
	splitOrderStatusRepo := splitrepo.NewSplitOrderStatusRepo(dbConn.Master)
	splitAuditRepo := splitrepo.NewSplitAuditRepo(dbConn.Master)
	splitExec := splitexec.NewExecutor(walletRepo, journalRepo, splitOrderStatusRepo, paymentRouter, logger)
	// 告警器（企业微信 webhook；alert_webhook_url 空配置时为空操作）
	alerter := notify.New(cfg.AlertWebhookURL, logger)
	splitExec.SetAlerter(alerter)
	splitRuleRepo := splitrepo.NewSplitRuleRepo(dbConn.Master)
	splitBillRepo := splitrepo.NewSplitBillRepo(dbConn.Master)
	splitBillBizDateRepo := splitrepo.NewBillBizDateRepo(dbConn.Master)
	splitDailyExecRepo := splitrepo.NewDailyExecutionRepo(dbConn.Master)
	splitDiffRepo := splitrepo.NewReconcileDiffRepo(dbConn.Master)
	splitRevenueRepo := splitrepo.NewStoreRevenueRepo(dbConn.Master)

	// 门店订单日报服务（T+1 02:00 聚合）
	statsRepo := statsrepo.NewStoreDailyStatsRepo(dbConn.Master)
	statsSvc := statsservice.NewService(statsRepo, dbConn.Master, logger)

	// 分账前置对账器（依赖 statsSvc 自动补跑 + 差异落库）
	prechecker := recon.NewPrechecker(dbConn.Master, statsSvc, splitDiffRepo, splitAuditRepo, logger)
	splitSvc := splitservice.NewService(
		ruleEngine, splitExec,
		splitRuleRepo, splitBillRepo,
		splitBillBizDateRepo, splitDailyExecRepo, splitDiffRepo,
		splitAuditRepo, splitOrderStatusRepo,
				walletAdapter{accountSvc},walletAdapter{accountSvc}, splitRevenueRepo,
		prechecker,
		logger,
	)

	// 管理后台：调度监测 + 门店日报报表 + 分账管理（每日执行/审计/差异/重算重置）
	adminSchedulerSvc := adminservice.NewSchedulerService(dbConn.Master, logger)
	adminSchedulerH := adminhandler.NewSchedulerHandler(adminSchedulerSvc, logger)
	adminStoreStatsH := adminhandler.NewStoreStatsHandler(statsSvc, logger)
	adminSplitManageSvc := adminservice.NewSplitManageService(
		dbConn.Master, splitDailyExecRepo, splitAuditRepo, splitDiffRepo, statsSvc, logger,
	)
	adminSplitManageH := adminhandler.NewSplitManageHandler(adminSplitManageSvc, logger)
	adminAuthSvc := adminservice.NewAdminAuthService(cfg.AdminUsername, cfg.AdminPassword, cfg.AuthSecret, logger)
	adminAuthH := adminhandler.NewAdminAuthHandler(adminAuthSvc, logger)

	// 商户自助：门店日报统计 + 定时任务监测（只读）
	merchantStoreStatsH := merchanthandler.NewStoreStatsHandler(statsSvc, logger)
	merchantSchedulerH := merchanthandler.NewSchedulerHandler(adminSchedulerSvc, logger)

	// 门店服务
	storeSvc := storeservice.NewService(storeRepo, logger)

	// 收款码牌服务
	paymentCodeSvc := paymentcodeservice.NewService(paymentCodeRepo, storeRepo, logger, cfg.CheckoutBaseURL)

	// 6. 装配 Gin
	gin.SetMode(cfg.GinMode)
	r := gin.New()
	r.Use(obs.GinRecovery(logger))
	r.Use(obs.GinTrace())
	r.Use(obs.GinAccessLog(logger))
	r.Use(errs.GinErrorHandler(logger))
	r.Use(middleware.CORS())
	r.Use(middleware.NewMerchantAuth(cfg.AuthSecret, cfg.TrustMerchantHeader))
	r.Use(middleware.NewAdminAuth(cfg.AuthSecret))

	// 7. 注册路由
	orderH := orderhandler.New(orderSvc, logger)
	accountH := accounthandler.New(accountSvc, logger)
	splitH := splithandler.New(splitSvc, logger)
	merchantH := merchanthandler.New(merchantSvc, logger)
	paymentCodeH := paymentcodehandler.New(paymentCodeSvc, logger)
	storeH := storehandler.New(storeSvc, logger)
	notifyH := notifyhandler.New(wxAdapter, wxManager, orderSvc, accountSvc, ledgerSvc, idemStore, settlementWalletID, logger)

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
		v1.GET("/checkout/stats", orderH.Stats)
		v1.POST("/checkout/embed-info", orderH.EmbedInfo)
		v1.GET("/checkout/code/:code_id", orderH.GetCode)
		v1.GET("/checkout/:order_no", orderH.Get)
		v1.GET("/checkout/:order_no/query", orderH.Query)
		v1.POST("/checkout/:order_no/refund", orderH.Refund)
		v1.POST("/checkout/:order_no/pay", orderH.Pay)

		// 商户登录（Bearer token 鉴权）
		v1.POST("/auth/merchant/login", merchantH.Login)

		v1.POST("/notify/wechat", notifyH.HandleWechat)
		v1.POST("/notify/wechat/:merchant_id", notifyH.HandleWechat)
		v1.POST("/notify/wechat/refund", notifyH.HandleWechatRefund)

		if oauthH != nil {
			v1.GET("/oauth/wechat/authorize", oauthH.Authorize)
			v1.GET("/oauth/wechat/callback", oauthH.Callback)
		}

		v1.GET("/wallets/:entity_id", accountH.GetWallet)
		v1.GET("/wallets/:entity_id/entries", accountH.ListEntries)

		v1.POST("/split/execute", splitH.Execute)
		v1.GET("/split/:order_no", splitH.Get)
		v1.POST("/merchant/split/execute-period", splitH.ExecuteByPeriod)
		v1.POST("/merchant/split/preview", splitH.Preview)
		v1.GET("/merchant/split/bills", splitH.ListBills)
		v1.POST("/merchant/split/bills", splitH.GenerateBill)
		v1.GET("/merchant/split/bills/:batch_no", splitH.GetBill)
		v1.GET("/merchant/split/bills/:batch_no/stores", splitH.BillStores)
		v1.GET("/merchant/split/bills/:batch_no/stores/:store_id/orders", splitH.BillStoreOrders)
		v1.POST("/merchant/split/bills/:batch_no/approve", splitH.ApproveBill)
		v1.POST("/merchant/split/bills/:batch_no/reject", splitH.RejectBill)
		v1.GET("/merchant/split/executions", splitH.ListExecutions)
		v1.GET("/merchant/split/executions/:order_no", splitH.GetExecutionDetail)
		v1.POST("/merchant/split/executions/:order_no/retry", splitH.RetryExecution)
		v1.POST("/merchant/split/executions/:order_no/reopen", splitH.ReopenExecution)
		// 差错中心：异常订单聚合 / 审计查询 / 对账差异与核销
		v1.GET("/merchant/split/exceptions", splitH.ListExceptions)
		v1.GET("/merchant/split/audit", splitH.ListAudits)
		v1.GET("/merchant/split/reconcile-diffs", splitH.ListReconcileDiffs)
		v1.POST("/merchant/split/reconcile-diffs/:id/resolve", splitH.ResolveReconcileDiff)

		// 管理后台登录（无需鉴权）
		v1.POST("/admin/login", adminAuthH.Login)

		// 管理后台：商户进件、列表、详情、更新、状态、概览
		v1.POST("/admin/merchants", merchantH.Onboard)
		v1.POST("/admin/merchants/:id/login-password", merchantH.SetLoginPassword)
		v1.GET("/admin/merchants", merchantH.List)
		v1.GET("/admin/merchants/:id", merchantH.Get)
		v1.PUT("/admin/merchants/:id", merchantH.Update)
		v1.POST("/admin/merchants/:id/status", merchantH.SetStatus)
		v1.GET("/admin/merchants/:id/overview", merchantH.Overview)
		v1.GET("/admin/merchants/:id/wechat-config", merchantH.GetWechatConfig)
		v1.PUT("/admin/merchants/:id/wechat-config", merchantH.UpdateWechatConfig)

		// 管理后台：门店日报报表（admin 路由暂未接入鉴权，TODO 下轮统一）
		v1.GET("/admin/store-stats", adminStoreStatsH.ListStoreStats)
		v1.GET("/admin/store-stats/summary", adminStoreStatsH.StoreStatsSummary)
		v1.POST("/admin/store-stats/backfill", adminStoreStatsH.Backfill)
		v1.GET("/admin/stores/:id/daily-stats", adminStoreStatsH.GetStoreDailyStats)

		// 管理后台：定时任务监测（admin 路由暂未接入鉴权，TODO 下轮统一）
		v1.GET("/admin/scheduler/tasks", adminSchedulerH.ListTasks)
		v1.GET("/admin/scheduler/runs", adminSchedulerH.ListRuns)
		v1.GET("/admin/scheduler/runs/:id", adminSchedulerH.GetRun)
		v1.POST("/admin/scheduler/tasks/:name/run", adminSchedulerH.TriggerTask)

		// 管理后台：分账管理（每日执行/审计/对账差异/重算重置）
		v1.GET("/admin/split/daily-executions", adminSplitManageH.ListDailyExecutions)
		v1.GET("/admin/split/daily-executions/:id", adminSplitManageH.GetDailyExecution)
		v1.GET("/admin/split/audit", adminSplitManageH.ListAudits)
		v1.GET("/admin/reconcile-diffs", adminSplitManageH.ListDiffs)
		v1.POST("/admin/split/executions/:order_no/resolve", adminSplitManageH.ResolveExecution)
		v1.POST("/admin/split/executions/:order_no/reopen", adminSplitManageH.ReopenExecution)
		v1.GET("/admin/split/exceptions", adminSplitManageH.ListExceptions)
		v1.POST("/admin/reconcile-diffs/:id/resolve", adminSplitManageH.ResolveReconcileDiff)
		v1.POST("/admin/store-stats/recompute", adminSplitManageH.RecomputeStoreStats)
		v1.POST("/admin/store-stats/reset-split-status", adminSplitManageH.ResetStoreSplitStatus)

		// 收款码牌：商户侧自助管理
		v1.POST("/merchant/codes", paymentCodeH.Create)
		v1.GET("/merchant/codes", paymentCodeH.List)
		v1.POST("/merchant/codes/:id/disable", paymentCodeH.Disable)
		v1.POST("/merchant/codes/:id/store", paymentCodeH.SetStore)

		// 门店：商户侧自助管理
		v1.POST("/merchant/stores", storeH.Create)
		v1.GET("/merchant/stores", storeH.List)
		v1.GET("/merchant/stores/stats", storeH.Stats)
		v1.GET("/merchant/stores/:id", storeH.Get)
		v1.PUT("/merchant/stores/:id", storeH.Update)
		v1.POST("/merchant/stores/:id/status", storeH.SetStatus)
		v1.DELETE("/merchant/stores/:id", storeH.Delete)

		// 分账规则：商户侧自助管理
		v1.GET("/merchant/split/rules", splitH.ListRules)
		v1.POST("/merchant/split/rules", splitH.CreateRule)
		v1.PUT("/merchant/split/rules/:id", splitH.UpdateRule)
		v1.POST("/merchant/split/rules/:id/status", splitH.SetRuleStatus)
		v1.DELETE("/merchant/split/rules/:id", splitH.DeleteRule)

		// 商户自助：当前商户资料与经营概览（读 X-Merchant-Id 中间件）
		v1.GET("/merchant/profile", merchantH.SelfProfile)
		v1.GET("/merchant/overview", merchantH.SelfOverview)

		// 商户自助：门店日报统计（按当前商户过滤）
		v1.GET("/merchant/store-stats", merchantStoreStatsH.ListStats)
		v1.GET("/merchant/store-stats/summary", merchantStoreStatsH.Summary)
		v1.GET("/merchant/store-stats/stores/:id/daily", merchantStoreStatsH.GetDailyStats)

		// 商户自助：定时任务监测（任务列表 + 运行日志 + 手动执行）
		v1.GET("/merchant/scheduler/tasks", merchantSchedulerH.ListTasks)
		v1.GET("/merchant/scheduler/runs", merchantSchedulerH.ListRuns)
		v1.GET("/merchant/scheduler/runs/:id", merchantSchedulerH.GetRun)
		v1.POST("/merchant/scheduler/tasks/:name/run", merchantSchedulerH.TriggerTask)
	}

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/metrics", prom.Handler())

	// 8. 启动定时任务（超时关单 + 幂等键清理 + 每日对账 + 分账补偿 + 门店订单日报 + 分账日对账）
	if dbConn.Master != nil {
		go orderscheduler.NewCloseExpiredScheduler(dbConn.Master, paymentRouter, wxManager, 30*time.Second, logger).Start(context.Background())
		go orderscheduler.StartIdempotencyCleanup(context.Background(), dbConn.Master, 1*time.Hour, logger)
		if billDownloader != nil {
			go reconcilesched.StartDailyReconcile(context.Background(), billDownloader, dbConn.Master, logger)
		}
		// 分账补偿调度：悬挂检测 + 失败/部分失败订单自动重入（30s 轮询）
		compSched := splitsched.NewCompensateScheduler(splitOrderStatusRepo, splitExec, accountSvc, logger)
		compSched.SetAlerter(alerter)
		go compSched.Start(context.Background(), 30*time.Second)
		// split_status 异步汇总调度：扫 t_split_execution 变更增量回算门店×日分账状态（10 分钟轮询）
		go splitsched.NewRecomputeScheduler(dbConn.Master, statsSvc, logger).Start(context.Background())
		// 门店订单日报：T+1 02:00 聚合 T 日订单（接入监测框架）
		statsHandle := statsscheduler.NewStoreDailyStatsScheduler(dbConn.Master, statsSvc, logger)
		go statsHandle.Start(context.Background(), statsscheduler.Runnable(statsSvc), statsscheduler.Options())
		// 分账日对账：T+1 02:30 比对本地账本与执行记录，差异落库 + 告警（接入监测框架，支持手动触发）
		reconcileHandle := splitsched.NewSplitReconcileScheduler(dbConn.Master, splitDiffRepo, alerter, logger)
		go reconcileHandle.Start(context.Background(),
			splitsched.ReconcileRunnable(dbConn.Master, splitDiffRepo, alerter, logger),
			splitsched.ReconcileOptions())
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
