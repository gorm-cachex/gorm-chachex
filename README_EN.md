# gorm-cachex

**🌐 Language**: [简体中文](./README.md) | **English**

> A high-performance caching extension for GORM: **L1 + Redis + singleflight**, with built-in protection against cache penetration, breakdown, and avalanche, plus first-class observability.

---

## 📑 Table of Contents

- [Features](#-features)
- [Architecture](#%EF%B8%8F-architecture)
- [Install](#-install)
- [Quick Start](#-quick-start)
- [Core Usage](#-core-usage)
- [Transactions](#-transactions)
- [Concurrency Helpers](#-concurrency-helpers)
- [Observability](#-observability)
- [Cache Strategy](#%EF%B8%8F-cache-strategy)
- [Cache Protection](#%EF%B8%8F-cache-protection)
- [Extensibility](#-extensibility)
- [Testing](#-testing)
- [Benchmark](#-benchmark)
- [Consistency Notes](#%EF%B8%8F-consistency-notes)
- [Design Philosophy](#-design-philosophy)
- [Contributing](#%EF%B8%8F-contributing)

---

## ✨ Features

- 🚀 **L1 in-memory cache** — nanosecond latency
- ⚡ **Redis L2 cache** — shared across instances, absorbs traffic spikes
- 🛡️ **singleflight** — collapses concurrent loads, prevents cache breakdown
- ❄️ **Null cache** — anti-penetration via `__null__` sentinel
- 🌪️ **TTL jitter** — randomized expiration prevents avalanche
- 🔁 **Auto invalidation** — fired *after* transaction commit (never before)
- 📦 **List cache with version** — pagination cache invalidated by atomic version bumps
- 🔀 **Sharding router** — pluggable DB/table routing
- 🔌 **Pluggable** Router / Cache / TxManager / Hook
- 🔭 **Structured logs** — `op / key / hit_layer / elapsed_ms / req_id`
- 🪢 **Goroutine helpers** — `log.Go / GoSafe` propagate reqId and recover panics

---

## 🏗️ Architecture

```
        Request
          ↓
     L1 Cache  (~ns)
          ↓ miss
   Redis L2    (~µs)
          ↓ miss
  singleflight (dedupe concurrent loads)
          ↓
    Database   (~ms)
```

Write path:

```
  Update / Insert
        ↓
   DB write
        ↓
  Tx Commit ────► AfterCommit ────► Del PK / L1 / Bump list version
                                      (no invalidation if not committed,
                                       avoids dirty reads)
```

---

## 📦 Install

```bash
go get github.com/<your-org>/gorm-cachex
```

Dependencies: `gorm.io/gorm`, `github.com/redis/go-redis/v9`, `golang.org/x/sync/singleflight`.

---

## 🚀 Quick Start

### Initialize

```go
import (
    "cachex/cachex/cache"
    "cachex/cachex/tx"
)

cacheDB := cache.NewCacheDb(&cache.CacheDB{
    DB:        db,                              // *gorm.DB
    Cache:     &cache.RedisCache{Client: rdb},  // L2
    TxManager: &tx.DefaultTxManager{},          // transaction manager
    // Router: myRouter,                        // optional: sharding

    // Redis TTL
    Timeout:          time.Minute * 10,
    Seed:             500,                       // jitter window (seconds)
    NullValueTimeout: time.Minute * 5,
    ListValueTimeout: time.Minute * 10,

    // L1 cache
    EnableL1Cache: true,
    L1ValueTTL:    2 * time.Second,
    L1NullTTL:     500 * time.Millisecond,
})
```

### Define a Model

`User` must implement the `cache.PK` interface:

```go
type User struct {
    ID    int64 `gorm:"primaryKey"`
    Name  string
    Email string
}

func (u User) GetPK() string    { return strconv.FormatInt(u.ID, 10) }
func (User) TableName() string  { return "users" }
func (User) PrimaryKey() string { return "id" }
```

---

## 🧩 Core Usage

```go
m := cache.NewModel[User](cacheDB).WithContext(ctx)

// Insert: writes DB and bumps list version on commit
_ = m.Insert(&User{Name: "Alice"})

// Get by primary key: L1 → Redis → singleflight → DB
u, err := m.GetByPK(int64(1))

// Get by unique key: UK → PK → Entity (reuses PK cache)
u2, err := m.GetByUnique("email", "alice@example.com")

// Get by composite conditions (only equality conditions are cacheable)
u3, err := m.GetByConditions([]cache.CacheCondition{
    {Field: "status", Op: cache.OpEq, Value: "active"},
})

// Paginated list: caches PK array + batch loads entities
list, total, err := m.GetList(
    []cache.CacheCondition{{Field: "status", Op: cache.OpEq, Value: "active"}},
    /*page*/ 1, /*size*/ 10, /*order*/ nil,
)

// Update: invalidates L1 + Redis PK + bumps list version
_, _ = m.Update(int64(1), map[string]any{"name": "ggboy"}, nil)

// Optimistic locking
v := int64(3)
_, _ = m.Update(int64(1), map[string]any{"name": "x"}, &cache.UpdateOption{
    ExpectedVersion: &v,
})
```

---

## 🔁 Transactions

```go
err := cacheDB.Transaction(ctx, func(ctx context.Context) error {
    m := cache.NewModel[User](cacheDB).WithContext(ctx)
    if _, err := m.Update(1, map[string]any{"name": "TxUser"}, nil); err != nil {
        return err
    }
    return m.Insert(&User{Name: "Bob"})
})
```

- All cache invalidations inside a transaction are **deferred until after Commit** (`AfterCommit`). This prevents other readers from seeing stale DB rows with already-invalidated cache.
- If `fn` returns an error → Rollback, **no invalidation runs**.
- If an `AfterCommit` hook panics, the panic is logged then re-raised — preserving the original Go semantics.

---

## 🪢 Concurrency Helpers

`log.Go` / `log.GoSafe` automatically:

1. Derive a **child reqId** from the parent (`req-abc → req-abc.1 → req-abc.1.1`)
2. **Recover panics** — child crashes only log, never crash the process
3. Clean up the goid → reqId map when the goroutine exits

```go
log.SetReqId("req-abc")

log.Go(func() {                       // [req-abc.1]
    log.Infow("child task")
    log.Go(func() {                   // [req-abc.1.1]
        log.Infow("grandchild task")
    })
})

// With panic callback (alerting / metrics)
log.GoSafe(func() {
    riskyWork()
}, func(err interface{}) {
    metrics.IncPanic("worker", err)
})

// Use recover anywhere
go func() {
    defer log.CatchPanic(nil)
    riskyWork()
}()
```

---

## 🔭 Observability

### Propagate req_id

```go
ctx = context.WithValue(ctx, log.ReqIDKey, "req-abc")
m := cache.NewModel[User](cacheDB).WithContext(ctx)
_, _ = m.GetByPK(1)
// → all internal logs are tagged [req-abc]
```

### Log level / output

```go
log.SetLogLevel(log.InfoLevel)   // recommended Info in production
log.SetOutput(myWriter)          // defaults to os.Stdout, redirectable
```

### Hit-rate monitoring

```go
mh := &hook.MetricsHook{}
cacheDB.UseHook(mh)
// ...
hit, total := mh.Snapshot() // atomic.Int64, concurrency-safe
```

### Sample structured log

```
DBG ... cache hit       op=GetByPK key=users:pk:1   hit_layer=l1
DBG ... cache miss      op=GetByPK key=users:pk:404 layer=redis
INF ... db fallback done op=GetByPK key=users:pk:404 elapsed_ms=12 err=
INF ... cache invalidate op=Update  table=users      pk=1 reason=update
WAR ... update no rows affected op=Update table=users pk=9999
ERR ... redis set failed key=users:pk:1 ttl=10m err="i/o timeout"
```

---

## ⚙️ Cache Strategy

### L1 (in-process)

| Type  | Recommended TTL | Field          |
|-------|-----------------|----------------|
| Value | 1 ~ 3s          | `L1ValueTTL`   |
| Null  | 300ms ~ 1s      | `L1NullTTL`    |

> Short value TTL keeps multi-instance eventual consistency tight; even shorter null TTL avoids stale "not found" results after a write.

### Redis (shared)

| Type  | TTL formula                          | Field             |
|-------|--------------------------------------|-------------------|
| Entity| `Timeout + rand(Seed)s` (jitter)     | `Timeout / Seed`  |
| List  | `ListValueTimeout + rand(Seed)s`     | `ListValueTimeout`|
| Null  | `NullValueTimeout`                   | `NullValueTimeout`|

Entity write path:

```go
ttl := b.Timeout + time.Duration(rand.Intn(b.Seed))*time.Second
b.cacheDB.Cache.Set(ctx, key, bytes, ttl)
```

---

## 🛡️ Cache Protection

| Risk | Defense |
|------|---------|
| **Breakdown** (hot key expires → DB stampede) | `singleflight` collapses concurrent loads → 1 DB hit |
| **Penetration** (querying non-existent rows) | Cache `__null__` sentinel with short TTL |
| **Avalanche** (many keys expire simultaneously) | TTL + `rand(Seed)` jitter |
| **L1 inconsistency across instances** | Short L1 TTL + Redis as source-of-truth + `Update` deletes local L1 |

---

## 🧩 Extensibility

### Router (sharding)

```go
type Router interface {
    Route(ctx context.Context, key string, table string) (db *gorm.DB, realTable string)
}
```

Built-in: `router.ModRouter` (mod 64 by id), `router.TimeRouter` (yyyymm tables).

### Cache (custom backend)

```go
type Cache interface {
    Get(ctx context.Context, key string) *redis.StringCmd
    Set(ctx context.Context, key string, val any, ttl time.Duration) (string, error)
    Del(ctx context.Context, keys ...string) *redis.IntCmd
    Incr(ctx context.Context, key string) (int64, error)
    MGet(ctx context.Context, keys ...string) ([]any, error)
}
```

### TxManager

```go
type TxManager interface {
    Begin(ctx context.Context, db *gorm.DB) (context.Context, error)
    Commit(ctx context.Context) error
    Rollback(ctx context.Context) error
    AfterCommit(ctx context.Context, fn func())
    InTx(ctx context.Context) bool
}
```

### Hooks

```go
type Hook interface {
    BeforeQuery(ctx context.Context, key string)
    AfterQuery(ctx context.Context, key string, hit bool)
    AfterUpdate(ctx context.Context, key string)
    AfterListInvalidate(ctx context.Context, table string)
}
```

Built-in: `MetricsHook` (hit/total counters), `LogHook`, `FuncHook` (closure-based).

---

## 🧪 Testing

### Unit tests (no external services)

Powered by `go-sqlmock` + `miniredis` — runs without MySQL or Redis:

```bash
go test ./... -run Unit -count=1
```

### Integration tests (require local MySQL + Redis)

```bash
go test -tags=integration ./cachex/tests/... -count=1
```

> Integration tests are gated by `//go:build integration`; the default `go test ./...` does **not** trigger them.

### Coverage

```bash
go test ./cachex/... ./log/... -coverprofile=cover.out -covermode=atomic
go tool cover -func=cover.out | tail -1
```

### Demos (visualize log output)

```bash
go test -v -run TestDemo_LogOutput        ./log/...   # 6 categories of logs
go test -v -run TestDemo_GoChildReqID     ./log/...   # parent/child reqId
go test -v -run TestDemo_GoPanicRecovery  ./log/...   # child panic does not crash
```

---

## 📊 Benchmark

| Scenario              | Latency | Role                |
|-----------------------|---------|---------------------|
| DB                    | ~60µs   | Source of truth     |
| Redis                 | ~100µs  | Load protection     |
| Cachex (L1 + Redis)   | ~0.1µs  | High-performance    |

> On localhost, Redis can appear *slower* than direct DB queries due to network round-trips, serialization, and pooling.
> Redis is **not** about reducing single-call latency — it's about **shielding the database under high concurrency** in distributed deployments.

### Cold start, 1000 concurrent requests

| Setup           | DB hits |
|-----------------|---------|
| No cache        | 1000 ❌ |
| Redis only      | ~1000 ⚠️ (all miss simultaneously) |
| **gorm-cachex** | **1** ✅ (singleflight) |

---

## ⚠️ Consistency Notes

- **L1 is an eventual-consistency layer**: across instances, expect a 1–3s window of inconsistency. Acceptable because:
  - L1 is a performance optimization; Redis is the authoritative cache; DB is the source of truth.
  - The write path deletes the local L1 + Redis PK and bumps the list version; other instances will reload naturally on TTL expiry.
- For strict consistency, disable L1: `EnableL1Cache: false`.
- Inside transactions, queries go straight to DB (no cache read/write) so you read your own writes.

---

## 🧠 Design Philosophy

- 🚀 **Performance-first** — sub-microsecond access on L1 hit
- ⚖️ **Eventual consistency by design** — explicit trade-off, never silent
- 🔌 **Pluggable everything** — Cache / Tx / Router / Hook are all replaceable
- 🧩 **Minimal GORM intrusion** — we don't wrap GORM's chain API; only the model layer
- 🔭 **Observable by default** — structured logs + hit-rate metrics out of the box

---

## ❤️ Contributing

PRs / Issues / Stars ⭐ welcome!

Before submitting, please ensure:

```bash
go build ./...
go vet ./...
go test ./... -run Unit -count=1
```

all pass. If your change touches integration tests, also run `go test -tags=integration ./cachex/tests/...`.

