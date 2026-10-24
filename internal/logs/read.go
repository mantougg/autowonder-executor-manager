// read.go：事件日志滚动链读取与执行器日志按位定位（一期时间带/崩溃穿透共用）。
//
// 安全约定：执行器日志文件名必须严格等于 <name>.log 或 <name>.log.<正整数>，
// 读取只在该执行器日志链内进行（拒绝 ../、绝对路径、其他执行器文件）。
package logs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ReadEvents 读取 events.log 滚动链（events.log.N … events.log.1、events.log），
// 按时间稳定排序返回。文件不存在视为空；坏行跳过（不阻塞页面）。
func (m *Manager) ReadEvents() ([]Event, error) {
	files := eventChain(m.eventsPath)
	var out []Event
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("读取事件日志 %s 失败：%w", file, err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var ev Event
			if err := json.Unmarshal([]byte(line), &ev); err != nil {
				continue
			}
			if ev.Time.IsZero() {
				continue
			}
			out = append(out, ev)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	return out, nil
}

// eventChain 返回事件日志链文件（旧→新）；链尾（最大编号）最新，当前文件最后。
func eventChain(eventsPath string) []string {
	matches, _ := filepath.Glob(eventsPath + ".*")
	type numbered struct {
		idx  int
		path string
	}
	var nums []numbered
	for _, m := range matches {
		suffix := strings.TrimPrefix(filepath.Base(m), filepath.Base(eventsPath)+".")
		idx, err := strconv.Atoi(suffix)
		if err != nil || idx < 1 {
			continue
		}
		nums = append(nums, numbered{idx: idx, path: m})
	}
	sort.Slice(nums, func(i, j int) bool { return nums[i].idx > nums[j].idx })
	files := make([]string, 0, len(nums)+1)
	for _, n := range nums {
		files = append(files, n.path)
	}
	return append(files, eventsPath)
}

// Position 返回执行器当前日志文件标识（基名）与字节大小（崩溃事件定位用）。
func (m *Manager) Position(name string) (string, int64) {
	st, err := os.Stat(m.executorPath(name))
	if err != nil {
		return "", 0
	}
	return filepath.Base(m.executorPath(name)), st.Size()
}

// ValidExecutorLogFile 校验 file 是否属于该执行器日志链且实际存在。
func (m *Manager) ValidExecutorLogFile(name, file string) bool {
	if !validLogFileName(name, file) {
		return false
	}
	st, err := os.Stat(filepath.Join(m.executorsDir, file))
	return err == nil && !st.IsDir()
}

// validLogFileName 严格校验日志文件名（不触盘）。
func validLogFileName(name, file string) bool {
	if name == "" || name != filepath.Base(name) || name == "." || name == ".." {
		return false
	}
	if file == "" || file != filepath.Base(file) {
		return false
	}
	if strings.ContainsAny(file, `/\\`) || file == "." || file == ".." {
		return false
	}
	base := name + ".log"
	if file == base {
		return true
	}
	prefix := base + "."
	if !strings.HasPrefix(file, prefix) {
		return false
	}
	suffix := strings.TrimPrefix(file, prefix)
	if suffix == "" {
		return false
	}
	for _, r := range suffix {
		if r < '0' || r > '9' {
			return false
		}
	}
	n, err := strconv.Atoi(suffix)
	return err == nil && n >= 1
}

// LogChain 返回该执行器日志链（旧→新，仅含实际存在的文件）。
func (m *Manager) LogChain(name string) []string {
	base := name + ".log"
	matches, _ := filepath.Glob(filepath.Join(m.executorsDir, base+".*"))
	type numbered struct {
		idx  int
		file string
	}
	var nums []numbered
	for _, p := range matches {
		file := filepath.Base(p)
		suffix := strings.TrimPrefix(file, base+".")
		idx, err := strconv.Atoi(suffix)
		if err != nil || idx < 1 || !validLogFileName(name, file) {
			continue
		}
		nums = append(nums, numbered{idx: idx, file: file})
	}
	sort.Slice(nums, func(i, j int) bool { return nums[i].idx > nums[j].idx })
	out := make([]string, 0, len(nums)+1)
	for _, n := range nums {
		if m.ValidExecutorLogFile(name, n.file) {
			out = append(out, n.file)
		}
	}
	if m.ValidExecutorLogFile(name, base) {
		out = append(out, base)
	}
	return out
}

// fileLine 是一行日志及其在文件中的字节偏移（start 为行首，end 为含换行）。
type fileLine struct {
	start int64
	end   int64
	text  string
}

// readFileLines 读取文件并按行切分（保留字节偏移）。
func readFileLines(path string) ([]fileLine, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []fileLine
	var start int64
	for start < int64(len(data)) {
		rel := bytes.IndexByte(data[start:], '\n')
		var end int64
		var text string
		if rel < 0 {
			end = int64(len(data))
			text = string(data[start:])
		} else {
			end = start + int64(rel) + 1
			text = strings.TrimSuffix(string(data[start:start+int64(rel)]), "\r")
		}
		out = append(out, fileLine{start: start, end: end, text: text})
		start = end
	}
	return out, nil
}

// ReadBefore 读取该执行器日志链中 file 文件的 before 字节之前、最多 limit 行的
// 历史日志（跨滚动文件向前补齐，返回按时间顺序）。startFile/startOffset 为
// 第一行的位置；hasMore 表示窗口前仍有更早日志。
func (m *Manager) ReadBefore(name, file string, before int64, limit int) ([]string, string, int64, bool, error) {
	if before < 0 {
		return nil, "", 0, false, fmt.Errorf("before 不能为负数")
	}
	if limit <= 0 {
		limit = 500
	}
	if limit > 2000 {
		limit = 2000
	}
	if !m.ValidExecutorLogFile(name, file) {
		return nil, "", 0, false, fmt.Errorf("日志文件不在该执行器日志链内：%s", file)
	}
	chain := m.LogChain(name)
	idx := -1
	for i, f := range chain {
		if f == file {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, "", 0, false, fmt.Errorf("日志文件不在该执行器日志链内：%s", file)
	}

	type posLine struct {
		file  string
		start int64
		text  string
	}
	var acc []posLine // 按时间顺序
	hasMore := false
	// 从定位文件向更早的滚动文件回退，直到凑够 limit+1 行（多 1 行用于判断 hasMore）。
	for i := idx; i >= 0; i-- {
		lines, err := readFileLines(filepath.Join(m.executorsDir, chain[i]))
		if err != nil {
			return nil, "", 0, false, fmt.Errorf("读取日志 %s 失败：%w", chain[i], err)
		}
		if i == idx {
			// 只保留完整位于 before 之前的行（崩溃点所在半行不展示）。
			filtered := lines[:0]
			for _, ln := range lines {
				if ln.end <= before {
					filtered = append(filtered, ln)
				}
			}
			lines = filtered
		}
		remaining := limit + 1 - len(acc)
		if remaining <= 0 {
			hasMore = true
			break
		}
		if len(lines) > remaining {
			lines = lines[len(lines)-remaining:]
			hasMore = true
		}
		part := make([]posLine, 0, len(lines))
		for _, ln := range lines {
			part = append(part, posLine{file: chain[i], start: ln.start, text: ln.text})
		}
		acc = append(part, acc...)
	}
	if len(acc) > limit {
		acc = acc[len(acc)-limit:]
		hasMore = true
	}
	out := make([]string, 0, len(acc))
	for _, ln := range acc {
		out = append(out, ln.text)
	}
	if len(acc) == 0 {
		return out, file, before, false, nil
	}
	return out, acc[0].file, acc[0].start, hasMore, nil
}
