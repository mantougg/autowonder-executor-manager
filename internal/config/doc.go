// Package config 负责 awem 的配置管理：config.toml 的读写与校验、
// OS 惯例目录（awem/config.toml + data/）的定位与初始化、文件监视热加载。
//
// 数据模型在 model.go（Config / Executor / ServerConfig / PlatformConfig /
// LogConfig / RestartPolicy）；目录布局用 Paths（DefaultLayout 尊重
// AWEM_CONFIG_DIR 覆盖）；热加载用 Watcher（watch.go，Subscribe 订阅变更）。
// 另含 API token 读写（token*.go，0600）与 daemon.json 读写（daemon*.go）。
package config
