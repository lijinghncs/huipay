// 包 reconcile 提供微信交易对账单的下载与解析；比对与差异落库由 recon 域承担。
package reconcile

import (
	"context"
	"fmt"

	"github.com/huipay/huipay-backend/infra/config"
	"github.com/huipay/huipay-backend/internal/payment/channel/wechat"
)

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
