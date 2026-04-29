package cache_test

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"cachex/cachex/cache"
	cerr "cachex/cachex/errors"
)

func TestUnit_Update_InvalidatesPK(t *testing.T) {
	f := newFixture(t, true, true)
	defer f.teardown()

	// 预置缓存
	pkKey := f.cdb.PkKey(int64(1), "users")
	f.mc.Server.Set(pkKey, `{"ID":1,"Name":"old"}`)

	f.mock.ExpectBegin()
	f.mock.ExpectExec("UPDATE `users` SET `name`=? WHERE id = ?").
		WithArgs("new", int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	f.mock.ExpectCommit()

	m := cache.NewModel[User](f.cdb).WithContext(f.ctx())
	res, err := m.Update(int64(1), map[string]any{"name": "new"}, nil)
	if err != nil {
		t.Fatalf("update err=%v", err)
	}
	if !res.Updated || res.RowsAffected != 1 {
		t.Fatalf("res=%+v", res)
	}

	// PK 缓存应失效
	if _, err := f.mc.Server.Get(pkKey); err == nil {
		t.Fatalf("pk cache should be deleted, still exists")
	}

	// list version 应被 incr 到 1（首次 incr 会从无到 1）
	if v, err := f.mc.Server.Get(f.cdb.ListVersionKey("users")); err != nil || v != "1" {
		t.Fatalf("list version v=%q err=%v", v, err)
	}

	if err := f.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

func TestUnit_Update_OptimisticLock(t *testing.T) {
	f := newFixture(t, true, true)
	defer f.teardown()

	expected := int64(5)
	f.mock.ExpectBegin()
	f.mock.ExpectExec("UPDATE `users` SET `name`=?,`version`=? WHERE id = ? AND version = ?").
		WithArgs("nx", expected+1, int64(1), expected).
		WillReturnResult(sqlmock.NewResult(0, 1))
	f.mock.ExpectCommit()

	m := cache.NewModel[User](f.cdb).WithContext(f.ctx())
	_, err := m.Update(int64(1), map[string]any{"name": "nx"}, &cache.UpdateOption{ExpectedVersion: &expected})
	if err != nil {
		t.Fatalf("update err=%v", err)
	}
	if err := f.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

func TestUnit_Update_NoRows(t *testing.T) {
	f := newFixture(t, true, true)
	defer f.teardown()

	f.mock.ExpectBegin()
	f.mock.ExpectExec("UPDATE `users` SET `name`=? WHERE id = ?").
		WithArgs("z", int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	f.mock.ExpectCommit()

	m := cache.NewModel[User](f.cdb).WithContext(f.ctx())
	res, err := m.Update(int64(1), map[string]any{"name": "z"}, nil)
	if !errors.Is(err, cerr.RowsAffected) {
		t.Fatalf("want RowsAffected, got %v", err)
	}
	if res.Updated {
		t.Fatalf("Updated should be false")
	}
}

