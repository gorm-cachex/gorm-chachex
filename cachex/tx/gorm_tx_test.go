package tx

import (
	"context"
	"database/sql"
	"sync/atomic"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func newGormDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	mockDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(false))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	dialector := mysql.New(mysql.Config{
		Conn:                      mockDB,
		SkipInitializeWithVersion: true,
	})
	gdb, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	return gdb, mock, mockDB
}

func TestUnit_AfterCommit_NoTx_RunsImmediately(t *testing.T) {
	m := &DefaultTxManager{}
	var ran int32
	m.AfterCommit(context.Background(), func() { atomic.AddInt32(&ran, 1) })
	if ran != 1 {
		t.Fatalf("expected immediate run, got %d", ran)
	}
}

func TestUnit_AfterCommit_RunsAfterCommit(t *testing.T) {
	gdb, mock, raw := newGormDB(t)
	defer raw.Close()

	mock.ExpectBegin()
	mock.ExpectCommit()

	m := &DefaultTxManager{}
	ctx, err := m.Begin(context.Background(), gdb)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}

	var ran int32
	m.AfterCommit(ctx, func() { atomic.AddInt32(&ran, 1) })
	if ran != 0 {
		t.Fatalf("after-commit must defer until Commit, got ran=%d", ran)
	}
	if err := m.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if ran != 1 {
		t.Fatalf("after Commit ran=%d", ran)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expect: %v", err)
	}
}

func TestUnit_Rollback_DoesNotRunAfterCommit(t *testing.T) {
	gdb, mock, raw := newGormDB(t)
	defer raw.Close()

	mock.ExpectBegin()
	mock.ExpectRollback()

	m := &DefaultTxManager{}
	ctx, err := m.Begin(context.Background(), gdb)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	var ran int32
	m.AfterCommit(ctx, func() { atomic.AddInt32(&ran, 1) })
	_ = m.Rollback(ctx)
	if ran != 0 {
		t.Fatalf("rollback should not trigger after-commit, got %d", ran)
	}
}

func TestUnit_AfterCommit_PanicLoggedAndPropagates(t *testing.T) {
	gdb, mock, raw := newGormDB(t)
	defer raw.Close()

	mock.ExpectBegin()
	mock.ExpectCommit()

	m := &DefaultTxManager{}
	ctx, _ := m.Begin(context.Background(), gdb)
	m.AfterCommit(ctx, func() { panic("boom") })

	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("panic should propagate to caller")
		}
	}()
	_ = m.Commit(ctx)
}

func TestUnit_InTx(t *testing.T) {
	m := &DefaultTxManager{}
	if m.InTx(context.Background()) {
		t.Fatalf("plain ctx should not be in tx")
	}
	gdb, mock, raw := newGormDB(t)
	defer raw.Close()
	mock.ExpectBegin()
	ctx, _ := m.Begin(context.Background(), gdb)
	if !m.InTx(ctx) {
		t.Fatalf("ctx after Begin must be in tx")
	}
}

