// clock.go：可注入时钟（T05）。守护状态机的全部时间获取与退避等待都走
// Clock，测试可注入替身；默认系统时钟。实测策略：单测注入"快参数 +
// 真实时钟"（毫秒级退避），等价于快进验证且更贴近真实并发行为。
package supervisor

import "time"

// Clock 抽象时间源。
type Clock interface {
	Now() time.Time                         // 当前时间（uptime/稳定期判定）
	After(d time.Duration) <-chan time.Time // 退避等待（d<=0 立即返回）
}

// RealClock 是系统时钟实现。
type RealClock struct{}

// Now 返回系统当前时间。
func (RealClock) Now() time.Time { return time.Now() }

// After 返回 d 之后到达一次的通道（即 time.After）。
func (RealClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
