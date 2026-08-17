package executor

import (
	"context"
	"fmt"
)

func (e *Executor) checkBalance(ctx context.Context, walletID uint64, amount int64) error {
	w, err := e.walletRepo.GetByID(ctx, walletID)
	if err != nil {
		return err
	}
	if w == nil {
		return fmt.Errorf("wallet not found: %d", walletID)
	}
	if w.Balance < amount {
		return fmt.Errorf("insufficient balance: have %d, need %d", w.Balance, amount)
	}
	return nil
}
