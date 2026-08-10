// 对账单 GZIP+CSV 解析。
package reconcile

import (
	"compress/gzip"
	"encoding/csv"
	"io"
	"strconv"
)

// BillEntry 微信交易对账单中的一笔交易。
type BillEntry struct {
	TradeTime     string // 交易时间
	TransactionID string // 微信订单号（第 5 列）
	OutTradeNo    string // 商户订单号（第 6 列）
	TradeType     string // 交易类型
	TradeState    string // 交易状态
	OrderAmount   int64  // 订单金额（分，退款为负）
}

// weChatBillCols 交易对账单关键列索引（0 起）。
const (
	colTradeTime     = 0
	colTransactionID = 4
	colOutTradeNo    = 5
	colTradeType     = 7
	colTradeState    = 8
	colOrderAmount   = 23
)

// parseBill 从 gzip 压缩的 CSV 流中解析对账条目。
// 表头行与末尾汇总行（首列为"总交易单数"）会被跳过。
func parseBill(r io.Reader) ([]BillEntry, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	cr := csv.NewReader(gz)
	cr.FieldsPerRecord = -1 // 兼容列数不一致的行

	var entries []BillEntry
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(rec) <= colOrderAmount {
			continue
		}
		// 跳过表头与汇总行
		if rec[colTradeTime] == "交易时间" || rec[colTradeTime] == "总交易单数" {
			continue
		}
		entries = append(entries, BillEntry{
			TradeTime:     rec[colTradeTime],
			TransactionID: rec[colTransactionID],
			OutTradeNo:    rec[colOutTradeNo],
			TradeType:     rec[colTradeType],
			TradeState:    rec[colTradeState],
			OrderAmount:   parseAmount(rec[colOrderAmount]),
		})
	}
	return entries, nil
}

// parseAmount 解析微信账单金额（分）；非数字返回 0。
func parseAmount(s string) int64 {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return v
}