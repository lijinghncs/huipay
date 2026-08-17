// Package domain 是对账（recon）领域的纯领域层：实体与字典常量，零外部依赖。
package domain

// 差异类型（唯一定义点；与 t_reconcile_diff.diff_type 字典一致）。
const (
	DiffTypeLong          = "LONG"            // 渠道对账：本地有、渠道无
	DiffTypeShort         = "SHORT"           // 渠道对账：渠道有、本地无
	DiffTypeMismatch      = "MISMATCH"        // 渠道对账：金额不一致
	DiffTypeSplitTotal    = "SPLIT_TOTAL"     // 前置对账 Layer A：总额不一致
	DiffTypeSplitDetail   = "SPLIT_DETAIL"    // 前置对账 Layer B：门店×日期不一致
	DiffTypeSplitPost     = "SPLIT_POST"      // 执行后对账：账本 ≠ 执行
	DiffTypeSplitDegraded = "SPLIT_DEGRADED" // 执行后对账：降级执行
)

// 审计字典（与 t_split_audit 既有取值一致；recon 侧写入时引用本定义，避免反向依赖 split 仓储）。
const (
	AuditBizTypeDailySplit     = "DAILY_SPLIT"
	AuditActionReconcilePassed = "RECONCILE_PASSED"
	AuditActionReconcileFailed = "RECONCILE_FAILED"
	AuditOperatorSystem        = "SYSTEM"
)

// 差异原因分类（domain.Diff.Reason 取值）。
const (
	ReasonLocalOnly     = "local_only"      // 渠道对账：本地有、渠道无
	ReasonRemoteOnly    = "remote_only"     // 渠道对账：渠道有、本地无
	ReasonMismatch      = "mismatch"        // 渠道对账：金额不一致
	ReasonJournalVsExec = "journal_vs_exec" // 执行后对账：账本与执行不一致
	ReasonDegraded      = "degraded"        // 执行后对账：降级执行
)

// Diff 单条对账差异（比对结果视图，不含持久化字段；落库由 repository 补齐商户/日期/类型）。
type Diff struct {
	StoreID       uint64 // 门店维度（前置对账 Layer B）
	BizDate       string // 业务日期 YYYY-MM-DD（前置对账 Layer B）
	OrderNo       string
	TransactionID string
	LocalAmount   int64 // 本地侧金额（订单/账本/本地订单）
	RemoteAmount  int64 // 对端侧金额（日报/执行/渠道账单）
	Reason        string
	Detail        string // 落库 detail 列内容（纯文本或 JSON，由各 Job 生成）
}

// Amount 差异金额（本地 - 对端）。
func (d Diff) Amount() int64 { return d.LocalAmount - d.RemoteAmount }

// CheckResult 前置对账结果（Pass=false 时阻断分账执行；DiffID 指向已落库的差异行）。
type CheckResult struct {
	Pass      bool
	TotalDiff *int64
	Diffs     []Diff
	DiffID    *uint64
}

// LocalOrder 本地已支付订单（渠道对账本地侧）。
type LocalOrder struct {
	OrderNo        string
	ChannelTradeNo string
	MerchantID     uint64
	PaidAmount     int64
}

// BillEntry 渠道账单条目（渠道对账对端侧）。
type BillEntry struct {
	TransactionID string
	OutTradeNo    string
	OrderAmount   int64
}

// OrderExecSum 单笔订单的分账执行汇总（执行后对账对端侧）。
type OrderExecSum struct {
	OrderNo  string
	ExecSum  int64
	Degraded int
}
