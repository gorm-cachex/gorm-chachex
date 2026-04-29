package log

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm/logger"
)

var (
	traceStr     = "[%.3fms] [rows:%v] %s"
	traceWarnStr = "%s [%.3fms] [rows:%v] %s"
	traceErrStr  = "%s [%.3fms] [rows:%v] %s"
)

// GormLogger 封装gorm.logger以适应公共库的log风格
type GormLogger struct {
	LogLevel                  logger.LogLevel
	SlowThreshold             time.Duration
	IgnoreRecordNotFoundError bool
	logger.Interface
}

// LogMode log mode
func (l *GormLogger) LogMode(level logger.LogLevel) logger.Interface {
	newlogger := GormLogger{}
	newlogger.LogLevel = level
	return &newlogger
}

// Info print info
func (l *GormLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if l.LogLevel >= logger.Info {
		Infof(msg, data...)
	}
}

// Warn print warn messages
func (l *GormLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if l.LogLevel >= logger.Warn {
		Warnf(msg, data...)
	}
}

// Error print error messages
func (l *GormLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if l.LogLevel >= logger.Error {
		Errorf(msg, data...)
	}
}

func (l *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.LogLevel <= logger.Silent {
		return
	}

	elapsed := time.Since(begin)
	switch {
	case err != nil && l.LogLevel >= logger.Error && (!errors.Is(err, logger.ErrRecordNotFound) || !l.IgnoreRecordNotFoundError):
		sql, rows := fc()
		if rows == -1 {
			Errorf(traceErrStr, err, float64(elapsed.Nanoseconds())/1e6, "-", sql)
		} else {
			Errorf(traceErrStr, err, float64(elapsed.Nanoseconds())/1e6, rows, sql)
		}
	case elapsed > l.SlowThreshold && l.SlowThreshold != 0 && l.LogLevel >= logger.Warn:
		sql, rows := fc()
		slowLog := fmt.Sprintf("SLOW SQL >= %v", l.SlowThreshold)
		if rows == -1 {
			Warnf(traceWarnStr, slowLog, float64(elapsed.Nanoseconds())/1e6, "-", sql)
		} else {
			Warnf(traceWarnStr, slowLog, float64(elapsed.Nanoseconds())/1e6, rows, sql)
		}
	case l.LogLevel == logger.Info:
		sql, rows := fc()
		if rows == -1 {
			Infof(traceStr, float64(elapsed.Nanoseconds())/1e6, "-", sql)
		} else {
			Infof(traceStr, float64(elapsed.Nanoseconds())/1e6, rows, sql)
		}
	}
}
