package cache_test

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"cachex/cachex/cache"
)

func TestUnit_Transaction_CommitPath(t *testing.T) {
	f := newFixture(t, false, true) // 用真实 DefaultTxManager
	defer f.teardown()

	f.mock.ExpectBegin()
	f.mock.ExpectExec("UPDATE `users` SET `name`=? WHERE id = ?").
		WithArgs("tx", int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	f.mock.ExpectCommit()

	err := f.cdb.Transaction(context.Background(), func(ctx context.Context) error {
		m := cache.NewModel[User](f.cdb).WithContext(ctx)
		return m.UpdateByPKWithTx(int64(1), map[string]any{"name": "tx"})
	})
	if err != nil {
		t.Fatalf("tx err=%v", err)
	}
	if err := f.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

func TestUnit_Transaction_RollbackPath(t *testing.T) {
	f := newFixture(t, false, true)
	defer f.teardown()

	f.mock.ExpectBegin()
	f.mock.ExpectRollback()

	wantErr := errors.New("biz fail")
	err := f.cdb.Transaction(context.Background(), func(ctx context.Context) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("want biz fail, got %v", err)
	}
}

