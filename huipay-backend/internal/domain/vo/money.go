// 包 vo 定义领域值对象。
package vo

import "errors"

// Money 以分为单位，避免浮点精度问题。
type Money struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// CNY 构造人民币 Money。
func CNY(amount int64) Money { return Money{Amount: amount, Currency: "CNY"} }

// IsZero 是否为零。
func (m Money) IsZero() bool { return m.Amount == 0 }

// IsNegative 是否为负。
func (m Money) IsNegative() bool { return m.Amount < 0 }

// Add 加法。
func (m Money) Add(o Money) (Money, error) {
	if m.Currency != o.Currency {
		return Money{}, errors.New("currency mismatch")
	}
	return Money{Amount: m.Amount + o.Amount, Currency: m.Currency}, nil
}

// Sub 减法。
func (m Money) Sub(o Money) (Money, error) {
	if m.Currency != o.Currency {
		return Money{}, errors.New("currency mismatch")
	}
	return Money{Amount: m.Amount - o.Amount, Currency: m.Currency}, nil
}

// Mul 乘法（用于比例计算）。
func (m Money) Mul(ratioBps int64) Money {
	return Money{Amount: m.Amount * ratioBps / 10000, Currency: m.Currency}
}