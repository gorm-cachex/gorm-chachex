package hook
import (
	"context"
	"sync/atomic"
)
// MetricsHook 通过原子计数收集缓存命中率指标，多 goroutine 安全。
type MetricsHook struct {
	hit   atomic.Int64
	total atomic.Int64
}
func (m *MetricsHook) BeforeQuery(ctx context.Context, key string) {}
func (m *MetricsHook) AfterQuery(ctx context.Context, key string, hit bool) {
	m.total.Add(1)
	if hit {
		m.hit.Add(1)
	}
}
func (m *MetricsHook) AfterUpdate(ctx context.Context, key string) {}
// AfterListInvalidate 实现 hook.Hook 接口；参数语义为 table 而非 key。
func (m *MetricsHook) AfterListInvalidate(ctx context.Context, table string) {}
// Snapshot 返回当前命中数与总数，便于测试与监控读取。
func (m *MetricsHook) Snapshot() (hit, total int64) {
	return m.hit.Load(), m.total.Load()
}
