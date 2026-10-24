// Package service 负责把 awem daemon 注册为系统服务，随系统常驻：
// Linux systemd user 服务（systemd.go）、macOS launchd LaunchAgent
// （launchd.go）、Windows 登录自启（startup.go）。
//
// 入口：ForOS / Current 取当前平台实现（Platform 接口）；
// Install / Uninstall / Status 为 CLI service 子命令的实现，
// 操作均写管理事件日志（logs.EventLog）。
package service
