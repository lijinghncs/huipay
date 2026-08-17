// Package domain 定义对账（recon）域的纯领域模型与字典常量。
// 本包零外部依赖，不导入任何基础设施或业务域包。
package domain

// 差异类型（t_reconcile_diff.diff_type 字典，统一定义点）。
const (
	DiffTypeLong          = "LONG"           // 渠道对账：本地有单，渠道无单
	DiffTypeShort         = "SHORT"          // 渠道对账：渠道有单，本地无单
	DiffTypeMismatch      = "MISMATCH"       // 渠道对账：金额不一致
	DiffTypeSplitTotal    = "SPLIT_TOTAL"    // 前置对账 Layer A：商户级总额不平
	DiffTypeSplitDetail   = "SPLIT_DETAIL"   // 前置对账 Layer B：门店×日期明细不平
	DiffTypeSplitPost     = "SPLIT_POST"     // 执行后对账：账本 vs 执行金额不平
	DiffTypeSplitDegraded = "SPLIT_DEGRADED" // 执行后对账：降级订单提示级差异
)

// 差异原因（比对结果标注）。
const (
	ReasonLocalOnly      = "local_only"
	ReasonRemoteOnly     = "remote_only"
	ReasonMismatch       = "mismatch"
	ReasonDegraded       = "degraded"
	ReasonJournalVsExec  = "journal_vs_exec"
)

// 审计字典（写入 t_split_audit，值与 split 域字典一致）。
const (
	AuditBizTypeDailySplit     = "DAILY_SPLIT"
	AuditActionReconcilePassed = "RECONCILE_PASSED"
	AuditActionReconcileFailed = "RECONCILE_FAILED"
	AuditOperatorSystem        = "SYSTEM"
)

// Diff 统一对账差异行（比对结果视图）。
// 金额用指针区分「该侧无金额（NULL）」与「金额为 0」，与落库语义一致。
type Diff struct {
	StoreID       uint64 // 门店维度（前置对账 Layer B）
	BizDate       string // 业务日期 YYYY-MM-DD
	OrderNo       string
	TransactionID string
	LocalAmount   *int64 // 本地侧金额（订单/账本/本地订单）
	RemoteAmount  *int64 // 对端侧金额（日报/执行/渠道账单）
	Reason        string
	Detail        string // 落库 detail 列内容（纯文本或 JSON，由各 Job 生成）
}

// Amount 差异金额（本地 - 对端，nil 按 0 计）。
func (d Diff) Amount() int64 { return deref(d.LocalAmount) - deref(d.RemoteAmount) }

// Local 本地侧金额（nil 按 0 计）。
func (d Diff) Local() int64 { return deref(d.LocalAmount) }

// Remote 对端侧金额（nil 按 0 计）。
func (d Diff) Remote() int64 { return deref(d.RemoteAmount) }

func deref(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

// Int64 取 int64 指针（构造 Diff 用）。
func Int64(v int64) *int64 { return &v }

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
