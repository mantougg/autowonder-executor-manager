// Package supervisor 负责执行器子进程守护：按配置 spawn
// `npx -y autowonder@<ver> connect …`、整进程树停止、崩溃自动拉起
// （指数退避）、运行状态查询。完整实现见任务 T05。
package supervisor
