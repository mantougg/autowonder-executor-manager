// listen.go：本地 API 监听与端口写回（T06-F2）。
// 语义：配置 port=0 → 内核挑空闲端口并把实际端口**结构化写回** config.toml
// （config.Save：注释头保留、全量字段落盘）；写回失败不阻塞启动（记事件，
// 下次启动重新挑端口）。写回后由调用方 Watcher.SetCurrent 同步内存，
// 抑制文件监视触发的等价重载。
package daemon

import (
	"fmt"
	"net"
	"strconv"

	"awem/internal/config"
)

// listenWithWriteBack 按配置监听 127.0.0.1:port（port=0 自动挑），
// port=0 时把实际端口写回 config.toml 并返回更新后的配置。
// 返回的 listener 由调用方负责关闭。
func listenWithWriteBack(p config.Paths, cfg config.Config, onEvent func(detail string)) (net.Listener, config.Config, error) {
	ln, err := net.Listen("tcp", net.JoinHostPort(cfg.Server.Bind, strconv.Itoa(cfg.Server.Port)))
	if err != nil {
		return nil, cfg, fmt.Errorf("监听 %s:%d 失败：%w", cfg.Server.Bind, cfg.Server.Port, err)
	}
	tcp, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		ln.Close()
		return nil, cfg, fmt.Errorf("监听地址 %s 非 TCP，无法确定端口", ln.Addr())
	}
	if cfg.Server.Port == 0 && tcp.Port != 0 {
		cfg.Server.Port = tcp.Port
		if err := config.Save(p.ConfigFile, cfg); err != nil {
			// 写回失败：daemon 继续用当前端口运行（CLI 探活读 daemon.json），
			// 下次启动重新挑端口；记事件提示。
			if onEvent != nil {
				onEvent("端口写回失败（不影响本次运行）：" + err.Error())
			}
		}
	}
	return ln, cfg, nil
}
