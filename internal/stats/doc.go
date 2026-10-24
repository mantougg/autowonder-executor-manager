// Package stats 重放本机管理事件日志（events.log 及其滚动链），生成执行器
// 运行状态区间、崩溃点与数据起点，供执行器详情页运行时间带与二期图表共用。
//
// 纯函数实现：不读文件、不发起网络请求；滚动链读取由 internal/logs 提供，
// HTTP 端点由 internal/api 组装。状态语义见 docs/design/design.md §9。
package stats
