// argv.go：spawn 参数向量重建（T05-F1）。从整条启动命令重建可直接 exec 的
// argv：argv[0] 恒为字面 "npx"（Supervisor 负责替换为实际解析到的 npx 路径），
// 其后固定 -y autowonder@<ver> connect + connect 参数的原始词序——保留用户
// 书写的 --flag=value 形态与未知 flag，不做归一化（归一化是 Render 的职责）。
package launchcmd

// Argv 返回 spawn 用的参数向量；命令含包装残留（nohup/&）会被剥掉。
func Argv(cmd string) ([]string, error) {
	tokens, err := Tokenize(cmd)
	if err != nil {
		return nil, err
	}
	prefix, rest, err := parsePrefix(tokens)
	if err != nil {
		return nil, err
	}
	argv := make([]string, 0, len(rest)+4)
	argv = append(argv, "npx", "-y", "autowonder@"+prefix.RuntimeVersion, "connect")
	return append(argv, rest...), nil
}
