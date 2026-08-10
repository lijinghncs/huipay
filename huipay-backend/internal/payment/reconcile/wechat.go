// 包 reconcile 实现 T+1 对账：下载微信交易对账单，与本地订单比对，差异落库。
package reconcile

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/infra/config"
	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/order/model"
	"github.com/huipay/huipay-backend/internal/payment/channel/wechat"
)

// 差异类型。
const (
	DiffLong     = "LONG"     // 本地有 / 微信无（长款）
	DiffShort    = "SHORT"    // 微信有 / 本地无（短款）
	DiffMismatch = "MISMATCH" // 金额不一致
)

// DiffEntry 对账差异条目。
type DiffEntry struct {
	OrderNo       string
	TransactionID string
	LocalAmount   int64
	ChannelAmount int64
	Detail        string
}

// ReconcileReport 对账结果报告。
type ReconcileReport struct {
	BizDate        string
	LongOrders     []DiffEntry
	ShortOrders    []DiffEntry
	MismatchOrders []DiffEntry
}

// Downloader 对账下载器，持有微信 V3 客户端用于申请并下载对账单。
type Downloader struct {
	cli *wechat.Client
}

// NewDownloader 构造对账下载器。
func NewDownloader(cfg config.WeChatConfig) (*Downloader, error) {
	cli, err := wechat.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return &Downloader{cli: cli}, nil
}

// DownloadBill 下载某日（yyyy-MM-dd）的微信交易对账单并解析为条目。
func (d *Downloader) DownloadBill(ctx context.Context, date string) ([]BillEntry, error) {
	resp, err := d.cli.TradeBill(ctx, date)
	if err != nil {
		return nil, fmt.Errorf("reconcile: request trade bill fail: %w", err)
	}
	rc, err := d.cli.DownloadFile(ctx, resp.DownloadURL)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	entries, err := parseBill(rc)
	if err != nil {
		return nil, fmt.Errorf("reconcile: parse trade bill fail: %w", err)
	}
	return entries, nil
}

// Reconcile 对账：拉取本地订单（paid_at 落在日期区间）与微信账单比对，产出差异报告。
// 关联键优先 transaction_id，其次 out_trade_no。
func Reconcile(ctx context.Context, d *Downloader, db *gorm.DB, date string) (*ReconcileReport, error) {
	billEntries, err := d.DownloadBill(ctx, date)
	if err != nil {
		return nil, err
	}

	start, perr := time.ParseInLocation("2006-01-02", date, time.Local)
	if perr != nil {
		return nil, perr
	}
	end := start.AddDate(0, 0, 1)

	var orders []model.OrderModel
	if err := db.WithContext(ctx).
		Where("status = ? AND paid_at >= ? AND paid_at < ?", string(vo.OrderPaid), start, end).
		Find(&orders).Error; err != nil {
		return nil, fmt.Errorf("reconcile: query local orders fail: %w", err)
	}

	// 本地索引
	localByTxn := map[string]*model.OrderModel{}
	localByOrderNo := map[string]*model.OrderModel{}
	for i := range orders {
		o := &orders[i]
		if o.ChannelTradeNo != "" {
			localByTxn[o.ChannelTradeNo] = o
		}
		localByOrderNo[o.OrderNo] = o
	}

	// 微信账单索引
	billByTxn := map[string]BillEntry{}
	billByOrderNo := map[string]BillEntry{}
	for _, b := range billEntries {
		if b.TransactionID != "" {
			billByTxn[b.TransactionID] = b
		}
		billByOrderNo[b.OutTradeNo] = b
	}

	report := &ReconcileReport{BizDate: date}

	// 本地有 / 微信无 → LONG；金额不一致 → MISMATCH
	for i := range orders {
		o := &orders[i]
		b, ok := billByTxn[o.ChannelTradeNo]
		if !ok {
			b, ok = billByOrderNo[o.OrderNo]
		}
		if !ok {
			report.LongOrders = append(report.LongOrders, DiffEntry{
				OrderNo: o.OrderNo, TransactionID: o.ChannelTradeNo,
				LocalAmount: o.PaidAmount, Detail: "local only",
			})
			continue
		}
		if b.OrderAmount != o.PaidAmount {
			report.MismatchOrders = append(report.MismatchOrders, DiffEntry{
				OrderNo: o.OrderNo, TransactionID: o.ChannelTradeNo,
				LocalAmount: o.PaidAmount, ChannelAmount: b.OrderAmount,
				Detail: "amount mismatch",
			})
		}
	}

	// 微信有 / 本地无 → SHORT
	matched := map[string]bool{}
	for i := range orders {
		o := &orders[i]
		matched[o.ChannelTradeNo] = true
		matched[o.OrderNo] = true
	}
	for _, b := range billEntries {
		if matched[b.TransactionID] || matched[b.OutTradeNo] {
			continue
		}
		report.ShortOrders = append(report.ShortOrders, DiffEntry{
			OrderNo: b.OutTradeNo, TransactionID: b.TransactionID,
			ChannelAmount: b.OrderAmount, Detail: "channel only",
		})
	}

	return report, nil
}

// LogSummary 记录对账统计日志。
func LogSummary(logger *zap.Logger, report *ReconcileReport) {
	logger.Info("reconcile summary",
		zap.String("biz_date", report.BizDate),
		zap.Int("long", len(report.LongOrders)),
		zap.Int("short", len(report.ShortOrders)),
		zap.Int("mismatch", len(report.MismatchOrders)),
	)
}