// 包 service 编排门店业务：创建、列表、详情、更新、启停、删除。
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/huipay/huipay-backend/infra/errs"
	"github.com/huipay/huipay-backend/internal/store/domain"
	"github.com/huipay/huipay-backend/internal/store/repository"
)

// CreateRequest 创建门店请求。
type CreateRequest struct {
	MerchantID   uint64
	Name         string
	StoreType    string
	ContactPhone string
	Region       string
	Address      string
	Longitude    *float64
	Latitude     *float64
	Metadata     map[string]any
}

// UpdateRequest 更新门店请求。
type UpdateRequest struct {
	Name         string
	StoreType    string
	ContactPhone string
	Region       string
	Address      string
	Longitude    *float64
	Latitude     *float64
	Metadata     map[string]any
}

// StoreView 门店视图。
type StoreView struct {
	ID           uint64    `json:"id"`
	StoreCode    string    `json:"store_code"`
	MerchantID   uint64    `json:"merchant_id"`
	Name         string    `json:"name"`
	StoreType    string    `json:"store_type"`
	ContactPhone string    `json:"contact_phone"`
	Region       string    `json:"region"`
	Address      string    `json:"address"`
	Longitude    *float64  `json:"longitude"`
	Latitude     *float64  `json:"latitude"`
	Status       int       `json:"status"`
	CodeCount    int64     `json:"code_count"`
	OrderCount   int64     `json:"order_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// StoreDetail 门店详情（复用 StoreView，含关联码牌数/订单数）。
type StoreDetail struct {
	StoreView
}

// StoreList 分页列表。
type StoreList struct {
	Items []StoreView `json:"items"`
	Total int64       `json:"total"`
	Page  int         `json:"page"`
	Size  int         `json:"size"`
}

// StoreStats 门店统计（列表 KPI）。
type StoreStats struct {
	Total     int64 `json:"total"`
	Active    int64 `json:"active"`
	MonthNew  int64 `json:"month_new"`
}

// Service 门店服务。
type Service struct {
	repo   *repository.StoreRepo
	logger *zap.Logger
}

// NewService 构造 Service。
func NewService(repo *repository.StoreRepo, logger *zap.Logger) *Service {
	return &Service{repo: repo, logger: logger}
}

// Create 创建门店。
func (s *Service) Create(ctx context.Context, req *CreateRequest) (*StoreView, error) {
	if req.MerchantID == 0 {
		return nil, errs.New(errs.CodeInvalidParams, "merchant_id required", 400)
	}
	name := strings.TrimSpace(req.Name)
	if len([]rune(name)) < 2 || len([]rune(name)) > 40 {
		return nil, errs.New(errs.CodeInvalidParams, "store name must be 2-40 chars", 400)
	}
	if req.StoreType != "" && !validStoreType(req.StoreType) {
		return nil, errs.New(errs.CodeInvalidParams, "invalid store_type", 400)
	}
	metaJSON, err := marshalMetadata(req.Metadata)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInvalidParams, "invalid metadata", 400, err)
	}
	if metaJSON == "" {
		metaJSON = "null" // JSON 列不接受空字符串，置为 null
	}

	// 门店编码冲突时换号重试，最多 5 次
	for i := 0; i < 5; i++ {
		m := &repository.StoreModel{
			StoreCode:    genStoreCode(),
			MerchantID:   req.MerchantID,
			Name:         name,
			StoreType:    req.StoreType,
			ContactPhone: strings.TrimSpace(req.ContactPhone),
			Region:       strings.TrimSpace(req.Region),
			Address:      strings.TrimSpace(req.Address),
			Longitude:    req.Longitude,
			Latitude:     req.Latitude,
			Metadata:     metaJSON,
			Status:       1,
		}
		if err := s.repo.Create(ctx, m); err == nil {
			s.audit(ctx, m.ID, req.MerchantID, "CREATE", "")
			return s.toView(m), nil
		} else if !isDuplicateKey(err) {
			return nil, fmt.Errorf("create store: %w", err)
		}
	}
	return nil, fmt.Errorf("failed to allocate unique store code")
}

// List 分页查询门店。
func (s *Service) List(ctx context.Context, merchantID uint64, page, size int, keyword string, status *int) (*StoreList, error) {
	if merchantID == 0 {
		return nil, errs.New(errs.CodeInvalidParams, "merchant_id required", 400)
	}
	if page < 1 {
		page = 1
	}
	if size <= 0 || size > 200 {
		size = 20
	}
	rows, total, err := s.repo.ListByMerchant(ctx, merchantID, repository.ListFilter{
		Keyword: keyword,
		Status:  status,
		Offset:  (page - 1) * size,
		Limit:   size,
	})
	if err != nil {
		return nil, err
	}
	items := make([]StoreView, 0, len(rows))
	for i := range rows {
		items = append(items, *s.toView(&rows[i]))
	}
	return &StoreList{Items: items, Total: total, Page: page, Size: size}, nil
}

// Stats 门店统计。
func (s *Service) Stats(ctx context.Context, merchantID uint64) (*StoreStats, error) {
	if merchantID == 0 {
		return nil, errs.New(errs.CodeInvalidParams, "merchant_id required", 400)
	}
	total, err := s.repo.CountByMerchant(ctx, merchantID)
	if err != nil {
		return nil, err
	}
	active := int64(0)
	if a, err := s.repo.CountByMerchantStatus(ctx, merchantID, 1); err == nil {
		active = a
	}
	monthStart := time.Now().AddDate(0, 0, -time.Now().Day()+1).Format("2006-01-02 00:00:00")
	monthNew, err := s.repo.CountByMerchantCreatedAfter(ctx, merchantID, monthStart)
	if err != nil {
		monthNew = 0
	}
	return &StoreStats{Total: total, Active: active, MonthNew: monthNew}, nil
}

// Get 门店详情。
func (s *Service) Get(ctx context.Context, id, merchantID uint64) (*StoreDetail, error) {
	if merchantID == 0 {
		return nil, errs.New(errs.CodeInvalidParams, "merchant_id required", 400)
	}
	m, err := s.repo.GetByIDAndMerchant(ctx, id, merchantID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, errs.New(errs.CodeInvalidParams, "store not found", 404)
	}
	d := &StoreDetail{StoreView: *s.toView(m)}
	if c, err := s.repo.CountCodesByStore(ctx, id); err == nil {
		d.CodeCount = c
	}
	if o, err := s.repo.CountOrdersByStore(ctx, id); err == nil {
		d.OrderCount = o
	}
	return d, nil
}

// Update 更新门店。
func (s *Service) Update(ctx context.Context, id, merchantID uint64, req *UpdateRequest) (*StoreView, error) {
	if merchantID == 0 {
		return nil, errs.New(errs.CodeInvalidParams, "merchant_id required", 400)
	}
	existing, err := s.repo.GetByIDAndMerchant(ctx, id, merchantID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, errs.New(errs.CodeInvalidParams, "store not found", 404)
	}
	if req.Name != "" {
		name := strings.TrimSpace(req.Name)
		if len([]rune(name)) < 2 || len([]rune(name)) > 40 {
			return nil, errs.New(errs.CodeInvalidParams, "store name must be 2-40 chars", 400)
		}
	}
	if req.StoreType != "" && !validStoreType(req.StoreType) {
		return nil, errs.New(errs.CodeInvalidParams, "invalid store_type", 400)
	}
	metaJSON, err := marshalMetadata(req.Metadata)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInvalidParams, "invalid metadata", 400, err)
	}
	fields := map[string]any{}
	if req.Name != "" {
		fields["name"] = strings.TrimSpace(req.Name)
	}
	if req.StoreType != "" {
		fields["store_type"] = req.StoreType
	}
	if req.ContactPhone != "" {
		fields["contact_phone"] = strings.TrimSpace(req.ContactPhone)
	}
	if req.Region != "" {
		fields["region"] = strings.TrimSpace(req.Region)
	}
	if req.Address != "" {
		fields["address"] = strings.TrimSpace(req.Address)
	}
	if req.Longitude != nil {
		fields["longitude"] = *req.Longitude
	}
	if req.Latitude != nil {
		fields["latitude"] = *req.Latitude
	}
	if metaJSON != "" {
		fields["metadata"] = metaJSON
	}
	if len(fields) == 0 {
		return s.toView(existing), nil
	}
	if err := s.repo.Update(ctx, id, merchantID, fields); err != nil {
		return nil, err
	}
	s.audit(ctx, id, merchantID, "UPDATE", "")
	updated, err := s.repo.GetByIDAndMerchant(ctx, id, merchantID)
	if err != nil || updated == nil {
		return s.toView(existing), nil
	}
	return s.toView(updated), nil
}

// SetStatus 启停门店。
func (s *Service) SetStatus(ctx context.Context, id, merchantID uint64, status int) (*StoreView, error) {
	if merchantID == 0 {
		return nil, errs.New(errs.CodeInvalidParams, "merchant_id required", 400)
	}
	if status != 0 && status != 1 {
		return nil, errs.New(errs.CodeInvalidParams, "invalid status", 400)
	}
	existing, err := s.repo.GetByIDAndMerchant(ctx, id, merchantID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, errs.New(errs.CodeInvalidParams, "store not found", 404)
	}
	if err := s.repo.UpdateStatus(ctx, id, merchantID, status); err != nil {
		return nil, err
	}
	s.audit(ctx, id, merchantID, "STATUS", fmt.Sprintf(`{"status":%d}`, status))
	existing.Status = status
	return s.toView(existing), nil
}

// Delete 删除门店（仅无关联码牌/订单可删，软删除）。
func (s *Service) Delete(ctx context.Context, id, merchantID uint64) error {
	if merchantID == 0 {
		return errs.New(errs.CodeInvalidParams, "merchant_id required", 400)
	}
	existing, err := s.repo.GetByIDAndMerchant(ctx, id, merchantID)
	if err != nil {
		return err
	}
	if existing == nil {
		return errs.New(errs.CodeInvalidParams, "store not found", 404)
	}
	if c, err := s.repo.CountCodesByStore(ctx, id); err == nil && c > 0 {
		return errs.New(errs.CodeInvalidParams, "store has associated payment codes, cannot delete", 400)
	}
	if o, err := s.repo.CountOrdersByStore(ctx, id); err == nil && o > 0 {
		return errs.New(errs.CodeInvalidParams, "store has associated orders, cannot delete", 400)
	}
	if err := s.repo.SoftDelete(ctx, id, merchantID); err != nil {
		return err
	}
	s.audit(ctx, id, merchantID, "DELETE", "")
	return nil
}

// toView 模型转视图。
func (s *Service) toView(m *repository.StoreModel) *StoreView {
	return &StoreView{
		ID:           m.ID,
		StoreCode:    m.StoreCode,
		MerchantID:   m.MerchantID,
		Name:         m.Name,
		StoreType:    m.StoreType,
		ContactPhone: m.ContactPhone,
		Region:       m.Region,
		Address:      m.Address,
		Longitude:    m.Longitude,
		Latitude:     m.Latitude,
		Status:       m.Status,
		CodeCount:    m.CodeCount,
		OrderCount:   m.OrderCount,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}

// audit 写入门店审计日志。
func (s *Service) audit(ctx context.Context, storeID, merchantID uint64, action, detail string) {
	if detail == "" {
		detail = "{}"
	}
	_ = s.repo.AuditLog(ctx, &repository.AuditLogItem{
		StoreID:    storeID,
		MerchantID: merchantID,
		Action:     action,
		Detail:     detail,
	})
}

// validStoreType 校验门店类型枚举。
func validStoreType(t string) bool {
	return domain.ValidStoreTypes[domain.StoreType(t)]
}

// marshalMetadata 序列化 metadata；空返回 ""。
func marshalMetadata(md map[string]any) (string, error) {
	if len(md) == 0 {
		return "", nil
	}
	b, err := json.Marshal(md)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// genStoreCode 生成门店编码：S + 14 位时间戳 + 8 位随机数字。
func genStoreCode() string {
	ts := time.Now().Format("20060102150405")
	randNum := fmt.Sprintf("%08d", rand.Intn(100000000))
	return "S" + ts + randNum
}

// isDuplicateKey 识别唯一键冲突。
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