# gorm-cachex

**🌐 Language**: **简体中文** | [English](./README_EN.md)

> 面向 GORM 的高性能缓存扩展：**L1 + Redis + singleflight**，自带穿透 / 击穿 / 雪崩防御与可观测性。

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

- 🚀 **L1 in-memory cache**（纳秒级）
- ⚡ **Redis L2 cache**（共享 / 抗高并发）
- 🛡️ **singleflight** 防缓存击穿
- ❄️ **Null cache** 防穿透
- 🌪️ **TTL jitter** 防雪崩
- 🔁 **Auto invalidation**（事务 commit 后失效）
- 📦 **List cache with version**（分页缓存 + 版本号失效）
- 🔀 **Sharding router**（分库分表可插拔）
- 🔌 **Pluggable** Router / Cache / TxManager / Hook
- 🔭 **Structured logs**（`op / key / hit_layer / elapsed_ms / req_id`）
- 🪢 **Goroutine helpers**：`log.Go / GoSafe` 自动透传 reqId + 内建 panic recover

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

写路径：

```
  Update / Insert
        ↓
   DB write
        ↓
  Tx Commit ────► AfterCommit ────► Del PK / L1 / Bump list version
                                      （未提交则不失效，避免脏读）
```

---

## 📦 Install

```bash
go get github.com/<your-org>/gorm-cachex
```

依赖：`gorm.io/gorm`、`github.com/redis/go-redis/v9`、`golang.org/x/sync/singleflight`。

---

## 🚀 Quick Start

### 初始化

```go
import (
    "cachex/cachex/cache"
    "cachex/cachex/tx"
)

cacheDB := cache.NewCacheDb(&cache.CacheDB{
    DB:        db,                              // *gorm.DB
    Cache:     &cache.RedisCache{Client: rdb},  // L2
    TxManager: &tx.DefaultTxManager{},          // 事务
    // Router: myRouter,                        // 可选：分库分表

    // Redis TTL
    Timeout:          time.Minute * 10,
    Seed:             500,                       // 抖动窗口（秒）
    NullValueTimeout: time.Minute * 5,
    ListValueTimeout: time.Minute * 10,

    // L1 cache
    EnableL1Cache: true,
    L1ValueTTL:    2 * time.Second,
    L1NullTTL:     500 * time.Millisecond,
})
```

### 定义模型

`User` 必须实现 `cache.PK` 接口：

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

// Insert：写库 + 失效列表缓存（commit 后）
_ = m.Insert(&User{Name: "Alice"})

// 主键查询：L1 → Redis → singleflight → DB
u, err := m.GetByPK(int64(1))

// 唯一键查询：UK → PK → Entity（复用 PK 缓存）
u2, err := m.GetByUnique("email", "alice@example.com")

// 等值组合条件查询（仅等值条件可缓存）
u3, err := m.GetByConditions([]cache.CacheCondition{
    {Field: "status", Op: cache.OpEq, Value: "active"},
})

// 分页列表：缓存 PK 数组 + 批量加载实体
list, total, err := m.GetList(
    []cache.CacheCondition{{Field: "status", Op: cache.OpEq, Value: "active"}},
    /*page*/ 1, /*size*/ 10, /*order*/ nil,
)

// 更新：失效 L1 + Redis PK + 列表 version
_, _ = m.Update(int64(1), map[string]any{"name": "ggboy"}, nil)

// 乐观锁更新
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

- 事务内的所有 **缓存失效都被推迟到 Commit 之后**（`AfterCommit`），避免提交前别的请求读到旧 DB + 已失效缓存。
- `fn` 返回 error → Rollback，**不触发任何缓存失效**。
- `AfterCommit` hook 内若 panic：先打日志再向上传播，行为与原生一致。

---

## 🪢 Concurrency Helpers

`log.Go` / `log.GoSafe` 让你启动子 goroutine 时自动：

1. 把当前 reqId 派生成 **子 reqId**（`req-abc → req-abc.1 → req-abc.1.1`）
2. **内建 panic recover**：子协程崩溃只打日志不拖垮进程
3. 子协程退出时清理 goid 映射

```go
log.SetReqId("req-abc")

log.Go(func() {                       // [req-abc.1]
    log.Infow("child task")
    log.Go(func() {                   // [req-abc.1.1]
        log.Infow("grandchild task")
    })
})

// 带告警回调
log.GoSafe(func() {
    riskyWork()
}, func(err interface{}) {
    metrics.IncPanic("worker", err)
})

// 任意位置使用 recover
go func() {
    defer log.CatchPanic(nil)
    riskyWork()
}()
```

---

## 🔭 Observability

### req_id 透传

```go
ctx = context.WithValue(ctx, log.ReqIDKey, "req-abc")
m := cache.NewModel[User](cacheDB).WithContext(ctx)
_, _ = m.GetByPK(1)
// → 所有内部日志自动带 [req-abc]
```

### 日志级别 / 输出

```go
log.SetLogLevel(log.InfoLevel)   // 生产建议 Info
log.SetOutput(myWriter)          // 默认 os.Stdout，可重定向
```

### 缓存命中率监控

```go
mh := &hook.MetricsHook{}
cacheDB.UseHook(mh)
// ...
hit, total := mh.Snapshot() // atomic.Int64，并发安全
```

### 结构化日志样例

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

### L1（本地）

| 类型 | 推荐 TTL | 字段 |
|------|---------|------|
| 有值 | 1 ~ 3s  | `L1ValueTTL` |
| 空值 | 300ms ~ 1s | `L1NullTTL` |

> Value TTL 短：保证多实例最终一致；Null TTL 更短：避免数据已插入但仍返回 not found。

### Redis（共享）

| 类型 | TTL 公式 | 字段 |
|------|----------|------|
| 实体 | `Timeout + rand(Seed)s`（抖动） | `Timeout / Seed` |
| 列表 | `ListValueTimeout + rand(Seed)s` | `ListValueTimeout` |
| 空值 | `NullValueTimeout` | `NullValueTimeout` |

代码片段（实体写入路径）：

```go
ttl := b.Timeout + time.Duration(rand.Intn(b.Seed))*time.Second
b.cacheDB.Cache.Set(ctx, key, bytes, ttl)
```

---

## 🛡️ Cache Protection

| 风险 | 防御 |
|------|------|
| **击穿**（热 key 过期 → DB 雪崩） | `singleflight` 合并并发，1 次 DB |
| **穿透**（查不存在的数据） | 写入 `__null__` 哨兵，短 TTL |
| **雪崩**（大量 key 同时过期） | TTL + `rand(Seed)` 抖动 |
| **多实例 L1 不一致** | 短 L1 TTL + Redis 权威 + `Update` 同步 Del L1 |

---

## 🧩 Extensibility

### Router（分库分表）

```go
type Router interface {
    Route(ctx context.Context, key string, table string) (db *gorm.DB, realTable string)
}
```

内置 `router.ModRouter`（按 ID 模 64）、`router.TimeRouter`（按 yyyymm 分表）。

### Cache（自定义后端）

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

内置：`MetricsHook`（hit/total）、`LogHook`、`FuncHook`（按需注入闭包）。

---

## 🧪 Testing

### Unit Tests（无外部依赖）

依赖 `go-sqlmock` + `miniredis`，零 MySQL / Redis 即可运行：

```bash
go test ./... -run Unit -count=1
```

### Integration Tests（需要本地 MySQL + Redis）

```bash
go test -tags=integration ./cachex/tests/... -count=1
```

> 集成测试已用 `//go:build integration` 标记隔离，默认 `go test ./...` **不会** 触发。

### Coverage

```bash
go test ./cachex/... ./log/... -coverprofile=cover.out -covermode=atomic
go tool cover -func=cover.out | tail -1
```

### Demo（人眼观察日志格式）

```bash
go test -v -run TestDemo_LogOutput        ./log/...   # 6 大类日志样例
go test -v -run TestDemo_GoChildReqID     ./log/...   # 父子协程 reqId
go test -v -run TestDemo_GoPanicRecovery  ./log/...   # 子协程 panic 不崩溃
```

---

## 📊 Benchmark

| Scenario              | Latency | Role                |
|-----------------------|---------|---------------------|
| DB                    | ~60µs   | Source of truth     |
| Redis                 | ~100µs  | Load protection     |
| Cachex (L1 + Redis)   | ~0.1µs  | High-performance    |

> 本地基准 Redis 可能比直连 DB 慢，原因：网络往返、序列化、连接池。
> Redis 的价值不是降低单次延迟，而是 **抗高并发保护 DB**。
> 在分布式 / 多实例场景下，Redis + L1 是必备。

### 冷启动 1000 并发

| 方案 | DB hits |
|------|---------|
| 无缓存 | 1000 ❌ |
| Redis only | ~1000 ⚠️（同时 miss） |
| **gorm-cachex** | **1** ✅（singleflight） |

---

## ⚠️ Consistency Notes

- **L1 是最终一致性层**：多实例下 1~3 秒可能不一致。可接受的前提：
  - L1 是性能优化层；Redis 是权威缓存；DB 是真源。
  - 写路径会 Del 本进程 L1 + Redis PK + Bump list version，新进程读会自然回源。
- 强一致需求请关闭 L1：`EnableL1Cache: false`。
- 事务内的查询走 DB（不读不写缓存），保证读自己写过的数据。

---

## 🧠 Design Philosophy

- 🚀 **Performance-first**：单次访问 ≤ 1µs（L1 命中）
- ⚖️ **Eventual consistency by design**：性能与一致性的清晰权衡
- 🔌 **Pluggable everything**：Cache / Tx / Router / Hook 都可替换
- 🧩 **Minimal GORM intrusion**：不接管 GORM 的链式 API，仅在模型层封装
- 🔭 **Observable by default**：结构化日志 + 命中率指标开箱即用

---

## ❤️ Contributing

欢迎 PR / Issue / Star ⭐

提交前请确保：

```bash
go build ./...
go vet ./...
go test ./... -run Unit -count=1
```

均为绿色。集成测试如修改请同时运行 `go test -tags=integration ./cachex/tests/...`。

