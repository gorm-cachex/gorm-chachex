package hook

import (
	"cachex/log"
	"context"
)

type LogHook struct{}

func (l *LogHook) BeforeQuery(ctx context.Context, key string) {
	log.Info("query start", " key:", key)
}

func (l *LogHook) AfterQuery(ctx context.Context, key string, hit bool) {
	log.Info("query done", " key:", key, " hit:", hit)
}

func (l *LogHook) AfterUpdate(ctx context.Context, key string) {
	log.Info("cache invalidated", " key:", key)
}

func (l *LogHook) AfterListInvalidate(ctx context.Context, table string) {
	log.Info("list cache invalidated", " table:", table)
}
