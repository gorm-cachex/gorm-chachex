package log

import "context"

// context 显式日志 API：直接从 ctx 中取 reqId，绕开 goid + sync.Map 的隐式 GLS 查询。
//
// 何时用哪个：
//   - 已经在手上持有 ctx（HTTP handler、gRPC、gorm 回调等）→ 优先用 *Ctx 系列，
//     无需依赖 goid，也不受协程复用 / 漏 ClearReqId 影响，热路径少一次 map 查询。
//   - 深层工具函数、不方便层层传 ctx → 继续用 GLS 的 Infof/Infow 等旧 API。
//
// ctx 中没有 reqId（或 ctx 为 nil）时，自动回退到当前 goroutine 的 GLS reqId，
// 因此与旧 API 混用也不会丢失请求关联。
//
// *Ctx 与 *fCtx / *wCtx 的调用栈深度与 *w 系列一致，故沿用 wCallerSkip。

// reqIdFromCtx 优先返回 ctx 内的 reqId，缺失时回退 GLS。
func reqIdFromCtx(ctx context.Context) string {
	if ctx != nil {
		if v := ctx.Value(ReqIDKey); v != nil {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return GetReqId()
}

// —— printf 风格 ——

func DebugfCtx(ctx context.Context, template string, args ...interface{}) {
	if DebugLevel < GetLogLevel() {
		return
	}
	logItSkipReq(DebugLevel, reqIdFromCtx(ctx), sprintfTemplate(template, args...), wCallerSkip)
}

func InfofCtx(ctx context.Context, template string, args ...interface{}) {
	logItSkipReq(InfoLevel, reqIdFromCtx(ctx), sprintfTemplate(template, args...), wCallerSkip)
}

func WarnfCtx(ctx context.Context, template string, args ...interface{}) {
	logItSkipReq(WarnLevel, reqIdFromCtx(ctx), sprintfTemplate(template, args...), wCallerSkip)
}

func ErrorfCtx(ctx context.Context, template string, args ...interface{}) {
	logItSkipReq(ErrorLevel, reqIdFromCtx(ctx), sprintfTemplate(template, args...), wCallerSkip)
}

// —— 结构化（消息 + 键值对）风格 ——

func DebugwCtx(ctx context.Context, msg string, kv ...any) {
	if DebugLevel < GetLogLevel() {
		return
	}
	logItSkipReq(DebugLevel, reqIdFromCtx(ctx), msg+formatKV(kv), wCallerSkip)
}

func InfowCtx(ctx context.Context, msg string, kv ...any) {
	logItSkipReq(InfoLevel, reqIdFromCtx(ctx), msg+formatKV(kv), wCallerSkip)
}

func WarnwCtx(ctx context.Context, msg string, kv ...any) {
	logItSkipReq(WarnLevel, reqIdFromCtx(ctx), msg+formatKV(kv), wCallerSkip)
}

func ErrorwCtx(ctx context.Context, msg string, kv ...any) {
	logItSkipReq(ErrorLevel, reqIdFromCtx(ctx), msg+formatKV(kv), wCallerSkip)
}
