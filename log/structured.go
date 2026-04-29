package log
// 结构化日志快捷函数：消息 + 键值对。
// 例: log.Infow("get by pk", "key", "users:pk:1", "hit_layer", "redis")
//
// 实现说明：*w 系列调用栈比旧 API 浅一层（少了 logItArgs/logItFmt 中转），
// 因此使用 logItSkip(skip=3) 让 caller 定位到调用 *w 的真实业务代码。
const wCallerSkip = 3
func Debugw(msg string, kv ...any) {
	if DebugLevel < level {
		return
	}
	logItSkip(DebugLevel, msg+formatKV(kv), wCallerSkip)
	afterLog(DebugLevel)
}
func Infow(msg string, kv ...any) {
	logItSkip(InfoLevel, msg+formatKV(kv), wCallerSkip)
	afterLog(InfoLevel)
}
func Warnw(msg string, kv ...any) {
	logItSkip(WarnLevel, msg+formatKV(kv), wCallerSkip)
	afterLog(WarnLevel)
}
func Errorw(msg string, kv ...any) {
	logItSkip(ErrorLevel, msg+formatKV(kv), wCallerSkip)
	afterLog(ErrorLevel)
}
