package adapter

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/internal/recon/domain"
)

// OrderFetcher 订单侧取数适配器（t_order 只读投影）。
type OrderFetcher struct{ db *gorm.DB }

func NewOrderFetcher(db *gorm.DB) *OrderFetcher { return &OrderFetcher{db: db} }

// SumForSplit 分账口径商户级总额（Layer A 订单侧）。
func (f *OrderFetcher) SumForSplit(ctx context.Context, merchantID uint64, start, end time.Time) (int64, error) {
	var total int64
	if err := f.db.WithContext(ctx).Raw(LayerAQuery, merchantID, start, end).Scan(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

// SumByStoreAndDate 分账口径门店 × 日期聚合（Layer B 订单侧），key 为 "biz_date|store_id"。
func (f *OrderFetcher) SumByStoreAndDate(ctx context.Context, merchantID uint64, start, end time.Time) (map[string]int64, error) {
	var rows []struct {
		BizDate string `gorm:"column:biz_date"`
		StoreID uint64 `gorm:"column:store_id"`
		Total   int64  `gorm:"column:total"`
	}
	if err := f.db.WithContext(ctx).Raw(LayerBQuery, merchantID, start, end).Scan(&rows).Error; err != nil {
		return nil, err
	}
	m := make(map[string]int64, len(rows))
	for _, r := range rows {
		m[fmt.Sprintf("%s|%d", r.BizDate, r.StoreID)] = r.Total
	}
	return m, nil
}

// localOrderRow 渠道对账本地订单行。
type localOrderRow struct {
	OrderNo        string `gorm:"column:order_no"`
	ChannelTradeNo string `gorm:"column:channel_trade_no"`
	MerchantID     uint64 `gorm:"column:merchant_id"`
	PaidAmount     int64  `gorm:"column:paid_amount"`
}

// ListPaidForChannel 渠道对账口径：区间内已支付订单（含商户归属，供 SHORT 回填之外的 LONG/MISMATCH 归属）。
func (f *OrderFetcher) ListPaidForChannel(ctx context.Context, start, end time.Time) ([]domain.LocalOrder, error) {
	var rows []localOrderRow
	err := f.db.WithContext(ctx).
		Raw(`SELECT order_no, channel_trade_no, merchant_id, paid_amount
			FROM t_order
			WHERE status = 'PAID' AND paid_at >= ? AND paid_at < ?`, start, end).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.LocalOrder, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.LocalOrder{
			OrderNo:        r.OrderNo,
			ChannelTradeNo: r.ChannelTradeNo,
			MerchantID:     r.MerchantID,
			PaidAmount:     r.PaidAmount,
		})
	}
	return out, nil
}

// MerchantsByOrderNos 按商户订单号回填商户归属（渠道对账 SHORT 单：本地订单缺失或非 PAID 时尝试关联）。
func (f *OrderFetcher) MerchantsByOrderNos(ctx context.Context, orderNos []string) (map[string]uint64, error) {
	out := make(map[string]uint64, len(orderNos))
	if len(orderNos) == 0 {
		return out, nil
	}
	var rows []struct {
		OrderNo    string `gorm:"column:order_no"`
		MerchantID uint64 `gorm:"column:merchant_id"`
	}
	err := f.db.WithContext(ctx).
		Raw(`SELECT order_no, merchant_id FROM t_order WHERE order_no IN (?)`, orderNos).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.OrderNo] = r.MerchantID
	}
	return out, nil
}
