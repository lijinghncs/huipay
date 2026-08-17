package executor

import "context"
	"fmt"

func (e *Executor) checkBalance(ctx context.Context, merchantID uint64, amount int64) error {
	wallet, err := e.walletRepo.GetByEntity(ctx, merchantID)
	if err != nil {
		return err
	}
	if wallet == nil {
		return fmt.Errorf("merchant wallet not found: %d", merchantID)
	}
	if wallet.Balance < amount {
		return fmt.Errorf("insufficient balance: have %d, need %d", wallet.Balance, amount)
	}
	return nil
}
