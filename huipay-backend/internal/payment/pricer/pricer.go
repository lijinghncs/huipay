// 包 pricer 提供计价引擎骨架。
// P3 阶段实现"券 + 营销 + 积分"的最优解算法。
package pricer

// Coupon 券（最小结构）。
type Coupon struct {
	Code      string
	Discount  int64  // 分
	Type      string // CASH / PERCENT
	Stackable bool
}

// PriceInput 计价输入。
type PriceInput struct {
	Amount     int64
	Coupons    []Coupon
}

// PriceResult 计价结果。
type PriceResult struct {
	FinalAmount    int64
	BestCouponCode string
	Discount       int64
}

// Price 计价（骨架：选择第一张可用的券）。
func Price(in PriceInput) PriceResult {
	for _, c := range in.Coupons {
		if c.Discount > 0 {
			final := in.Amount - c.Discount
			if final < 0 {
				final = 0
			}
			return PriceResult{FinalAmount: final, BestCouponCode: c.Code, Discount: c.Discount}
		}
	}
	return PriceResult{FinalAmount: in.Amount, Discount: 0}
}