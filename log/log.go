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

var level = DebugLevel

func init() {
	pid = os.Getpid()
}

func SetLogLevel(l Level) {
	level = l
}

// GetLogLevel 返回当前日志级别（测试用）。
func GetLogLevel() Level {
	return level
}

var pid = 0
var formatTimeSec uint32
var formatTimeSecStr string

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
	pre := formatTimeSec
	preStr := formatTimeSecStr
	if pre == sec {
		// 受并行优化的影响，小概率取了旧值，因为是打LOG，就不搞这么严谨了
		return preStr
	}
	x := t.Format("01-02T15:04:05")
	formatTimeSec = sec
	formatTimeSecStr = x
	return x
}

func formatLog(l Level, buf string, callerSkip int) string {
	now := time.Now()

	var b bytes.Buffer
	routineId := goid.Get()
	// 获取请求ID
	reqId := GetReqId()

	// 进程、协程、请求ID
	if reqId != "" {
		b.WriteString(fmt.Sprintf("(%d,%d) [%s] ", pid, routineId, reqId))
	} else {
		b.WriteString(fmt.Sprintf("(%d,%d) ", pid, routineId))
	}
	// 时间
	b.WriteString(formatTime(now))
	b.WriteString(fmt.Sprintf(".%04d ", now.Nanosecond()/100000))

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
	b.WriteString(":")
	b.WriteString(fmt.Sprintf("%d:", callerLine))
	b.WriteString(fileFunc)
	b.WriteString(colorEnd)
	b.WriteString(" ")

	// 文本内容
	b.WriteString(buf)
	b.WriteString("\n")

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
	if l < level {
		return
	}

	msg = formatLog(l, msg, 4)
	fmt.Fprint(currentOutput(), msg)
}

// logItSkip 与 logIt 行为一致，但允许调用方显式指定 caller skip。
// 主要供 *w 系列结构化日志函数使用，因为它们的调用栈比旧 API 浅一层。
func logItSkip(l Level, msg string, skip int) {
	if l < level {
		return
	}
	msg = formatLog(l, msg, skip)
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

func logItFmt(l Level, template string, args ...interface{}) {
	msg := template
	if msg == "" && len(args) > 0 {
		msg = fmt.Sprint(args...)
	} else if msg != "" && len(args) > 0 {
		msg = fmt.Sprintf(template, args...)
	}
	logIt(l, msg)
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
	if DebugLevel < level {
		return
	}
	logItArgs(DebugLevel, args...)
}

func Debugf(template string, args ...interface{}) {
	// fast check
	if DebugLevel < level {
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
