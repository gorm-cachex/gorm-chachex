package log

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/petermattis/goid"
)

type Level int8

const (
	DebugLevel Level = iota - 1
	InfoLevel
	WarnLevel
	ErrorLevel
	DPanicLevel
	PanicLevel
	FatalLevel
	ImportantLevel
	GORMLevel
)
const (
	colorRed = uint8(iota + 91)
	colorGreen
	colorYellow
)

var colorEnd string
var red string
var green string
var yellow string

// 请求ID相关的key
type contextKey string

var (
	// 全局请求ID存储，以协程ID为key
	reqIdMap = sync.Map{}
)

const (
	ReqIDKey contextKey = "req_id"
)

func init() {
	red = fmt.Sprintf("\x1b[%dm", colorRed)
	green = fmt.Sprintf("\x1b[%dm", colorGreen)
	yellow = fmt.Sprintf("\x1b[%dm", colorYellow)
	colorEnd = "\x1b[0m"
}

// CatchPanic 在当前 goroutine 内 recover 并打印完整栈。
//
// 用法：
//
//	go func() {
//	    defer log.CatchPanic(nil)
//	    // ... 业务代码
//	}()
//
// panicCallback 可选；若不为 nil，会在日志打印之后被调用，
// 通常用于上报告警 / 释放资源。
//
// 注：本函数使用 log.Errorf / log.Error，因此 panic 日志会自动带上当前 goroutine 的 reqId。
func CatchPanic(panicCallback func(err interface{})) {
	if err := recover(); err != nil {
		Errorf("PROCESS PANIC: err %v", err)
		st := debug.Stack()
		if len(st) > 0 {
			Errorf("dump stack (%v):", err)
			lines := strings.Split(string(st), "\n")
			for _, line := range lines {
				Error("  ", line)
			}
		} else {
			Errorf("stack is empty (%v)", err)
		}
		if panicCallback != nil {
			panicCallback(err)
		}
	}
}

// Go 启动新 goroutine 并传递 reqId。
//
// 行为：
//   - 父 goroutine 没有 reqId → 子 goroutine 也没有
//   - 父 goroutine 有 reqId="req-abc" → 子 goroutine reqId="req-abc.1"，
//     再启第二个子 → "req-abc.2"，以此类推
//   - 子 goroutine 内再调用 Go() → 孙子 reqId="req-abc.1.1"，形成树状层级
//   - 子 goroutine 退出时自动 ClearReqId，避免 goid 复用串号
//   - 子 goroutine 内若 panic：自动 recover，打印完整栈与 reqId，**不会** 拖垮进程
//
// Example:
//
//	log.SetReqId("req-abc")
//	log.Go(func() {
//	    log.Infow("child task")           // [req-abc.1]
//	    log.Go(func() {
//	        log.Infow("grandchild task")  // [req-abc.1.1]
//	    })
//	})
func Go(fn func()) {
	GoSafe(fn, nil)
}

// GoSafe 与 Go 行为一致，但允许传入 panic 回调（如告警 / metrics 上报）。
//
// 回调在日志打印 **之后** 执行，不会再次 panic 出去。
func GoSafe(fn func(), onPanic func(err interface{})) {
	parent := GetReqId()
	var child string
	if parent != "" {
		child = nextChildReqID(parent)
	}
	go func() {
		// defer 倒序执行：
		//   1) CatchPanic 最先 recover → 此时 reqId 仍在，panic 日志可关联请求
		//   2) ClearReqId 最后清理 goid 映射
		if child != "" {
			SetReqId(child)
			defer ClearReqId()
		}
		defer CatchPanic(onPanic)
		fn()
	}()
}

// childCounters: parentReqID -> *atomic.Int64，给每个 parent 分配自增子序号。
//
// 注意：map 条目随 reqId 字面量保留；在常见请求级生命周期内（每请求一个 reqId）
// 占用极小，无需 GC。如需在长运行进程中清理，可在请求结束处调用 ResetChildCounter。
var childCounters sync.Map

func nextChildReqID(parent string) string {
	v, _ := childCounters.LoadOrStore(parent, new(atomic.Int64))
	n := v.(*atomic.Int64).Add(1)
	return parent + "." + strconv.FormatInt(n, 10)
}

// ResetChildCounter 清理某个 parent reqId 的子计数器。可在请求结束时调用，避免长期累积。
// 不调用也不会影响功能（仅占少量内存）。
func ResetChildCounter(reqId string) {
	childCounters.Delete(reqId)
}
func SetReqId(reqId string) {
	routineID := goid.Get()
	reqIdMap.Store(routineID, reqId)
}

func GetReqId() string {
	routineID := goid.Get()
	if v, ok := reqIdMap.Load(routineID); ok {
		if s, ok2 := v.(string); ok2 {
			return s
		}
	}
	return ""
}

func ClearReqId() {
	routineID := goid.Get()
	reqIdMap.Delete(routineID)
}

func SetReqIdFromContext(ctx context.Context) {
	if ctx != nil {
		if requestID := ctx.Value(ReqIDKey); requestID != nil {
			if id, ok := requestID.(string); ok {
				SetReqId(id)
			}
		}
	}
}

func (l Level) String() string {
	switch l {
	case DebugLevel:
		return "debug"
	case InfoLevel:
		return "info"
	case WarnLevel:
		return "warn"
	case ErrorLevel:
		return "error"
	case DPanicLevel:
		return "dpanic"
	case PanicLevel:
		return "panic"
	case FatalLevel:
		return "fatal"
	case ImportantLevel:
		return "important"
	default:
		return fmt.Sprintf("Level(%d)", l)
	}
}

func (l Level) ShortString() string {
	switch l {
	case DebugLevel:
		return "DBG "
	case InfoLevel:
		return "INF "
	case WarnLevel:
		return "WAR "
	case ErrorLevel:
		return "ERR "
	case DPanicLevel:
		return "PAN "
	case PanicLevel:
		return "PAN "
	case FatalLevel:
		return "FAT "
	case ImportantLevel:
		return "IMP "
	default:
		return fmt.Sprintf("L(%d) ", l)
	}
}

func (l Level) Color() string {
	switch l {
	case DebugLevel, InfoLevel, ImportantLevel:
		return green
	case WarnLevel:
		return yellow
	default:
		return red
	}
}

// levelBits 保存当前日志级别。日志读、SetLogLevel 写，故用原子避免数据竞争。
var levelBits atomic.Int32

func init() {
	pid = os.Getpid()
	levelBits.Store(int32(DebugLevel))
}

func SetLogLevel(l Level) {
	levelBits.Store(int32(l))
}

// GetLogLevel 返回当前日志级别。
func GetLogLevel() Level {
	return Level(levelBits.Load())
}

var pid = 0

// timeCache 把"秒级时间戳 -> 格式化字符串"一起原子替换，
// 既保留每秒只 Format 一次的快路径，又消除并发读写的数据竞争。
type timeCache struct {
	sec uint32
	str string
}

var formatTimeCache atomic.Pointer[timeCache]

// bufPool 复用 formatLog 的 bytes.Buffer，降低每条日志的分配。
var bufPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

var (
	outputMu sync.RWMutex
	output   io.Writer = os.Stdout
)

// SetOutput 用于测试或自定义日志去向（默认 os.Stdout）。并发安全。
func SetOutput(w io.Writer) {
	outputMu.Lock()
	defer outputMu.Unlock()
	if w == nil {
		output = os.Stdout
		return
	}
	output = w
}

func currentOutput() io.Writer {
	outputMu.RLock()
	defer outputMu.RUnlock()
	return output
}

func formatTime(t time.Time) string {
	sec := uint32(t.Unix())
	if c := formatTimeCache.Load(); c != nil && c.sec == sec {
		return c.str
	}
	str := t.Format("01-02T15:04:05")
	formatTimeCache.Store(&timeCache{sec: sec, str: str})
	return str
}

// writeInt 把整数以十进制追加到 buf，避免 fmt.Sprintf 的分配。
func writeInt(b *bytes.Buffer, n int64) {
	var tmp [20]byte
	b.Write(strconv.AppendInt(tmp[:0], n, 10))
}

func formatLog(l Level, buf string, callerSkip int) string {
	return formatLogReq(l, GetReqId(), buf, callerSkip)
}

// formatLogReq 与 formatLog 一致，但由调用方直接提供 reqId，
// 使 context 路径无需再走 goid.Get()+map 查询。
func formatLogReq(l Level, reqId string, buf string, callerSkip int) string {
	now := time.Now()

	b := bufPool.Get().(*bytes.Buffer)
	b.Reset()
	defer bufPool.Put(b)

	routineId := goid.Get()

	// 进程、协程、请求ID
	b.WriteByte('(')
	writeInt(b, int64(pid))
	b.WriteByte(',')
	writeInt(b, routineId)
	b.WriteByte(')')
	if reqId != "" {
		b.WriteString(" [")
		b.WriteString(reqId)
		b.WriteByte(']')
	}
	b.WriteByte(' ')

	// 时间
	b.WriteString(formatTime(now))
	frac := now.Nanosecond() / 100000 // 0..9999
	b.WriteByte('.')
	b.WriteByte(byte('0' + frac/1000%10))
	b.WriteByte(byte('0' + frac/100%10))
	b.WriteByte(byte('0' + frac/10%10))
	b.WriteByte(byte('0' + frac%10))
	b.WriteByte(' ')

	// 日志级别
	b.WriteString(l.Color())
	b.WriteString(l.ShortString())

	var callerName, callerFile string
	var callerLine int
	var ok bool
	var pc uintptr
	pc, callerFile, callerLine, ok = runtime.Caller(callerSkip)
	callerName = ""
	if ok {
		callerName = runtime.FuncForPC(pc).Name()
	}
	// 调用位置
	filePath, fileFunc := getPackageName(callerName)
	b.WriteString(path.Join(filePath, path.Base(callerFile)))
	b.WriteByte(':')
	writeInt(b, int64(callerLine))
	b.WriteByte(':')
	b.WriteString(fileFunc)
	b.WriteString(colorEnd)
	b.WriteByte(' ')

	// 文本内容
	b.WriteString(buf)
	b.WriteByte('\n')

	return b.String()
}

func getPackageName(f string) (string, string) {
	slashIndex := strings.LastIndex(f, "/")
	if slashIndex > 0 {
		idx := strings.Index(f[slashIndex:], ".") + slashIndex
		return f[:idx], f[idx+1:]
	}
	return f, ""
}

func PrintStack(skip int) {
	for ; ; skip++ {
		pc, file, line, ok := runtime.Caller(skip)
		if !ok {
			break
		}
		name := runtime.FuncForPC(pc)
		if name.Name() == "runtime.goexit" {
			break
		}
		Errorf("#STACK: %s %s:%d", name.Name(), file, line)
	}
}

func logIt(l Level, msg string) {
	if l < GetLogLevel() {
		return
	}

	msg = formatLog(l, msg, 4)
	fmt.Fprint(currentOutput(), msg)
}

// logItSkip 与 logIt 行为一致，但允许调用方显式指定 caller skip。
// 主要供 *w 系列结构化日志函数使用，因为它们的调用栈比旧 API 浅一层。
func logItSkip(l Level, msg string, skip int) {
	if l < GetLogLevel() {
		return
	}
	msg = formatLog(l, msg, skip)
	fmt.Fprint(currentOutput(), msg)
}

// logItSkipReq 与 logItSkip 一致，但由调用方直接提供 reqId（供 context API 使用），
// 避免再走一次 goid.Get()+map 查询。
func logItSkipReq(l Level, reqId, msg string, skip int) {
	if l < GetLogLevel() {
		return
	}
	msg = formatLogReq(l, reqId, msg, skip)
	fmt.Fprint(currentOutput(), msg)
}

func afterLog(l Level) {
	if l == FatalLevel {
		PrintStack(4)
	}
	if l == FatalLevel {
		os.Exit(1)
	}
}

// sprintfTemplate 复刻旧 logItFmt 的消息构造语义：
// 空模板 + 有参数 → Sprint；非空模板 + 有参数 → Sprintf；否则原样返回模板。
func sprintfTemplate(template string, args ...interface{}) string {
	msg := template
	if msg == "" && len(args) > 0 {
		msg = fmt.Sprint(args...)
	} else if msg != "" && len(args) > 0 {
		msg = fmt.Sprintf(template, args...)
	}
	return msg
}

func logItFmt(l Level, template string, args ...interface{}) {
	logIt(l, sprintfTemplate(template, args...))
	afterLog(l)
}

func logItArgs(l Level, args ...interface{}) {
	msg := fmt.Sprint(args...)
	logIt(l, msg)
	afterLog(l)
}

func Infof(template string, args ...interface{}) {
	logItFmt(InfoLevel, template, args...)
}

func Fatal(args ...interface{}) {
	logItArgs(FatalLevel, args...)
}

func Error(args ...interface{}) {
	logItArgs(ErrorLevel, args...)
}

func Warn(args ...interface{}) {
	logItArgs(WarnLevel, args...)
}

func Info(args ...interface{}) {
	logItArgs(InfoLevel, args...)
}

func Debug(args ...interface{}) {
	// fast check
	if DebugLevel < GetLogLevel() {
		return
	}
	logItArgs(DebugLevel, args...)
}

func Debugf(template string, args ...interface{}) {
	// fast check
	if DebugLevel < GetLogLevel() {
		return
	}
	logItFmt(DebugLevel, template, args...)
}

func Warnf(template string, args ...interface{}) {
	logItFmt(WarnLevel, template, args...)
}

func Errorf(template string, args ...interface{}) {
	logItFmt(ErrorLevel, template, args...)
}

func Fatalf(template string, args ...interface{}) {
	logItFmt(FatalLevel, template, args...)
}

func Must(err error) {
	if err == nil {
		return
	}
	msg := fmt.Sprintf("%+v\n\n", err.Error())
	Fatal(msg)
}
