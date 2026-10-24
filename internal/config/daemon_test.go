package config

import (
	"os"
	"testing"
)

// TestDaemonInfo写读清理Roundtrip。
func TestDaemonInfo写读清理Roundtrip(t *testing.T) {
	p := Layout(t.TempDir())

	// 初始：无文件 → 无 daemon
	if _, running, err := ReadDaemonInfo(p); err != nil || running {
		t.Fatalf("无文件应返回无 daemon：running=%v err=%v", running, err)
	}

	// 写入当前进程 pid（必然存活）→ 读取为存活
	want := DaemonInfo{PID: os.Getpid(), Port: 7800}
	if err := WriteDaemonInfo(p, want); err != nil {
		t.Fatal(err)
	}
	got, running, err := ReadDaemonInfo(p)
	if err != nil || !running {
		t.Fatalf("应读到存活 daemon：running=%v err=%v", running, err)
	}
	if got != want {
		t.Fatalf("roundtrip 不一致：got=%+v want=%+v", got, want)
	}

	// 清理 → 无 daemon
	if err := ClearDaemonInfo(p); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p.DaemonFile); !os.IsNotExist(err) {
		t.Fatal("清理后文件应不存在")
	}
	if err := ClearDaemonInfo(p); err != nil {
		t.Fatalf("重复清理应为 no-op：%v", err)
	}
}

// TestDaemonInfoStale判定 pid 不存在时视为无 daemon（记录可读出）。
func TestDaemonInfoStale判定(t *testing.T) {
	p := Layout(t.TempDir())
	// 找一个几乎不可能存活的 pid：取大值（pid_max 上限一般远小于此）
	stale := DaemonInfo{PID: 4194300, Port: 7800}
	if err := WriteDaemonInfo(p, stale); err != nil {
		t.Fatal(err)
	}
	got, running, err := ReadDaemonInfo(p)
	if err != nil {
		t.Fatal(err)
	}
	if running {
		t.Fatal("pid 不存活应判定 stale（running=false）")
	}
	if got != stale {
		t.Fatalf("stale 记录应原样读出：got=%+v", got)
	}
}

// TestDaemonInfo文件损坏自愈 非 JSON 内容读取不 panic、按无 daemon 处理且文件被删。
func TestDaemonInfo文件损坏自愈(t *testing.T) {
	p := Layout(t.TempDir())
	if err := os.MkdirAll(p.DataDir, dirPerm); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.DaemonFile, []byte("不是json{{{"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, running, err := ReadDaemonInfo(p)
	if err != nil {
		t.Fatalf("损坏文件不应报错：%v", err)
	}
	if running || info.PID != 0 {
		t.Fatalf("损坏文件应按无 daemon 处理：running=%v info=%+v", running, info)
	}
	if _, statErr := os.Stat(p.DaemonFile); !os.IsNotExist(statErr) {
		t.Fatal("损坏文件应被自愈删除")
	}

	// 非法记录（pid<=0）同样自愈
	if err := os.WriteFile(p.DaemonFile, []byte(`{"pid":0,"port":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, running, err := ReadDaemonInfo(p); err != nil || running {
		t.Fatalf("pid=0 应按无 daemon 处理：running=%v err=%v", running, err)
	}
	if _, statErr := os.Stat(p.DaemonFile); !os.IsNotExist(statErr) {
		t.Fatal("pid=0 记录应被自愈删除")
	}
}
