package cache_test

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"cachex/cachex/cache"
	"cachex/cachex/hook"
)

func TestUnit_GetByUnique_RoundTrip(t *testing.T) {
	f := newFixture(t, true, true)
	defer f.teardown()

	// UK -> PK miss → DB scan PK
	f.mock.ExpectQuery("SELECT id FROM `users` WHERE email = ?").
		WithArgs("a@x").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("42"))

	// PK miss → DB get
	f.mock.ExpectQuery("SELECT * FROM `users` WHERE id = ? ORDER BY `users`.`id` LIMIT ?").
		WithArgs("42", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "email"}).AddRow(int64(42), "anna", "a@x"))

	m := cache.NewModel[User](f.cdb).WithContext(f.ctx())
	u, err := m.GetByUnique("email", "a@x")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if u.Name != "anna" {
		t.Fatalf("got %+v", u)
	}

	// 第二次：UK 命中 Redis → PK 命中 L1
	if _, err := m.GetByUnique("email", "a@x"); err != nil {
		t.Fatal(err)
	}
	if err := f.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

func TestUnit_Insert_BumpsListVersion(t *testing.T) {
	f := newFixture(t, true, true)
	defer f.teardown()

	f.mock.ExpectBegin()
	f.mock.ExpectExec("INSERT INTO `users` (`name`,`email`,`status`) VALUES (?,?,?)").
		WithArgs("x", "x@x", "active").
		WillReturnResult(sqlmock.NewResult(7, 1))
	f.mock.ExpectCommit()

	m := cache.NewModel[User](f.cdb).WithContext(f.ctx())
	if err := m.Insert(&User{Name: "x", Email: "x@x", Status: "active"}); err != nil {
		t.Fatalf("insert err=%v", err)
	}
	if v, err := f.mc.Server.Get(f.cdb.ListVersionKey("users")); err != nil || v != "1" {
		t.Fatalf("list version v=%q err=%v", v, err)
	}
}

func TestUnit_Hooks_Invoked(t *testing.T) {
	f := newFixture(t, true, true)
	defer f.teardown()

	var before, after, afterUpd, listInv int
	f.cdb.UseHook(hook.FuncHook{
		BeforeQueryFn:         func(ctx context.Context, key string) { before++ },
		AfterQueryFn:          func(ctx context.Context, key string, hit bool) { after++ },
		AfterUpdateFn:         func(ctx context.Context, key string) { afterUpd++ },
		AfterListInvalidateFn: func(ctx context.Context, table string) { listInv++ },
	})

	pkKey := f.cdb.PkKey(int64(1), "users")
	f.mc.Server.Set(pkKey, `{"ID":1,"Name":"x"}`)

	m := cache.NewModel[User](f.cdb).WithContext(f.ctx())
	_, _ = m.GetByPK(int64(1))

	if before == 0 || after == 0 {
		t.Fatalf("hooks not invoked: before=%d after=%d", before, after)
	}
}

func TestUnit_NewCacheDb_Defaults(t *testing.T) {
	c := cache.NewCacheDb(&cache.CacheDB{})
	if c.Timeout == 0 || c.NullValueTimeout == 0 || c.ListValueTimeout == 0 {
		t.Fatalf("defaults not applied: %+v", c)
	}
	if c.Limits.MaxPageSize == 0 || c.Limits.MaxConditions == 0 {
		t.Fatalf("limits defaults not applied: %+v", c.Limits)
	}
	if c.Seed == 0 {
		t.Fatalf("seed default not applied")
	}
}

