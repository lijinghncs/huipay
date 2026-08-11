# 微信支付平台证书自动下载设计

> 状态：设计稿（本轮仅产出接口骨架，HTTP 下载逻辑留待后续迭代实装）
> 配套骨架：`internal/payment/channel/wechat/certiface.go`

## 背景

当前微信支付回调验签依赖**静态配置**的平台公钥（`PlatformPublicKey` PEM）。微信平台证书约每 **1 年**轮换一次，静态配置在证书过期前需人工替换，存在运维风险。

本设计将验签公钥从"静态配置"升级为"动态获取 + 自动刷新"，消除人工运维。

## 目标

1. 启动时自动调用 `GET /v3/certificates` 拉取平台证书列表并解析公钥。
2. 按 `serial` 缓存公钥（`sync.Map[serial]*rsa.PublicKey`），支持多证书并存。
3. 证书过期前 **7 天**定时刷新，保证平滑轮换。
4. 与微信服务器时间对齐，避免签名 `timestamp` 校验偏差。

## 接口

```go
// CertProvider 平台证书动态获取接口（未来实装）。
type CertProvider interface {
    GetBySerial(ctx context.Context, serial string) (*rsa.PublicKey, error)
    Refresh(ctx context.Context) error
}
```

- `GetBySerial`：回调验签时按 `Wechatpay-Serial` 头取对应公钥；未命中返回错误。
- `Refresh`：拉取最新证书列表并更新缓存。

## 启动流程

1. 应用启动，构造 `CertProvider`（HTTP 实现）。
2. 调用 `Refresh()`：
   - `GET /v3/certificates`（V3 自动签名，见 `client.go`）。
   - 响应含数组，每个元素：`serial_no`、`encrypt_certificate`（AES-256-GCM 加密的 PEM 公钥）。
   - 用微信支付平台 APIv3 密钥 `APIv3Key` 解密 `encrypt_certificate`，得到 PEM 公钥。
   - 解析为 `*rsa.PublicKey`，写入 `sync.Map[serial]`。
3. 若 `Refresh` 失败：**启动不阻塞**，记录错误并回退到静态公钥（如有），保证可用性。

## 定时刷新

- 用 `time.Ticker` 每分钟检查一次（与对账调度器同模式）。
- 遍历缓存中每张证书的 `not_after`（过期时间）：
  - 若 `now` 距 `not_after` **≤ 7 天**，触发全量 `Refresh()`。
- 刷新失败仅告警，下一分钟重试。

## 回调验签接入

`verifyAndDecrypt` 改为：

```go
pub, err := a.certProvider.GetBySerial(ctx, headers["Wechatpay-Serial"])
if err != nil {
    return nil, err
}
if err := rsaSHA256Verify(pub, buildVerifySignStr(timestamp, nonce, string(raw)), signature); err != nil {
    return nil, err
}
```

与当前 `StaticCertProvider` 行为一致，验签逻辑不变，仅公钥来源抽象化。

## 时钟对齐

微信回调头含 `Wechatpay-Timestamp`。验签时：
- 校验 `|now - timestamp| ≤ 300s`（容忍网络/时钟偏差）。
- 若系统时钟与微信服务器偏差过大，可配置 CLB/NTP 校正，或通过 `X-wechatpay-timestamp` 校准偏移量（后续迭代）。

## 配置项（新增）

```yaml
wechat:
  cert_auto_refresh: true       # 是否启用自动下载（默认 false，兼容现有静态配置）
  cert_refresh_lead_days: 7     # 过期前提前刷新天数
  cert_refresh_interval: 1m     # 检查间隔
```

## 迁移路径

1. **当前**：`StaticCertProvider`（已实现，注入 PEM）。
2. **下一个迭代**：实现 `HTTPCertProvider`（上述流程），`Refresh` 拉取解密缓存。
3. **切换**：`New(cfg)` 依据 `cert_auto_refresh` 选择 provider。

## 安全隐患注意事项

- 平台证书私钥存在于微信侧，本服务只持有**公钥**，解密用的是 `APIv3Key`（商户侧密钥），两者不冲突。
- `encrypt_certificate` 解密失败（`APIv3Key` 错误）必须记录错误并保留旧证书，不覆盖。
- 勿把 `APIv3Key` 写入日志或 `/metrics`。

## Out of Scope

- 证书吊销/黑名单处理。
- 多平台证书按 appid 隔离。
- 证书下载本身的可观测性指标（可复用 `channel_latency_seconds` 的 `certificates` 标签）。