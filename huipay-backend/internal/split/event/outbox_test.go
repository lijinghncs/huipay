package event

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/glebarez/sqlite"
)

func setupOutboxDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// 创建出箱表（SQLite 兼容语法，不支持 CURRENT_TIMESTAMP(3)）
	err = db.Exec(`CREATE TABLE t_outbox_event (
		id TEXT PRIMARY KEY,
		aggregate_type TEXT NOT NULL,
		aggregate_id TEXT NOT NULL,
		event_type TEXT NOT NULL,
		payload TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'PENDING',
		retry_count INTEGER NOT NULL DEFAULT 0,
		next_retry_at DATETIME,
		processed_at DATETIME,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`).Error
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	return db
}

func TestOutboxRepo_InsertAndPoll(t *testing.T) {
	db := setupOutboxDB(t)
	repo := NewOutboxRepo(db)
	ctx := context.Background()

	payload, _ := json.Marshal(map[string]string{"order": "ORD001"})
	evt := &DomainEvent{
		ID:            "EVT001",
		AggregateType: "SPLIT_ORDER",
		AggregateID:   "ORD001",
		EventType:     TypeSplitOrderExecuted,
		Payload:       payload,
		Timestamp:     time.Now(),
	}

	if err := repo.Insert(ctx, evt); err != nil {
		t.Fatalf("insert: %v", err)
	}

	events, err := repo.PollPending(ctx, 10)
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 pending event, got %d", len(events))
	}
	if events[0].ID != "EVT001" {
		t.Errorf("expected EVT001, got %s", events[0].ID)
	}
	if events[0].Status != "PENDING" {
		t.Errorf("expected status PENDING, got %s", events[0].Status)
	}
}

func TestOutboxRepo_IdempotentInsert(t *testing.T) {
	db := setupOutboxDB(t)
	repo := NewOutboxRepo(db)
	ctx := context.Background()

	payload, _ := json.Marshal(map[string]string{"order": "ORD001"})
	evt := &DomainEvent{
		ID:            "EVT002",
		AggregateType: "SPLIT_ORDER",
		AggregateID:   "ORD002",
		EventType:     TypeSplitOrderExecuted,
		Payload:       payload,
		Timestamp:     time.Now(),
	}

	if err := repo.Insert(ctx, evt); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := repo.Insert(ctx, evt); err != nil {
		t.Fatalf("second insert: %v", err)
	}

	events, err := repo.PollPending(ctx, 10)
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event (idempotent), got %d", len(events))
	}
}

func TestOutboxRepo_MarkProcessed(t *testing.T) {
	db := setupOutboxDB(t)
	repo := NewOutboxRepo(db)
	ctx := context.Background()

	payload, _ := json.Marshal(map[string]string{"order": "ORD003"})
	evt := &DomainEvent{
		ID:            "EVT003",
		AggregateType: "SPLIT_ORDER",
		AggregateID:   "ORD003",
		EventType:     TypeSplitOrderExecuted,
		Payload:       payload,
		Timestamp:     time.Now(),
	}
	if err := repo.Insert(ctx, evt); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := repo.MarkProcessed(ctx, "EVT003"); err != nil {
		t.Fatalf("mark processed: %v", err)
	}

	// processed 事件不应被轮询到
	events, err := repo.PollPending(ctx, 10)
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 pending events after mark processed, got %d", len(events))
	}
}

func TestOutboxRepo_MarkFailed(t *testing.T) {
	db := setupOutboxDB(t)
	repo := NewOutboxRepo(db)
	ctx := context.Background()

	payload, _ := json.Marshal(map[string]string{"order": "ORD004"})
	evt := &DomainEvent{
		ID:            "EVT004",
		AggregateType: "SPLIT_ORDER",
		AggregateID:   "ORD004",
		EventType:     TypeSplitOrderExecuted,
		Payload:       payload,
		Timestamp:     time.Now(),
	}
	if err := repo.Insert(ctx, evt); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := repo.MarkFailed(ctx, "EVT004", "timeout"); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	// 刚标记失败（next_retry_at > now），不应被轮询到
	events, err := repo.PollPending(ctx, 10)
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 pending events immediately after mark failed, got %d", len(events))
	}
}

func TestOutboxRepo_DeleteProcessed(t *testing.T) {
	db := setupOutboxDB(t)
	repo := NewOutboxRepo(db)
	ctx := context.Background()

	payload, _ := json.Marshal(map[string]string{"order": "ORD005"})
	evt := &DomainEvent{
		ID:            "EVT005",
		AggregateType: "SPLIT_ORDER",
		AggregateID:   "ORD005",
		EventType:     TypeSplitOrderExecuted,
		Payload:       payload,
		Timestamp:     time.Now(),
	}
	if err := repo.Insert(ctx, evt); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := repo.MarkProcessed(ctx, "EVT005"); err != nil {
		t.Fatalf("mark processed: %v", err)
	}

	if err := repo.DeleteProcessed(ctx, time.Now().Add(1*time.Hour)); err != nil {
		t.Fatalf("delete processed: %v", err)
	}

	// 验证已删除
	var count int64
	db.Model(&OutboxEventModel{}).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 rows after delete, got %d", count)
	}
}

func TestOutboxRepo_PublishEvent(t *testing.T) {
	db := setupOutboxDB(t)
	repo := NewOutboxRepo(db)
	ctx := context.Background()

	payload := SplitOrderExecutedPayload{
		OrderNo:       "ORD010",
		MerchantID:    1,
		Status:        "SUCCESS",
		SuccessCount:  3,
		ReceiverCount: 3,
		TotalAmount:   10000,
	}
	if err := repo.PublishEvent(ctx, "SPLIT_ORDER", "ORD010", TypeSplitOrderExecuted, payload); err != nil {
		t.Fatalf("publish event: %v", err)
	}

	events, err := repo.PollPending(ctx, 10)
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != TypeSplitOrderExecuted {
		t.Errorf("expected event type %s, got %s", TypeSplitOrderExecuted, events[0].EventType)
	}

	// 验证 payload 可反序列化
	var decoded SplitOrderExecutedPayload
	if err := json.Unmarshal(events[0].Payload, &decoded); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if decoded.OrderNo != "ORD010" {
		t.Errorf("expected payload order_no ORD010, got %s", decoded.OrderNo)
	}
}

// 确保表名正确
func TestOutboxEventModel_TableName(t *testing.T) {
	m := OutboxEventModel{}
	if m.TableName() != "t_outbox_event" {
		t.Errorf("expected table name t_outbox_event, got %s", m.TableName())
	}
}

// 确保 OnConflict 子句不会在 SQLite 上出错
func TestOutboxRepo_OnConflictSQLite(t *testing.T) {
	db := setupOutboxDB(t)

	// 测试冲突插入
	payload, _ := json.Marshal(map[string]string{"test": "conflict"})
	model := &OutboxEventModel{
		ID:            "EVT100",
		AggregateType: "TEST",
		AggregateID:   "T001",
		EventType:     "TEST",
		Payload:       payload,
		Status:        "PENDING",
		CreatedAt:     time.Now(),
	}
	err := db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoNothing: true}).Create(model).Error
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	err = db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoNothing: true}).Create(model).Error
	if err != nil {
		t.Fatalf("conflict create: %v", err)
	}
}