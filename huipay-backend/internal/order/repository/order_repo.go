// 包 repository 提供订单数据访问。
package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/internal/order/model"
)

// OrderRepo 订单仓储。
type OrderRepo struct{ db *gorm.DB }

// NewOrderRepo 构造 OrderRepo。
func NewOrderRepo(db *gorm.DB) *OrderRepo { return &OrderRepo{db: db} }

// Create 创建订单（依赖 t_order 表的 UNIQUE KEY uk_merchant_order 兜底幂等）。
func (r *OrderRepo) Create(ctx context.Context, m *model.OrderModel) error {
	return r.db.WithContext(ctx).Create(m).Error
}

// GetByOrderNo 按平台订单号查询。
func (r *OrderRepo) GetByOrderNo(ctx context.Context, no string) (*model.OrderModel, error) {
	var m model.OrderModel
	if err := r.db.WithContext(ctx).Where("order_no = ?", no).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// GetByMerchantOrder 按商户号 + 商户单号查询（幂等键）。
func (r *OrderRepo) GetByMerchantOrder(ctx context.Context, merchantID uint64, mOrderNo string) (*model.OrderModel, error) {
	var m model.OrderModel
	if err := r.db.WithContext(ctx).
		Where("merchant_id = ? AND merchant_order_no = ?", merchantID, mOrderNo).
		First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// UpdateStatus 更新订单状态（乐观锁）。
func (r *OrderRepo) UpdateStatus(ctx context.Context, orderNo, status, splitStatus string) error {
	return r.db.WithContext(ctx).
		Model(&model.OrderModel{}).
		Where("order_no = ?", orderNo).
		Updates(map[string]any{"status": status, "split_status": splitStatus}).Error
}

// OrderListFilter 订单列表过滤条件。
type OrderListFilter struct {
	MerchantID uint64
	Status     string     // 空 = 全部状态
	CodeID     string     // 来源收款码短码（可选）
	Channel    string     // 支付通道（可选）
	StoreID    *uint64    // 来源门店（可选）
	Start      *time.Time // 创建时间起（可选）
	End        *time.Time // 创建时间止（可选）
	Page       int
	Size       int
}

// applyOrderFilter 复用状态/码牌/通道/门店/时间过滤条件，供列表与统计共用，保证口径一致。
func applyOrderFilter(q *gorm.DB, f OrderListFilter) *gorm.DB {
	q = q.Where("t_order.merchant_id = ?", f.MerchantID)
	if f.Status != "" {
		q = q.Where("t_order.status = ?", f.Status)
	}
	if f.CodeID != "" {
		q = q.Where("t_order.code_id = ?", f.CodeID)
	}
	if f.Channel != "" {
		q = q.Where("t_order.channel = ?", f.Channel)
	}
	if f.StoreID != nil {
		q = q.Where("t_order.store_id = ?", *f.StoreID)
	}
	if f.Start != nil {
		q = q.Where("t_order.created_at >= ?", *f.Start)
	}
	if f.End != nil {
		q = q.Where("t_order.created_at <= ?", *f.End)
	}
	return q
}

// ListByMerchant 按商户号分页查询订单（created_at DESC），支持状态/码牌/通道/门店/时间过滤。
func (r *OrderRepo) ListByMerchant(ctx context.Context, f OrderListFilter) ([]model.OrderModel, int64, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Size <= 0 || f.Size > 100 {
		f.Size = 20
	}

	q := applyOrderFilter(r.db.WithContext(ctx).Model(&model.OrderModel{}), f)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []model.OrderModel
	err := q.Select("t_order.*, t_store.name AS store_name").
		Joins("LEFT JOIN t_store ON t_store.id = t_order.store_id").
		Order("t_order.created_at DESC").
		Offset((f.Page - 1) * f.Size).Limit(f.Size).
		Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// OrderStatRow 订单统计单行。
type OrderStatRow struct {
	Key        string `json:"key"`         // 分组键（status/channel 原文；门店为 store_id 字符串）
	Label      string `json:"label"`       // 展示名（status/channel 原文；门店为名称）
	OrderCount int64  `json:"order_count"` // 订单数
	Amount     int64  `json:"amount"`      // 订单金额合计（分）
	Paid       int64  `json:"paid"`        // 实付金额合计（分）
}

// OrderStats 订单统计聚合结果（筛选范围内全部订单，非仅当前页）。
type OrderStats struct {
	OrderCount     int64          `json:"order_count"`      // 订单总数
	PaidOrderCount int64          `json:"paid_order_count"` // 已支付订单数
	TotalAmount    int64          `json:"total_amount"`     // 订单金额合计
	TotalPaid      int64          `json:"total_paid"`       // 实付金额合计
	ByStatus       []OrderStatRow `json:"by_status"`        // 按订单状态分组
	ByChannel      []OrderStatRow `json:"by_channel"`       // 按支付通道分组
	ByStore        []OrderStatRow `json:"by_store"`         // 按门店分组
}

// statRowDAL 聚合查询扫描目标（含门店联表）。
type statRowDAL struct {
	Key        string
	Label      string
	OrderCount int64
	Amount     int64
	Paid       int64
}

// statSummaryDAL 汇总查询扫描目标（扁平结构，避免 GORM 扫描含 slice 的 OrderStats）。
type statSummaryDAL struct {
	OrderCount     int64
	PaidOrderCount int64
	TotalAmount    int64
	TotalPaid      int64
}

// StatsByMerchant 按商户号聚合统计订单（复用 OrderListFilter 过滤，口径与列表一致）。
func (r *OrderRepo) StatsByMerchant(ctx context.Context, f OrderListFilter) (*OrderStats, error) {
	// 每次从模型重新构建过滤查询，避免 GORM 链式调用对 Select/Group 的污染。
	base := func() *gorm.DB {
		return applyOrderFilter(r.db.WithContext(ctx).Model(&model.OrderModel{}), f)
	}

	stats := &OrderStats{}
	// 汇总：订单数 / 已支付数 / 订单金额合计 / 实付金额合计
	var summary statSummaryDAL
	if err := base().Select(
		"COUNT(*) AS order_count, "+
			"COALESCE(SUM(CASE WHEN status = 'PAID' THEN 1 ELSE 0 END), 0) AS paid_order_count, "+
			"COALESCE(SUM(amount), 0) AS total_amount, "+
			"COALESCE(SUM(paid_amount), 0) AS total_paid",
	).Scan(&summary).Error; err != nil {
		return nil, err
	}
	stats.OrderCount = summary.OrderCount
	stats.PaidOrderCount = summary.PaidOrderCount
	stats.TotalAmount = summary.TotalAmount
	stats.TotalPaid = summary.TotalPaid

	var err error
	// 按订单状态分组
	if stats.ByStatus, err = r.statGroup(base(), "status", "status", ""); err != nil {
		return nil, err
	}
	// 按支付通道分组（空通道归入「未指定」）
	if stats.ByChannel, err = r.statGroup(base(), "channel", "channel", "未指定"); err != nil {
		return nil, err
	}
	// 按门店分组（未关联门店归入「未关联门店」）
	storeBase := base().Joins("LEFT JOIN t_store ON t_store.id = t_order.store_id")
	if stats.ByStore, err = r.statGroup(storeBase, "store_id", "t_store.name", "未关联门店"); err != nil {
		return nil, err
	}
	return stats, nil
}

// statGroup 按 column 分组统计，scan 为分组键，labelCol 为展示名表达式（空则复用 column）。
func (r *OrderRepo) statGroup(base *gorm.DB, column, labelCol, emptyLabel string) ([]OrderStatRow, error) {
	var rows []statRowDAL
	if labelCol == "" {
		labelCol = column
	}
	err := base.Select(column+" AS `key`, COALESCE("+labelCol+", '') AS label, "+
		"COUNT(*) AS order_count, COALESCE(SUM(amount), 0) AS amount, COALESCE(SUM(paid_amount), 0) AS paid").
		Group(column).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]OrderStatRow, 0, len(rows))
	for _, r := range rows {
		label := r.Label
		if label == "" {
			label = emptyLabel
		}
		out = append(out, OrderStatRow{
			Key:        r.Key,
			Label:      label,
			OrderCount: r.OrderCount,
			Amount:     r.Amount,
			Paid:       r.Paid,
		})
	}
	return out, nil
}
