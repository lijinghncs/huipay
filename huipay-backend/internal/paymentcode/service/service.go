// 包 service 编排收款码牌业务：创建、列表、停用。
package service

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/paymentcode/repository"
	storerepo "github.com/huipay/huipay-backend/internal/store/repository"
)

// 生成短码用到的字符集（排除易混淆字符 0/O、1/I/L）。
const codeAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

// 短码长度。
const codeLength = 6

// CreateRequest 创建码牌请求。
type CreateRequest struct {
	MerchantID uint64
	Remark     string
	StoreID    uint64 // 关联门店 ID（软约束，0 表示不绑定）
}

// Code 码牌视图。
type Code struct {
	ID          uint64    `json:"id"`
	MerchantID  uint64    `json:"merchant_id"`
	StoreID     *uint64   `json:"store_id,omitempty"`
	StoreName   string    `json:"store_name,omitempty"`
	CodeID      string    `json:"code_id"`
	Status      int       `json:"status"`
	Remark      string    `json:"remark"`
	CheckoutURL string    `json:"checkout_url"` // 扫码直达收银台金额输入页
	CreatedAt   time.Time `json:"created_at"`
	DisabledAt  *time.Time `json:"disabled_at,omitempty"`
}

// CodeList 分页列表。
type CodeList struct {
	Items []Code `json:"items"`
	Total int64  `json:"total"`
	Page  int    `json:"page"`
	Size  int    `json:"size"`
}

// Service 码牌服务。
type Service struct {
	repo        *repository.PaymentCodeRepo
	storeRepo   *storerepo.StoreRepo
	logger      *zap.Logger
	checkoutBase string // 收银台 H5 地址前缀，如 https://checkout.huipay.cn
}

// NewService 构造 Service。
func NewService(repo *repository.PaymentCodeRepo, storeRepo *storerepo.StoreRepo, logger *zap.Logger, checkoutBase string) *Service {
	if checkoutBase == "" {
		checkoutBase = "https://checkout.huipay.cn"
	}
	return &Service{repo: repo, storeRepo: storeRepo, logger: logger, checkoutBase: checkoutBase}
}

// Create 创建码牌：短码唯一键冲突时自动换号重试，最多 5 次。
func (s *Service) Create(ctx context.Context, req *CreateRequest) (*Code, error) {
	if req.MerchantID == 0 {
		return nil, errs.New(errs.CodeInvalidParams, "merchant_id required", 200)
	}
	remark := strings.TrimSpace(req.Remark)
	if len(remark) > 64 {
		remark = remark[:64]
	}
	// 关联门店校验（软约束：0 表示不绑定；非 0 时门店必须属于当前商户）
	var storeID *uint64
	if req.StoreID > 0 {
		if storeRepo := s.storeRepo; storeRepo != nil {
			store, err := storeRepo.GetByIDAndMerchant(ctx, req.StoreID, req.MerchantID)
			if err != nil {
				return nil, err
			}
			if store == nil {
				return nil, errs.New(errs.CodeInvalidParams, "store not found for merchant", 200)
			}
		}
		id := req.StoreID
		storeID = &id
	}
	for i := 0; i < 5; i++ {
		m := &repository.PaymentCodeModel{
			MerchantID: req.MerchantID,
			StoreID:    storeID,
			CodeID:     genCodeID(),
			Status:     int(vo.CodeActive),
			Remark:     remark,
		}
		if err := s.repo.Create(ctx, m); err == nil {
			return s.toCode(m), nil
		} else if !isDuplicateKey(err) {
			return nil, fmt.Errorf("create payment code: %w", err)
		}
	}
	return nil, fmt.Errorf("failed to allocate unique payment code")
}

// List 分页查询当前商户码牌。
func (s *Service) List(ctx context.Context, merchantID uint64, page, size int, status *int, storeID *uint64) (*CodeList, error) {
	if merchantID == 0 {
		return nil, errs.New(errs.CodeInvalidParams, "merchant_id required", 200)
	}
	if page < 1 {
		page = 1
	}
	if size <= 0 || size > 200 {
		size = 20
	}
	rows, total, err := s.repo.ListByMerchant(ctx, repository.PaymentCodeFilter{
		MerchantID: merchantID,
		StoreID:    storeID,
		Status:     status,
		Offset:     (page - 1) * size,
		Limit:      size,
	})
	if err != nil {
		return nil, err
	}
	items := make([]Code, 0, len(rows))
	for i := range rows {
		items = append(items, *s.toCode(&rows[i]))
	}
	return &CodeList{Items: items, Total: total, Page: page, Size: size}, nil
}

// Disable 停用码牌（仅本商户可操作）。
func (s *Service) Disable(ctx context.Context, id, merchantID uint64) error {
	if merchantID == 0 {
		return errs.New(errs.CodeInvalidParams, "merchant_id required", 200)
	}
	// 权限校验：仅本商户的码牌可停用
	existing, err := s.repo.GetByIDAndMerchant(ctx, id, merchantID)
	if err != nil {
		return err
	}
	if existing == nil {
		return errs.New(errs.CodeInvalidParams, "payment code not found", 200)
	}
	if _, err := s.repo.Disable(ctx, id); err != nil {
		return err
	}
	return nil
}

// SetStore 绑定/解绑码牌门店（storeID 为 0 表示解绑；非 0 时门店必须属于当前商户）。
func (s *Service) SetStore(ctx context.Context, id, merchantID, storeID uint64) (*Code, error) {
	if merchantID == 0 {
		return nil, errs.New(errs.CodeInvalidParams, "merchant_id required", 200)
	}
	// 权限校验 + 存在性校验
	existing, err := s.repo.GetByIDAndMerchant(ctx, id, merchantID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, errs.New(errs.CodeInvalidParams, "payment code not found", 200)
	}
	// 绑定门店时校验门店归属
	var bind *uint64
	if storeID > 0 {
		if s.storeRepo != nil {
			store, err := s.storeRepo.GetByIDAndMerchant(ctx, storeID, merchantID)
			if err != nil {
				return nil, err
			}
			if store == nil {
				return nil, errs.New(errs.CodeInvalidParams, "store not found for merchant", 200)
			}
		}
		bind = &storeID
	}
	if _, err := s.repo.UpdateStore(ctx, id, merchantID, bind); err != nil {
		return nil, err
	}
	// 重新读取最新记录，返回带门店名的视图
	updated, err := s.repo.GetByIDAndMerchant(ctx, id, merchantID)
	if err != nil || updated == nil {
		return s.toCode(existing), nil
	}
	return s.toCode(updated), nil
}

// toCode 模型转视图。
func (s *Service) toCode(m *repository.PaymentCodeModel) *Code {
	return &Code{
		ID:          m.ID,
		MerchantID:  m.MerchantID,
		StoreID:     m.StoreID,
		StoreName:   m.StoreName,
		CodeID:      m.CodeID,
		Status:      m.Status,
		Remark:      m.Remark,
		CheckoutURL: s.checkoutBase + "/h5?code=" + m.CodeID,
		CreatedAt:   m.CreatedAt,
		DisabledAt:  m.DisabledAt,
	}
}

// genCodeID 生成 6 位短码（排除歧义字符）。
func genCodeID() string {
	b := make([]byte, codeLength)
	for i := range b {
		b[i] = codeAlphabet[rand.Intn(len(codeAlphabet))]
	}
	return string(b)
}

// isDuplicateKey 识别唯一键冲突（gorm 翻译错误或 MySQL 1062），用于短码换号重试。
func isDuplicateKey(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var me *mysql.MySQLError
	if errors.As(err, &me) && me.Number == 1062 {
		return true
	}
	return false
}
