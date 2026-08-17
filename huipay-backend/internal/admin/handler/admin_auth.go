// 包 handler 提供管理后台登录接口。
package handler

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/huipay/huipay-backend/infra/errs"
	adminservice "github.com/huipay/huipay-backend/internal/admin/service"
)

// AdminAuthHandler 管理后台登录 Handler。
type AdminAuthHandler struct {
	svc    *adminservice.AdminAuthService
	logger *zap.Logger
}

// NewAdminAuthHandler 构造 Handler。
func NewAdminAuthHandler(svc *adminservice.AdminAuthService, logger *zap.Logger) *AdminAuthHandler {
	return &AdminAuthHandler{svc: svc, logger: logger}
}

// Login POST /v1/admin/login 管理后台登录。
func (h *AdminAuthHandler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errs.Fail(c, h.logger, errs.New(errs.CodeInvalidParams, "username and password required", 200))
		return
	}
	res, err := h.svc.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		_ = c.Error(err)
		return
	}
	errs.OK(c, res)
}