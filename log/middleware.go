package log

import "context"

// BindCtx 把 ctx 中的 req_id 绑定到当前 goroutine。
// 推荐在每个对外入口（如 CacheDB.Transaction、CallBeforeQuery）调用一次：
//
//	release := log.BindCtx(ctx)
//	defer release()
//
// 即使 ctx 为 nil 或没有 req_id，也安全（返回 no-op release）。
func BindCtx(ctx context.Context) (release func()) {
	if ctx == nil {
		return func() {}
	}
	v := ctx.Value(ReqIDKey)
	if v == nil {
		return func() {}
	}
	id, ok := v.(string)
	if !ok || id == "" {
		return func() {}
	}
	SetReqId(id)
	return ClearReqId
}

