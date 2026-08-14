// 临时验证：A3 账单审批乐观锁 + D2 审计写入。
package main

import (
	"context"
	"fmt"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	splitrepo "github.com/huipay/huipay-backend/internal/split/repository"
)

const dsn = "root:lijing123!@#@tcp(127.0.0.1:3306)/huipay?charset=utf8mb4&parseTime=True&loc=Local"

func main() {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		panic(err)
	}
	ctx := context.Background()
	billRepo := splitrepo.NewSplitBillRepo(db)
	auditRepo := splitrepo.NewSplitAuditRepo(db)

	// 创建一张 PENDING 账单
	batch := fmt.Sprintf("VERIFYBILL%d", time.Now().UnixNano()%1000000)
	bill := &splitrepo.SplitBillModel{
		BatchNo:     batch,
		MerchantID:  3,
		RuleCode:    "R1786601193054",
		RuleName:    "分账-验证",
		StartTime:   time.Now().Add(-24 * time.Hour),
		EndTime:     time.Now(),
		TotalAmount: 100,
		Detail:      "[]",
		OrderNos:    "[]",
		Status:      splitrepo.BillPending,
	}
	if err := billRepo.Create(ctx, bill); err != nil {
		panic(err)
	}
	fmt.Printf("[A3] created PENDING bill id=%d batch=%s\n", bill.ID, batch)

	// 并发审批模拟：两次 UpdateStatus(EXECUTED)，仅第一次应生效
	now := time.Now()
	ok1, err1 := billRepo.UpdateStatus(ctx, bill.ID, splitrepo.BillExecuted, map[string]any{"executed_at": now})
	ok2, err2 := billRepo.UpdateStatus(ctx, bill.ID, splitrepo.BillExecuted, map[string]any{"executed_at": now})
	fmt.Printf("[A3] first approve applied=%v err=%v ; second approve applied=%v err=%v\n", ok1, err1, ok2, err2)
	if ok1 && !ok2 {
		fmt.Println("[A3] PASS: 乐观锁生效，仅第一次审批成功")
	} else {
		fmt.Println("[A3] FAIL: 期望 first=true second=false")
	}

	// 驳回已执行账单应失败（非 PENDING）
	okR, _ := billRepo.UpdateStatus(ctx, bill.ID, splitrepo.BillRejected, nil)
	fmt.Printf("[A3] reject on EXECUTED applied=%v (期望 false)\n", okR)

	// D2 审计写入
	audit := &splitrepo.SplitAuditModel{
		BizType:      "BILL",
		BizID:        batch,
		Action:       "APPROVE",
		OperatorType: "MERCHANT",
		OperatorID:   3,
		Detail:       `{"total_amount":100}`,
	}
	if err := auditRepo.Append(ctx, audit); err != nil {
		fmt.Println("[D2] FAIL append:", err)
	} else {
		var cnt int64
		db.Model(&splitrepo.SplitAuditModel{}).Where("biz_id = ?", batch).Count(&cnt)
		fmt.Printf("[D2] PASS: audit written, biz_id=%s count=%d\n", batch, cnt)
	}
}