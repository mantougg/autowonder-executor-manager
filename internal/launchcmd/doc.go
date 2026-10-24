// Package launchcmd 负责执行器启动命令的解析与生成：
// 把整条 `npx -y autowonder@<ver> connect …` 命令解析为结构化字段
// （ws-url、token、executor-id、provider、model 等），
// 并支持编辑结构化字段后重新生成整条命令（未知 flag 原样保留）。
// 完整实现见任务 T03。
package launchcmd
