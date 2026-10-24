// Package launchcmd 负责执行器启动命令的解析、校验与重建：
// 把整条 `npx -y autowonder@<ver> connect …` 命令解析为结构化字段
// （ws-url、token、executor-id、provider、model 等，未知 flag 原样保序保留），
// 支持编辑结构化字段后重新生成整条命令（roundtrip 不丢信息），
// 以及解析旧 executor-manager.sh 脚本的 EXECUTORS 数组完成导入。
// 本包是纯函数库：不碰进程、不碰网络。
package launchcmd
