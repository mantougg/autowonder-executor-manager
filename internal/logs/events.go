// events.go：管理事件日志（T04-F4）。data/logs/events.log 记录启停/崩溃/
// 重启/配置加载与变更/服务安装/日志清理等管理事件。
// 格式选型 JSONL：一行一事件（时间/类型/执行器/详情），程序可解析、
// 人可 grep，追加写天然适配滚动；滚动复用 LogSink（与执行器日志共用参数，
// 取简单方案——事件日志体量小，同一套 max/keep 足够）。
package logs

import (
	"encoding/json"
	"time"
)

// EventType 是管理事件类型（§11 场景枚举）。
type EventType string

const (
	EventDaemonStart        EventType = "daemon_start"         // daemon 启动
	EventDaemonStop         EventType = "daemon_stop"          // daemon 退出
	EventExecutorStart      EventType = "executor_start"       // 执行器启动
	EventExecutorStop       EventType = "executor_stop"        // 执行器停止（人为）
	EventExecutorCrash      EventType = "executor_crash"       // 执行器崩溃退出
	EventExecutorRestart    EventType = "executor_restart"     // 执行器自动拉起
	EventExecutorSpawnFail  EventType = "executor_spawn_fail"  // 执行器启动失败（npx/spawn/日志，T05-F1）
	EventConfigLoaded       EventType = "config_loaded"        // 配置加载成功
	EventConfigReloadFailed EventType = "config_reload_failed" // 配置热加载失败（旧配置继续生效）
	EventConfigChanged      EventType = "config_changed"       // 配置变更生效
	EventServiceInstalled   EventType = "service_installed"    // 系统服务安装/卸载
	EventLogCleanup         EventType = "log_cleanup"          // 保留策略清理
	EventPlatformSync       EventType = "platform_sync"        // 平台联动事件（跳过/不可达/恢复，T08）
)

// Event 是一条管理事件（JSONL 一行）。
type Event struct {
	Time      time.Time `json:"time"`                 // 事件时间
	Type      EventType `json:"type"`                 // 事件类型
	Executor  string    `json:"executor,omitempty"`   // 相关执行器（可空）
	Detail    string    `json:"detail,omitempty"`     // 详情（可空）
	Restart   *bool     `json:"restart,omitempty"`    // executor_crash：是否将自动拉起（nil=旧事件，按 detail 兼容）
	LogFile   string    `json:"log_file,omitempty"`   // executor_crash：日志文件标识（该执行器日志链内，如 name.log.2）
	LogOffset int64     `json:"log_offset,omitempty"` // executor_crash：崩溃时日志文件字节偏移（定位用）
}

// EventLog 是事件日志写入器（内部复用 LogSink，天然线程安全 + 滚动）。
type EventLog struct {
	sink *LogSink
}

// OpenEventLog 打开事件日志（不存在则创建）。maxBytes/keep 为滚动参数。
func OpenEventLog(path string, maxBytes int64, keep int) (*EventLog, error) {
	sink, err := OpenSink(path, maxBytes, keep)
	if err != nil {
		return nil, err
	}
	return &EventLog{sink: sink}, nil
}

// Record 追加一条事件（JSONL）。
func (e *EventLog) Record(typ EventType, executor, detail string) error {
	return e.RecordEvent(Event{Type: typ, Executor: executor, Detail: detail})
}

// RecordEvent 追加一条完整事件（Time 为零时填当前时刻）。
// 增量字段（restart/log_file/log_offset）只增不改，旧读取方忽略即可。
func (e *EventLog) RecordEvent(ev Event) error {
	if ev.Time.IsZero() {
		ev.Time = time.Now()
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = e.sink.Write(data)
	return err
}

// Close 关闭事件日志。
func (e *EventLog) Close() error { return e.sink.Close() }

// MaskSecret 掩码敏感令牌（conventions §5：日志不回显完整敏感令牌）。
// 保留前 4 后 4 位便于人工比对，长度不足 9 时只保留前 2 位。
func MaskSecret(s string) string {
	runes := []rune(s)
	switch {
	case len(runes) <= 4:
		return "***"
	case len(runes) < 9:
		return string(runes[:2]) + "***"
	default:
		return string(runes[:4]) + "***" + string(runes[len(runes)-4:])
	}
}
