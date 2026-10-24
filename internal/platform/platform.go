// platform.go：平台状态联动的包说明与禁用实现（T06 由 daemon 装配；
// T08 起完整实现见 syncer.go/discovery.go/poller.go/drift.go）。
// 接口与数据结构定义在 syncer.go（Syncer 接口、Status、DriftField）。
package platform

// Noop 是联动禁用实现：Lookup 恒返回未命中（未配置 [platform] 段时
// daemon 装配它，本地功能完全不受影响）。
type Noop struct{}

// Lookup 实现 Syncer：恒返回未匹配。
func (Noop) Lookup(string) (Status, bool) { return Status{}, false }
