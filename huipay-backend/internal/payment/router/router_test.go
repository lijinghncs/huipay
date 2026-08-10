package router

import (
	"context"
	"testing"

	"github.com/huipay/huipay-backend/internal/domain/vo"
	"github.com/huipay/huipay-backend/internal/payment/channel"
)

// mockAdapter 最小化 Adapter 实现，仅用于验证路由行为。
type mockAdapter struct {
	code vo.ChannelCode
}

func (m *mockAdapter) Code() vo.ChannelCode { return m.code }

func (m *mockAdapter) CreatePayment(ctx context.Context, req *channel.CreatePaymentRequest) (*channel.CreatePaymentResponse, error) {
	return &channel.CreatePaymentResponse{}, nil
}

func (m *mockAdapter) QueryPayment(ctx context.Context, channelTradeNo string) (*channel.PaymentStatus, error) {
	return &channel.PaymentStatus{}, nil
}

func (m *mockAdapter) Refund(ctx context.Context, req *channel.RefundRequest) (*channel.RefundResponse, error) {
	return &channel.RefundResponse{}, nil
}

func (m *mockAdapter) Split(ctx context.Context, req *channel.SplitRequest) (*channel.SplitResponse, error) {
	return &channel.SplitResponse{}, nil
}

func (m *mockAdapter) ReturnSplit(ctx context.Context, req *channel.ReturnSplitRequest) error { return nil }

func (m *mockAdapter) FinishSplit(ctx context.Context, req *channel.FinishSplitRequest) error { return nil }

func (m *mockAdapter) CloseOrder(ctx context.Context, orderNo string) error { return nil }

func (m *mockAdapter) VerifyAndDecrypt(ctx context.Context, raw []byte, headers map[string]string) ([]byte, error) {
	return raw, nil
}

func (m *mockAdapter) VerifyNotify(ctx context.Context, raw []byte, headers map[string]string) (*channel.NotifyPayload, error) {
	return &channel.NotifyPayload{}, nil
}

func newTestRouter(channels ...vo.ChannelCode) *Router {
	rt := NewDefaultRouter()
	for _, c := range channels {
		rt.Register(&mockAdapter{code: c})
	}
	return rt
}

func TestRouteMerchantSpecified(t *testing.T) {
	rt := newTestRouter(vo.ChannelWeChat, vo.ChannelAlipay)
	d, err := rt.Route(context.Background(), &Request{Channel: vo.ChannelAlipay})
	if err != nil {
		t.Fatalf("Route(merchant specified) error: %v", err)
	}
	if d.Channel != vo.ChannelAlipay {
		t.Fatalf("expected channel %s, got %s", vo.ChannelAlipay, d.Channel)
	}
	if d.Reason != "merchant specified" {
		t.Fatalf("expected reason 'merchant specified', got %q", d.Reason)
	}
}

func TestRouteDefaultWeChat(t *testing.T) {
	rt := newTestRouter(vo.ChannelWeChat, vo.ChannelAlipay)
	d, err := rt.Route(context.Background(), &Request{})
	if err != nil {
		t.Fatalf("Route(default) error: %v", err)
	}
	if d.Channel != vo.ChannelWeChat {
		t.Fatalf("expected fallback channel %s, got %s", vo.ChannelWeChat, d.Channel)
	}
	if d.Reason != "default" {
		t.Fatalf("expected reason 'default', got %q", d.Reason)
	}
}

func TestRouteFallbackAlipay(t *testing.T) {
	rt := newTestRouter(vo.ChannelAlipay)
	d, err := rt.Route(context.Background(), &Request{})
	if err != nil {
		t.Fatalf("Route(fallback) error: %v", err)
	}
	if d.Channel != vo.ChannelAlipay {
		t.Fatalf("expected fallback channel %s, got %s", vo.ChannelAlipay, d.Channel)
	}
	if d.Reason != "fallback" {
		t.Fatalf("expected reason 'fallback', got %q", d.Reason)
	}
}

func TestRouteMerchantNotAvailable(t *testing.T) {
	rt := newTestRouter(vo.ChannelWeChat)
	_, err := rt.Route(context.Background(), &Request{Channel: vo.ChannelUnionPay})
	if err == nil {
		t.Fatal("expected error for unavailable channel, got nil")
	}
}

func TestRouteNoAvailableChannel(t *testing.T) {
	rt := newTestRouter()
	_, err := rt.Route(context.Background(), &Request{})
	if err == nil {
		t.Fatal("expected error for no available channel, got nil")
	}
}

func TestAvailable(t *testing.T) {
	rt := newTestRouter(vo.ChannelWeChat)
	if !rt.Available(vo.ChannelWeChat) {
		t.Fatal("expected WeChat available")
	}
	if rt.Available(vo.ChannelAlipay) {
		t.Fatal("expected Alipay unavailable")
	}
}

func TestGetAdapter(t *testing.T) {
	rt := newTestRouter(vo.ChannelWeChat)
	if rt.GetAdapter(vo.ChannelWeChat) == nil {
		t.Fatal("expected adapter for WeChat")
	}
	if rt.GetAdapter(vo.ChannelAlipay) != nil {
		t.Fatal("expected nil adapter for unregistered channel")
	}
}