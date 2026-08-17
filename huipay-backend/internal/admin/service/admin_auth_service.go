// 包 service 提供管理后台业务编排。
package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/infra/auth"
	"github.com/huipay/huipay-backend/infra/errs"
)

// AdminAuthService 管理后台登录与鉴权。
type AdminAuthService struct {
	username string
	password string
	secret   string
	logger   *zap.Logger
}

// NewAdminAuthService 构造。
func NewAdminAuthService(username, password, secret string, logger *zap.Logger) *AdminAuthService {
	return &AdminAuthService{username: username, password: password, secret: secret, logger: logger}
}

// AdminLoginResult 登录结果。
type AdminLoginResult struct {
	Token string `json:"token"`
	Admin struct {
		ID       uint64 `json:"id"`
		Username string `json:"username"`
	} `json:"admin"`
}

// Login 管理后台登录：校验用户名密码（从配置文件读取），返回 admin token。
func (s *AdminAuthService) Login(ctx context.Context, username, password string) (*AdminLoginResult, error) {
	if s.username == "" || s.password == "" {
		return nil, errs.New(errs.CodeInternalError, "admin account not configured", 500)
	}
	if username != s.username || password != s.password {
		return nil, errs.New(errs.CodeUnauthorized, "账号或密码错误", 401)
	}
	if s.secret == "" {
		return nil, errs.New(errs.CodeInternalError, "auth secret not configured", 500)
	}
	token, err := auth.SignAdmin(s.secret, 1) // admin ID 固定为 1
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, "sign admin token failed", 500, err)
	}
	var result AdminLoginResult
	result.Token = token
	result.Admin.ID = 1
	result.Admin.Username = s.username
	return &result, nil
}