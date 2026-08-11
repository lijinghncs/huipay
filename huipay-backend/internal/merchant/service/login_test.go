package service

import (
	"context"
	"testing"

	"github.com/huipay/huipay-backend/infra/auth"
	"github.com/huipay/huipay-backend/internal/merchant/repository"
)

func setupLoginMerchant(t *testing.T) (*Service, *repository.EntityModel) {
	t.Helper()
	svc, repo := buildRuntimeDB(t)
	m := &repository.EntityModel{
		EntityCode: "M_LOGIN_1", EntityType: "MERCHANT", Name: "登录测试商户", Status: 1,
	}
	if err := repo.Create(context.Background(), m); err != nil {
		t.Fatalf("create entity: %v", err)
	}
	_, err := svc.SetLoginPassword(context.Background(), m.ID, "13800000000", "secret123")
	if err != nil {
		t.Fatalf("set login password: %v", err)
	}
	return svc, m
}

func TestMerchantLoginSuccess(t *testing.T) {
	svc, m := setupLoginMerchant(t)
	res, err := svc.Login(context.Background(), "13800000000", "secret123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if res.Token == "" {
		t.Fatal("token should not be empty")
	}
	if res.Merchant == nil || res.Merchant.ID != m.ID {
		t.Fatalf("merchant = %#v, want id %d", res.Merchant, m.ID)
	}
	claims, err := auth.Verify("test-secret", res.Token)
	if err != nil {
		t.Fatalf("verify token: %v", err)
	}
	if claims.MerchantID != m.ID {
		t.Fatalf("claims merchant_id = %d, want %d", claims.MerchantID, m.ID)
	}
}

func TestMerchantLoginWrongPassword(t *testing.T) {
	svc, _ := setupLoginMerchant(t)
	if _, err := svc.Login(context.Background(), "13800000000", "wrong-pass"); err == nil {
		t.Fatal("wrong password should fail")
	}
}

func TestMerchantLoginUnknownPhone(t *testing.T) {
	svc, _ := setupLoginMerchant(t)
	if _, err := svc.Login(context.Background(), "13900000000", "secret123"); err == nil {
		t.Fatal("unknown phone should fail")
	}
}

func TestMerchantLoginNoPasswordSet(t *testing.T) {
	svc, repo := buildRuntimeDB(t)
	m := &repository.EntityModel{EntityCode: "M_LOGIN_2", EntityType: "MERCHANT", Name: "无密码", Status: 1}
	if err := repo.Create(context.Background(), m); err != nil {
		t.Fatalf("create entity: %v", err)
	}
	if _, err := svc.Login(context.Background(), "13800000001", "secret123"); err == nil {
		t.Fatal("login without password should fail")
	}
}

func TestMerchantLoginAuthSecretMissing(t *testing.T) {
	svc, repo := buildRuntimeDB(t)
	svc.authSecret = ""
	m := &repository.EntityModel{EntityCode: "M_LOGIN_3", EntityType: "MERCHANT", Name: "无密钥", Status: 1}
	if err := repo.Create(context.Background(), m); err != nil {
		t.Fatalf("create entity: %v", err)
	}
	if _, err := svc.SetLoginPassword(context.Background(), m.ID, "13800000002", "secret123"); err != nil {
		t.Fatalf("set password: %v", err)
	}
	if _, err := svc.Login(context.Background(), "13800000002", "secret123"); err == nil {
		t.Fatal("login without auth secret should fail")
	}
}
