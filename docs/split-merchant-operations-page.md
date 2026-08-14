# 商户运营页面 · 具体实现方案（迭代 2）

> 目标：让商户在商家工作台自助完成「生成分账单 → 查看明细 → 审批/驳回 → 执行结果」与「按时间段分账」，并让分账记录页可运营（筛选/重试/试算）。
> 前置：分账后端 API 已全部就绪（规则/执行/账单/记录），前端已有 SplitRules（规则）、Splits（记录）两页；本方案补齐 SplitBills 页与记录页增强。
> 依赖标注：**[Bx]** 表示需要后端小改造，其余为纯前端。

---

## 一、信息架构与菜单

「收银与资金」分组新增「分账单」菜单（`/split-bills`），最终分账模块三个入口：

| 菜单 | 路由 | 职责 |
|---|---|---|
| 分账规则 | `/split-rules` | 规则 CRUD / 门店配置 / 试算 |
| 分账 | `/splits` | 分账记录（按订单聚合）列表 + 明细 + 筛选 + 重试 |
| 分账单 | `/split-bills`（新增） | 账单生成 / 明细 / 审批 / 驳回 / 执行 |

---

## 二、分账单页（SplitBills）

### 2.1 页面结构

```
┌ 顶部摘要（3 个 KPI 卡） ─────────────────────────────┐
│  待审批 N   本月执行 ¥X   本月驳回 N                  │
├ 卡片：分账单 ────────────────── [生成账单] ─────────┤
│ 表格：批次号/规则/时间段/总额/状态/创建时间/操作        │
│ 空态：暂无分账单 → 生成第一张账单                       │
└─────────────────────────────────────────────────────┘
```

- **状态徽章**：`PENDING` 待审批（琥珀）/ `APPROVED` 已通过（蓝）/ `REJECTED` 已驳回（灰）/ `EXECUTED` 已执行（绿），带圆点不只看颜色。
- **列表列**：
  - 批次号：等宽字体，点击复制
  - 规则名称 + 规则编码（两级）
  - 时间段：`MM-DD HH:mm ~ MM-DD HH:mm`（跨年显示年份）
  - 分账总额：等宽 + 右对齐
  - 状态徽章、创建时间
  - 操作：详情（全部）；审批 / 驳回（仅 PENDING）；查看（其余）
- **分页**：默认 20 条，`showSizeChanger`，与 Splits 页一致。

### 2.2 生成账单（两步式 Modal，宽 720）

**Step 1 · 选择规则与时间段**
- 规则下拉：仅列启用中规则（复用 `listSplitRules`），带「门店配置 N 家/全部门店」角标
- 时间段 `RangePicker`：
  - 限制：结束时间 ≤ 当前时间；跨度 ≤ 31 天（超出提示）
  - 快捷选项：近 7 天 / 近 30 天
- 实时显示预估基数：选完后调 **[B1] 预览接口** 展示「时间段实收总额 ¥X」（不落库）

**Step 2 · 预览明细**
- 明细表：门店名称 / 门店编码 / 可分金额 / 占比（金额 ÷ 总额）
- 底部：合计 = 预览总额，注明「未分配部分归商户」
- 两个动作：
  - **生成账单（走审批）**（主按钮）→ `generateSplitBill`
  - **生成并立即执行**（次按钮 + Popconfirm 确认）→ `executeSplitByPeriod`，文案提示「跳过审批、直接扣款执行」

### 2.3 详情抽屉（宽 560）

- `Descriptions`：批次号 / 规则 / 时间段 / 总额 / 状态 / 创建时间 / 审批时间 / 执行时间
- 明细表：接收方（名称 + 类型）、金额、占比
- 底部操作（仅 PENDING）：
  - 「审批通过并执行」→ `approveSplitBill`，失败时展示后端错误（如「钱包余额不足」）
  - 「驳回」→ `rejectSplitBill`，Popconfirm 确认
- 已执行账单：展示执行时间，提供「查看分账记录」跳转 `/splits`（按批次号订单定位）

### 2.4 交互与反馈

- 生成/审批/驳回均为 `loading → success/error`（message 就近反馈）
- 审批余额不足：错误消息明确「钱包余额不足，请先结算或充值」+ 重试
- 生成成功后自动打开详情；列表 `invalidate` 刷新

---

## 三、分账记录页增强（Splits）

### 3.1 筛选栏（表格上方）

- 状态 `Select`：成功 / 部分失败 / 失败（全部）
- 时间段 `RangePicker`（按执行时间）
- 命中规则 `Select`（可选，**[B2]**）
- 查询 / 重置按钮；分页保留

> **[B2]** `GET /v1/merchant/split/executions` 增加 `status / start / end / rule_id` 过滤参数；聚合 SQL 增加 `MAX(se.rule_id)` 与 `LEFT JOIN t_split_rule` 回填 `rule_name`，summary 增加 `rule_name` 字段。

### 3.2 重试（失败 / 部分失败）

- 行内「重试」按钮（仅 PARTIAL / FAILED 显示）→ `POST /v1/merchant/split/executions/:order_no/retry`
- **[B3]** 后端实现：按订单重建分配（读执行记录中非 SUCCESS 接收方 + 规则/订单信息），调 executor 重跑（幂等跳过已成功接收方），更新聚合状态
- 重试中按钮 loading；完成后刷新列表并提示「重试完成：成功 N / 失败 M」

### 3.3 明细抽屉增强

- 新增展示：命中规则名称（**[B2]** 提供）、Level 层级、重试次数、失败原因（已有 `last_error`）
- 订单号点击 → 跳转 `/transactions?order_no=xxx`（交易详情）
- 失败明细行红色高亮 + 展开错误详情

---

## 四、规则试算（SplitRules 页增强）

- 规则列表操作列新增「试算」按钮 → 试算 Modal（宽 560）
- 输入：金额（元，必填，1–50000）、门店（单选/多选，复用门店选择，默认全部门店）、渠道（可选）
- 点击「试算」→ **[B1] 预览接口** → 展示：
  - 分配明细表：接收方 / 金额 / 占比
  - 「剩余归商户 ¥X」（金额未分配完时）
- 纯展示不落库；关闭即销毁

---

## 五、后端配套改造清单（[B1]–[B3]）

| 编号 | 接口/改动 | 说明 |
|---|---|---|
| B1 | `POST /v1/merchant/split/preview`（新增） | 入参两种模式：`amount`（单笔试算，按当前规则 + 门店实收占比）或 `start/end`（时间段账单预览）；返回 `{ total_amount, items[{receiver_name, amount, ratio}] , merchant_remain }`；复用 `buildAllocations`/`buildAllocationsPeriod` + `fillBillItemNames`，不落库 |
| B2 | `GET /v1/merchant/split/executions` 增强 | 过滤参数 `status/start/end/rule_id`；summary 回填 `rule_name` |
| B3 | `POST /v1/merchant/split/executions/:order_no/retry`（新增） | 重建未成功接收方分配并重跑 executor；返回重试结果统计 |

> B1 同时服务「账单生成预览」「按时间段分账预览」「规则试算」三处，接口收敛为一个。

---

## 六、涉及文件

| 文件 | 改动 |
|---|---|
| `pages/SplitBills/index.tsx`（新建） | 分账单页：KPI + 列表 + 生成两步 Modal + 详情抽屉 |
| `components/SplitPreview.tsx`（新建） | 预览明细组件（账单/试算共用） |
| `pages/Splits/index.tsx` | 筛选栏、重试按钮、规则名列、明细增强 |
| `pages/SplitRules/index.tsx` | 试算按钮 + Modal |
| `services/user.ts` | `previewSplit`、`listSplitExecutions` 参数扩展、`retrySplitExecution`、`rule_name` 类型 |
| `config/routes.ts`、`App.tsx`、`layouts/BasicLayout.tsx` | 新增 `/split-bills` 菜单与路由 |
| `internal/split/service/service.go`、`handler/handler.go` | B1 预览、B3 重试 |
| `internal/split/executor/executor.go`、`repository/` | B2 过滤 + rule_name 回填 |

---

## 七、验收标准

1. 商户可在分账单页生成账单（两步：选规则+时间段 → 预览明细 → 确认），生成后状态 PENDING。
2. 待审批账单可查看明细、审批通过并执行、驳回；审批时余额不足给出明确提示且不执行。
3. 「生成并立即执行」直接完成分账，执行记录可在分账页看到且金额一致。
4. 分账记录页支持按状态/时间段/规则筛选；失败/部分失败可重试，重试只补齐未成功接收方。
5. 规则试算输入金额后可预览各门店分配与剩余归商户金额，不产生任何落库数据。
6. 三端（分账规则/分账/分账单）菜单入口清晰，移动端可折叠导航访问；`vite build` 通过，后端 `go build/test` 通过。
