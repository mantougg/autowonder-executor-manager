// Package launchcmd 负责执行器启动命令的解析、校验与重建：
// 把整条 `npx -y autowonder@<ver> connect …` 命令解析为结构化字段
// （ws-url、token、executor-id、provider、model 等，未知 flag 原样保序保留），
// 支持编辑结构化字段后重新生成整条命令（roundtrip 不丢信息），
// 以及解析旧 executor-manager.sh 脚本的 EXECUTORS 数组完成导入。
// 本包是纯函数库：不碰进程、不碰网络。
//
// 入口：Parse 解析整条命令为 Parsed（fields.go，含 Validate 校验与
// Render 重建）；Argv 做 shell 词法切分（argv.go / tokenize.go）；
// script.go 解析旧脚本，archive.go 归档旧日志（ArchiveLogs）。
package launchcmd
