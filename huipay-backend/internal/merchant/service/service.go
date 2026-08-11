// 包 service 编排商户进件与列表查询，进件同时自动开通钱包，为后续支付/分账打基础。
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"go.uber.org/zap"
	"gorm.io/gorm"

	accountservice "github.com/huipay/huipay-backend/internal/account/service"
	"github.com/huipay/huipay-backend/internal/domain/entity"
	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/infra/errs"
	merchantrepo "github.com/huipay/huipay-backend/internal/merchant/repository"
	"github.com/huipay/huipay-backend/internal/merchant/secretcrypto"
	"github.com/huipay/huipay-backend/internal/order/model"
	"github.com/huipay/huipay-backend/internal/payment/channel/wechat"
	paymentcoderepo "github.com/huipay/huipay-backend/internal/paymentcode/repository"
)

// OnboardRequest 商户进件请求。
type OnboardRequest struct {
	Name         string `json:"name"`          // 商户名称
	Type         string `json:"type"`          // 主体类型，默认 MERCHANT
	LegalName    string `json:"legal_name"`    // 法人/经营者姓名
	LicenseNo    string `json:"license_no"`    // 营业执照/证件号
	BankAccount  string `json:"bank_account"`  // 结算银行卡号
	BankName     string `json:"bank_name"`     // 开户行
	ContactName  string `json:"contact_name"`  // 联系人
	ContactPhone string `json:"contact_phone"` // 联系电话
	WechatConfig *entity.WechatConfig `json:"wechat_config"` // 微信支付配置（敏感字段加密入库）
}

// WechatConfigView 微信支付配置读视图（敏感字段只回 configured 标记，不回显明文）。
type WechatConfigView struct {
	Enabled                      bool   `json:"enabled"`
	MchID                        string `json:"mchid"`
	AppID                        string `json:"appid"`
	AppSecretConfigured          bool   `json:"app_secret_configured"`
	APIv3KeyConfigured           bool   `json:"api_v3_key_configured"`
	MerchantPrivateKeyConfigured bool   `json:"merchant_private_key_configured"`
	PlatformPublicKeyConfigured  bool   `json:"platform_public_key_configured"`
	MerchantSerialNo             string `json:"merchant_serial_no"`
	PlatformSerialNo             string `json:"platform_serial_no"`
	NotifyBaseURL                string `json:"notify_base_url"`
}

// Merchant 商户视图。
type Merchant struct {
	ID         uint64    `json:"id"`
	EntityCode string    `json:"entity_code"` // 商户号
	EntityType string    `json:"entity_type"`
	Name       string    `json:"name"`
	KYCStatus  int       `json:"kyc_status"`
	Status     int       `json:"status"`
	WalletNo   string    `json:"wallet_no"`
	CreatedAt  time.Time `json:"created_at"`
}

// MerchantList 分页列表。
type MerchantList struct {
	Items []Merchant `json:"items"`
	Total int64      `json:"total"`
	Page  int        `json:"page"`
	Size  int        `json:"size"`
}

// MerchantDetail 商户详情（含商户身份认证资料与钱包余额）。
type MerchantDetail struct {
	ID         uint64         `json:"id"`
	EntityCode string         `json:"entity_code"`
	EntityType string         `json:"entity_type"`
	Name       string         `json:"name"`
	KYCStatus  int            `json:"kyc_status"`
	KYCData    map[string]any `json:"kyc_data"`
	Status     int            `json:"status"`
	WalletNo   string         `json:"wallet_no"`
	Balance    int64          `json:"balance"`
	Frozen     int64          `json:"frozen"`
	PreFrozen  int64          `json:"pre_frozen"`
	WechatConfig *WechatConfigView `json:"wechat_config"` // 微信支付配置（敏感字段仅回 configured 标记）
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

// UpdateRequest 商户资料更新请求。
type UpdateRequest struct {
	Name         string `json:"name"`
	LegalName    string `json:"legal_name"`
	LicenseNo    string `json:"license_no"`
	BankAccount  string `json:"bank_account"`
	BankName     string `json:"bank_name"`
	ContactName  string `json:"contact_name"`
	ContactPhone string `json:"contact_phone"`
	WechatConfig *entity.WechatConfig `json:"wechat_config"` // 微信支付配置（仅变更字段；空敏感字段不覆盖）
}

// UpdateWechatConfigRequest 微信支付配置更新请求（独立端点）。
type UpdateWechatConfigRequest struct {
	WechatConfig *entity.WechatConfig `json:"wechat_config"`
}

// MerchantOverview 商户经营概览。
type MerchantOverview struct {
	MerchantID      uint64 `json:"merchant_id"`
	EntityCode      string `json:"entity_code"`
	Name            string `json:"name"`
	Balance         int64  `json:"balance"`           // 钱包余额（分）
	Frozen          int64  `json:"frozen"`            // 冻结金额（分）
	TotalPaid       int64  `json:"total_paid"`        // 累计实付金额（分）
	OrderCount      int64  `json:"order_count"`       // 订单总数
	PaidOrderCount  int64  `json:"paid_order_count"`  // 已支付订单数
	ActiveCodeCount int64  `json:"active_code_count"` // 可用收款码牌数
}

// Service 商户服务。
type Service struct {
	db         *gorm.DB
	entityRepo *merchantrepo.EntityRepo
	accountSvc *accountservice.Service
	logger     *zap.Logger
}

// NewService 构造 Service。
func NewService(db *gorm.DB, entityRepo *merchantrepo.EntityRepo, accountSvc *accountservice.Service, logger *zap.Logger) *Service {
	return &Service{db: db, entityRepo: entityRepo, accountSvc: accountSvc, logger: logger}
}

// Onboard 商户进件：事务内创建主体(MERCHANT)与钱包，保证原子性。
// 商户号(entity_code)唯一键冲突时自动换号重试，其他错误原样返回。
func (s *Service) Onboard(ctx context.Context, req *OnboardRequest) (*Merchant, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, errs.New(errs.CodeInvalidParams, "merchant name is required", 400)
	}
	if req.WechatConfig != nil {
		if err := s.validateWechatConfig(req.WechatConfig); err != nil {
			return nil, err
		}
	}
	entityType := vo.EntityMerchant
	if req.Type != "" {
		entityType = vo.EntityType(req.Type)
	}

	kycBytes, _ := json.Marshal(map[string]any{
		"legal_name":    req.LegalName,
		"license_no":    req.LicenseNo,
		"bank_account":  req.BankAccount,
		"bank_name":     req.BankName,
		"contact_name":  req.ContactName,
		"contact_phone": req.ContactPhone,
	})
	if len(kycBytes) == 0 {
		kycBytes = []byte("{}")
	}

	// 进件的微信支付配置：敏感字段加密后产出待入库 JSON（无既有值，空敏感字段不写入）。
	var wcJSON string
	if req.WechatConfig != nil {
		storage, err := s.wechatConfigToStorage(nil, req.WechatConfig)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(storage)
		wcJSON = string(b)
	}

	// 商户号冲突时换号重试；非唯一键错误直接返回，不掩盖真实原因
	for i := 0; i < 3; i++ {
		m, err := s.onboardOnce(ctx, genEntityCode(), req, entityType, string(kycBytes), wcJSON)
		if err == nil {
			return m, nil
		}
		if isDuplicateKey(err) {
			continue
		}
		return nil, err
	}
	return nil, fmt.Errorf("failed to allocate unique merchant code")
}

// onboardOnce 在单个事务内完成「创建主体 + 开通钱包」，任一步失败整体回滚。
func (s *Service) onboardOnce(ctx context.Context, code string, req *OnboardRequest, entityType vo.EntityType, kycData, wechatConfig string) (*Merchant, error) {
	tx := s.entityRepo.DB().WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("begin tx: %w", tx.Error)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	m := &merchantrepo.EntityModel{
		EntityCode:   code,
		EntityType:   string(entityType),
		Name:         req.Name,
		KYCStatus:    1, // 进件完成，资料已提交
		KYCData:      kycData,
		WechatConfig: wechatConfig,
		Status:       1,
	}
	if err := tx.WithContext(ctx).Create(m).Error; err != nil {
		return nil, err
	}
	w, err := s.accountSvc.EnsureWalletTx(ctx, tx, m.ID, entityType)
	if err != nil {
		return nil, fmt.Errorf("create wallet for %s: %w", code, err)
	}
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	committed = true
	return &Merchant{
		ID:         m.ID,
		EntityCode: m.EntityCode,
		EntityType: m.EntityType,
		Name:       m.Name,
		KYCStatus:  m.KYCStatus,
		Status:     m.Status,
		WalletNo:   w.WalletNo,
		CreatedAt:  m.CreatedAt,
	}, nil
}

// isDuplicateKey 识别唯一键冲突（gorm 翻译错误或 MySQL 1062），用于商户号换号重试。
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

// List 分页查询商户列表。
func (s *Service) List(ctx context.Context, page, size int, keyword string, status *int) (*MerchantList, error) {
	if page < 1 {
		page = 1
	}
	if size <= 0 || size > 200 {
		size = 20
	}
	entities, total, err := s.entityRepo.List(ctx, merchantrepo.ListFilter{
		Keyword: keyword,
		Status:  status,
		Offset:  (page - 1) * size,
		Limit:   size,
	})
	if err != nil {
		return nil, err
	}
	items := make([]Merchant, 0, len(entities))
	for i := range entities {
		items = append(items, *s.fillMerchant(ctx, &entities[i]))
	}
	return &MerchantList{Items: items, Total: total, Page: page, Size: size}, nil
}

// fillMerchant 组装商户视图（含钱包号）。
func (s *Service) fillMerchant(ctx context.Context, e *merchantrepo.EntityModel) *Merchant {
	m := &Merchant{
		ID:         e.ID,
		EntityCode: e.EntityCode,
		EntityType: e.EntityType,
		Name:       e.Name,
		KYCStatus:  e.KYCStatus,
		Status:     e.Status,
		CreatedAt:  e.CreatedAt,
	}
	if w, err := s.accountSvc.GetWallet(ctx, e.ID); err == nil && w != nil {
		m.WalletNo = w.WalletNo
	}
	return m
}

// Get 查询商户详情（含商户身份认证资料与钱包余额）。
func (s *Service) Get(ctx context.Context, id uint64) (*MerchantDetail, error) {
	e, err := s.entityRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, errs.New(errs.CodeInvalidParams, "merchant not found", 404)
	}
	d := &MerchantDetail{
		ID:         e.ID,
		EntityCode: e.EntityCode,
		EntityType: e.EntityType,
		Name:       e.Name,
		KYCStatus:  e.KYCStatus,
		Status:     e.Status,
		CreatedAt:  e.CreatedAt,
		UpdatedAt:  e.UpdatedAt,
	}
	if e.KYCData != "" {
		var m map[string]any
		if err := json.Unmarshal([]byte(e.KYCData), &m); err == nil {
			d.KYCData = m
		}
	}
	if w, err := s.accountSvc.GetWallet(ctx, e.ID); err == nil && w != nil {
		d.WalletNo = w.WalletNo
		d.Balance = w.Balance
		d.Frozen = w.Frozen
		d.PreFrozen = w.PreFrozen
	}
	d.WechatConfig = s.wechatConfigView(e.WechatConfig)
	return d, nil
}

// Update 更新商户基础资料（名称 + 商户身份认证信息）。
func (s *Service) Update(ctx context.Context, id uint64, req *UpdateRequest) (*Merchant, error) {
	e, err := s.entityRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, errs.New(errs.CodeInvalidParams, "merchant not found", 404)
	}
	name := e.Name
	if strings.TrimSpace(req.Name) != "" {
		name = req.Name
	}
	kycBytes, _ := json.Marshal(map[string]any{
		"legal_name":    req.LegalName,
		"license_no":    req.LicenseNo,
		"bank_account":  req.BankAccount,
		"bank_name":     req.BankName,
		"contact_name":  req.ContactName,
		"contact_phone": req.ContactPhone,
	})
	if err := s.entityRepo.UpdateProfile(ctx, id, name, string(kycBytes)); err != nil {
		return nil, err
	}
	updated, err := s.entityRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.fillMerchant(ctx, updated), nil
}

// SetStatus 启用 / 停用商户。
func (s *Service) SetStatus(ctx context.Context, id uint64, status int) (*Merchant, error) {
	if status != 0 && status != 1 {
		return nil, errs.New(errs.CodeInvalidParams, "status must be 0 or 1", 400)
	}
	e, err := s.entityRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, errs.New(errs.CodeInvalidParams, "merchant not found", 404)
	}
	if err := s.entityRepo.UpdateStatus(ctx, id, status); err != nil {
		return nil, err
	}
	e.Status = status
	return s.fillMerchant(ctx, e), nil
}

// Overview 商户经营概览（余额 / 交易 / 码牌统计）。
func (s *Service) Overview(ctx context.Context, id uint64) (*MerchantOverview, error) {
	e, err := s.entityRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, errs.New(errs.CodeInvalidParams, "merchant not found", 404)
	}
	ov := &MerchantOverview{MerchantID: id, EntityCode: e.EntityCode, Name: e.Name}
	if w, err := s.accountSvc.GetWallet(ctx, id); err == nil && w != nil {
		ov.Balance = w.Balance
		ov.Frozen = w.Frozen
	}
	// 订单统计（累计实付 / 订单数 / 已支付数）
	_ = s.db.WithContext(ctx).Model(&model.OrderModel{}).Where("merchant_id = ?", id).Count(&ov.OrderCount).Error
	_ = s.db.WithContext(ctx).Model(&model.OrderModel{}).
		Where("merchant_id = ? AND status = ?", id, vo.OrderPaid).Count(&ov.PaidOrderCount).Error
	_ = s.db.WithContext(ctx).Model(&model.OrderModel{}).
		Where("merchant_id = ? AND status = ?", id, vo.OrderPaid).
		Select("COALESCE(SUM(paid_amount),0)").Scan(&ov.TotalPaid).Error
	// 可用收款码牌数
	_ = s.db.WithContext(ctx).Model(&paymentcoderepo.PaymentCodeModel{}).
		Where("merchant_id = ? AND status = 1", id).Count(&ov.ActiveCodeCount).Error
	return ov, nil
}

// genEntityCode 生成商户号：M + 时间戳低位 + 随机后缀，保证唯一。
func genEntityCode() string {
	n := time.Now().UnixNano()
	return fmt.Sprintf("M%011d%05d", n%100000000000, rand.Intn(100000))
}

// --- 商户微信支付配置 ---

// validateWechatConfig 双级校验：
//   - 启用级：enabled=true 时 mchid 必填。
//   - 字段级：已提供的私钥/平台公钥必须可解析（复用 wechat 解析语义，不保留密钥）。
//
// enabled=false 或零值配置时仅做字段级校验（提供即校验）。
func (s *Service) validateWechatConfig(cfg *entity.WechatConfig) error {
	if cfg.Enabled && strings.TrimSpace(cfg.MchID) == "" {
		return errs.New(errs.CodeInvalidParams, "wechat_config.mchid is required when enabled", 400)
	}
	if err := wechat.ValidatePrivateKey(cfg.MerchantPrivateKey); err != nil {
		return errs.New(errs.CodeInvalidParams, "wechat_config.merchant_private_key invalid", 400)
	}
	if err := wechat.ValidatePublicKey(cfg.PlatformPublicKey); err != nil {
		return errs.New(errs.CodeInvalidParams, "wechat_config.platform_public_key invalid", 400)
	}
	return nil
}

// wechatFieldByJSON 按 JSON tag 读取 WechatConfig 字段值（反射），驱动加密装配。
func wechatFieldByJSON(in *entity.WechatConfig, jsonTag string) string {
	rv := reflect.ValueOf(in).Elem()
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		if rt.Field(i).Tag.Get("json") == jsonTag && rv.Field(i).Kind() == reflect.String {
			return rv.Field(i).String()
		}
	}
	return ""
}

// wechatConfigToStorage 将输入配置与既有密文合并，产出待入库的 JSON map。
//   - 非敏感字段：输入非空则写入，空则置空（清空语义）。
//   - 敏感字段：输入非空则 AES 加密写入；空则保留既有值（不覆盖）。
//
// existing 为 nil 表示新建（进件），空敏感字段不写入。
func (s *Service) wechatConfigToStorage(existing map[string]any, in *entity.WechatConfig) (map[string]any, error) {
	out := make(map[string]any, 12)
	for k, v := range existing {
		out[k] = v
	}
	// 敏感字段：非空覆盖（加密），空保留既有
	for _, f := range entity.SensitiveFields {
		if val := wechatFieldByJSON(in, f); val != "" {
			enc, err := secretcrypto.Encrypt(val)
			if err != nil {
				return nil, err
			}
			out[f] = enc
		}
	}
	// 非敏感字段：空值清空
	setStr := func(key, val string) {
		if val != "" {
			out[key] = val
		} else {
			delete(out, key)
		}
	}
	out["enabled"] = in.Enabled
	setStr("mchid", in.MchID)
	setStr("appid", in.AppID)
	setStr("merchant_serial_no", in.MerchantSerialNo)
	setStr("platform_serial_no", in.PlatformSerialNo)
	setStr("notify_base_url", in.NotifyBaseURL)
	return out, nil
}

// wechatConfigView 由存储 JSON 构建读视图：敏感字段仅回 configured 标记（密文非空即已配置）。
func (s *Service) wechatConfigView(stored string) *WechatConfigView {
	if stored == "" || stored == "null" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(stored), &m); err != nil {
		s.logger.Warn("wechat_config unmarshal fail", zap.String("wechat_config", stored), zap.Error(err))
		return nil
	}
	nonEmpty := func(key string) bool {
		v, ok := m[key]
		return ok && v != nil && fmt.Sprint(v) != ""
	}
	return &WechatConfigView{
		Enabled:                      boolVal(m["enabled"]),
		MchID:                        strVal(m["mchid"]),
		AppID:                        strVal(m["appid"]),
		AppSecretConfigured:          nonEmpty("app_secret"),
		APIv3KeyConfigured:           nonEmpty("api_v3_key"),
		MerchantPrivateKeyConfigured: nonEmpty("merchant_private_key"),
		PlatformPublicKeyConfigured:  nonEmpty("platform_public_key"),
		MerchantSerialNo:             strVal(m["merchant_serial_no"]),
		PlatformSerialNo:             strVal(m["platform_serial_no"]),
		NotifyBaseURL:                strVal(m["notify_base_url"]),
	}
}

// GetWechatConfig 查询商户微信支付配置读视图（敏感字段不回显明文）。
func (s *Service) GetWechatConfig(ctx context.Context, id uint64) (*WechatConfigView, error) {
	e, err := s.entityRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, errs.New(errs.CodeInvalidParams, "merchant not found", 404)
	}
	return s.wechatConfigView(e.WechatConfig), nil
}

// UpdateWechatConfig 更新商户微信支付配置（合并语义：空敏感字段不覆盖，空非敏感字段清空）。
func (s *Service) UpdateWechatConfig(ctx context.Context, id uint64, req *UpdateWechatConfigRequest) (*WechatConfigView, error) {
	e, err := s.entityRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, errs.New(errs.CodeInvalidParams, "merchant not found", 404)
	}
	if req.WechatConfig == nil {
		return nil, errs.New(errs.CodeInvalidParams, "wechat_config is required", 400)
	}
	cfg := req.WechatConfig
	if err := s.validateWechatConfig(cfg); err != nil {
		return nil, err
	}
	// 读既有密文 map 作为合并基线
	existing := map[string]any{}
	if e.WechatConfig != "" && e.WechatConfig != "null" {
		_ = json.Unmarshal([]byte(e.WechatConfig), &existing)
	}
	storage, err := s.wechatConfigToStorage(existing, cfg)
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(storage)
	if err := s.entityRepo.UpdateWechatConfig(ctx, id, string(b)); err != nil {
		return nil, err
	}
	return s.wechatConfigView(string(b)), nil
}

// strVal 取 map 值字符串。
func strVal(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

// boolVal 取 map 值布尔。
func boolVal(v any) bool {
	b, _ := v.(bool)
	return b
}