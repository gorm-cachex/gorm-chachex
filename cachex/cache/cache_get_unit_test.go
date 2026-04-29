package cache_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"

	"cachex/cachex/cache"
	"cachex/log"
)

// withCapturedLog 捕获 log 包输出，便于断言。日志输出全局共享，需互斥。
var logMu sync.Mutex

func withCapturedLog(t *testing.T) (*bytes.Buffer, func()) {
	t.Helper()
	logMu.Lock()
	buf := &bytes.Buffer{}
	prev := log.GetLogLevel()
	log.SetOutput(buf)
	log.SetLogLevel(log.DebugLevel)
	return buf, func() {
		log.SetOutput(nil)
		log.SetLogLevel(prev)
		logMu.Unlock()
	}
}

func TestUnit_GetByPK_RedisHit_NoErrLog(t *testing.T) {
	f := newFixture(t, true, true)
	defer f.teardown()

	// 预置 Redis 命中
	pkKey := f.cdb.PkKey(int64(1), "users")
	val := `{"ID":1,"Name":"alice","Email":"a@x","Status":"active"}`
	f.mc.Server.Set(pkKey, val)

	buf, restore := withCapturedLog(t)
	defer restore()

	m := cache.NewModel[User](f.cdb).WithContext(f.ctx())
	u, err := m.GetByPK(int64(1))
	if err != nil || u == nil || u.ID != 1 || u.Name != "alice" {
		t.Fatalf("got u=%+v err=%v", u, err)
	}
	out := buf.String()
	if strings.Contains(out, "ERR ") && strings.Contains(out, "<nil>") {
		t.Fatalf("should not log nil-error: %s", out)
	}
	if !strings.Contains(out, "hit_layer=redis") {
		t.Fatalf("expected redis hit log, got: %s", out)
	}
}

func TestUnit_GetByPK_NoSleep_FastPath(t *testing.T) {
	f := newFixture(t, true, true)
	defer f.teardown()

	pkKey := f.cdb.PkKey(int64(2), "users")
	f.mc.Server.Set(pkKey, `{"ID":2,"Name":"b"}`)

	m := cache.NewModel[User](f.cdb).WithContext(f.ctx())
	start := time.Now()
	if _, err := m.GetByPK(int64(2)); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Fatalf("redis hit too slow: %v", d)
	}
}

func TestUnit_GetByPK_Miss_DB_Hit(t *testing.T) {
	f := newFixture(t, true, true)
	defer f.teardown()

	rows := sqlmock.NewRows([]string{"id", "name", "email", "status"}).
		AddRow(int64(3), "carol", "c@x", "active")
	f.mock.ExpectQuery("SELECT * FROM `users` WHERE id = ? ORDER BY `users`.`id` LIMIT ?").
		WithArgs(int64(3), 1).
		WillReturnRows(rows)

	m := cache.NewModel[User](f.cdb).WithContext(f.ctx())
	u, err := m.GetByPK(int64(3))
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if u.Name != "carol" {
		t.Fatalf("got %+v", u)
	}

	// 第二次：应走 L1（不再触发 sqlmock；ExpectationsWereMet 检查只期望了一次）
	if _, err := m.GetByPK(int64(3)); err != nil {
		t.Fatal(err)
	}
	if err := f.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

func TestUnit_GetByPK_NotFound_NullCache(t *testing.T) {
	f := newFixture(t, true, true)
	defer f.teardown()

	f.mock.ExpectQuery("SELECT * FROM `users` WHERE id = ? ORDER BY `users`.`id` LIMIT ?").
		WithArgs(int64(404), 1).
		WillReturnError(gorm.ErrRecordNotFound)

	m := cache.NewModel[User](f.cdb).WithContext(f.ctx())
	if _, err := m.GetByPK(int64(404)); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("want NotFound, got %v", err)
	}
	// Redis 中应有 __null__
	pkKey := f.cdb.PkKey(int64(404), "users")
	if v, err := f.mc.Server.Get(pkKey); err != nil || v != "__null__" {
		t.Fatalf("null cache not written: v=%q err=%v", v, err)
	}

	// 第二次不再访问 DB
	if _, err := m.GetByPK(int64(404)); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("second call: %v", err)
	}
	if err := f.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

func TestUnit_GetByPK_Singleflight(t *testing.T) {
	f := newFixture(t, true, false) // 关闭 L1，避免本进程 L1 干扰 SF 验证
	defer f.teardown()

	rows := sqlmock.NewRows([]string{"id", "name"}).AddRow(int64(7), "g")
	f.mock.ExpectQuery("SELECT * FROM `users` WHERE id = ? ORDER BY `users`.`id` LIMIT ?").
		WithArgs(int64(7), 1).
		WillReturnRows(rows)

	const N = 100
	m := cache.NewModel[User](f.cdb).WithContext(f.ctx())
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			_, _ = m.GetByPK(int64(7))
		}()
	}
	wg.Wait()
	if err := f.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: only 1 DB hit expected, got: %v", err)
	}
}

func TestUnit_GetByPK_L1Disabled_NoPanic(t *testing.T) {
	f := newFixture(t, true, false)
	defer f.teardown()

	pkKey := f.cdb.PkKey(int64(8), "users")
	f.mc.Server.Set(pkKey, `{"ID":8,"Name":"h"}`)

	m := cache.NewModel[User](f.cdb).WithContext(f.ctx())
	if u, err := m.GetByPK(int64(8)); err != nil || u.Name != "h" {
		t.Fatalf("u=%+v err=%v", u, err)
	}
}

func TestUnit_GetByConditions_NullCache(t *testing.T) {
	f := newFixture(t, true, true)
	defer f.teardown()

	conds := []cache.CacheCondition{{Field: "status", Op: cache.OpEq, Value: "ghost"}}

	f.mock.ExpectQuery("SELECT id FROM `users` WHERE status = ?").
		WithArgs("ghost").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	m := cache.NewModel[User](f.cdb).WithContext(f.ctx())
	_, err := m.GetByConditions(conds)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("want NotFound, got %v", err)
	}
	ck := f.cdb.CompositeKey(conds, "users")
	v, gerr := f.mc.Server.Get(ck)
	if gerr != nil || v != "__null__" {
		t.Fatalf("null not cached: v=%q err=%v", v, gerr)
	}
}

func TestUnit_ReqID_Propagation(t *testing.T) {
	f := newFixture(t, true, true)
	defer f.teardown()

	pkKey := f.cdb.PkKey(int64(9), "users")
	f.mc.Server.Set(pkKey, `{"ID":9,"Name":"i"}`)

	buf, restore := withCapturedLog(t)
	defer restore()

	ctx := context.WithValue(context.Background(), log.ReqIDKey, "req-zzz")
	m := cache.NewModel[User](f.cdb).WithContext(ctx)
	if _, err := m.GetByPK(int64(9)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "[req-zzz]") {
		t.Fatalf("req_id not propagated: %s", buf.String())
	}
}

