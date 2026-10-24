package launchcmd

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// fullCmd 是覆盖全部已知字段的标准命令。
const fullCmd = `npx -y autowonder@0.3.22 connect --ws-url ws://10.0.0.1:7001/ws/executor ` +
	`--token tok-abc --executor-id 10086 --provider qoder --model qfmodel ` +
	`--reasoning-effort medium --context-window 1000000 --memory-mode platform ` +
	`--max-tasks 5 --token-aware-enable`

func TestParse全字段(t *testing.T) {
	p, err := Parse(fullCmd)
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	want := Parsed{
		Prefix:          Prefix{RuntimeVersion: "0.3.22"},
		WSURL:           "ws://10.0.0.1:7001/ws/executor",
		Token:           "tok-abc",
		ExecutorID:      "10086",
		Provider:        "qoder",
		Model:           "qfmodel",
		ReasoningEffort: "medium",
		ContextWindow:   "1000000",
		MemoryMode:      "platform",
		MaxTasks:        "5",
		TokenAware:      true,
	}
	if !reflect.DeepEqual(*p, want) {
		t.Fatalf("解析结果不符：\n got=%+v\nwant=%+v", *p, want)
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("完整命令应通过校验：%v", err)
	}
}

func TestParse等号形式(t *testing.T) {
	cmd := `npx -y autowonder@1.0.0 connect --ws-url=wss://h/ws --token=t --executor-id=7 --provider=claude`
	p, err := Parse(cmd)
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if p.WSURL != "wss://h/ws" || p.Token != "t" || p.ExecutorID != "7" || p.Provider != "claude" {
		t.Fatalf("等号形式解析不符：%+v", p)
	}
}

func TestParse布尔flag两种形式(t *testing.T) {
	base := `npx -y autowonder@1.0.0 connect --token t --executor-id 1 --provider qoder`
	for _, c := range []struct {
		extra string
		aware bool
		debug bool
	}{
		{"", false, false},
		{" --token-aware-enable --debug", true, true},
		{" --token-aware-enable=false --debug=true", false, true},
	} {
		p, err := Parse(base + c.extra)
		if err != nil {
			t.Fatalf("解析失败：%v", err)
		}
		if p.TokenAware != c.aware || p.Debug != c.debug {
			t.Fatalf("布尔解析不符：%+v", p)
		}
	}
	// 非法布尔值报错
	if _, err := Parse(base + " --token-aware-enable=maybe"); err == nil {
		t.Fatal("布尔 flag 非法值未报错")
	}
}

func TestParse未知flag保序保留(t *testing.T) {
	cmd := `npx -y autowonder@1.0.0 connect --token t --executor-id 1 --provider qoder ` +
		`--future-flag=v1 --another 42 --token-aware-enable --odd=3 --bare-word`
	p, err := Parse(cmd)
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	want := []string{"--future-flag=v1", "--another", "42", "--odd=3", "--bare-word"}
	if !reflect.DeepEqual(p.Passthrough, want) {
		t.Fatalf("passthrough = %v，期望 %v", p.Passthrough, want)
	}
}

func TestParse缺值报错并定位(t *testing.T) {
	// 末尾缺值
	_, err := Parse(`npx -y autowonder@1.0.0 connect --ws-url`)
	var fe FieldErrors
	if !errors.As(err, &fe) || len(fe) != 1 || fe[0].Flag != "ws-url" {
		t.Fatalf("应定位 --ws-url 缺值，实际：%v", err)
	}
	// 值位置是另一个 flag
	_, err = Parse(`npx -y autowonder@1.0.0 connect --token --provider qoder`)
	if !errors.As(err, &fe) || len(fe) != 1 || fe[0].Flag != "token" {
		t.Fatalf("应定位 --token 缺值，实际：%v", err)
	}
}

func TestValidate逐字段规则(t *testing.T) {
	base := func() string {
		return `npx -y autowonder@1.0.0 connect --ws-url ws://h --token t --executor-id 100 --provider qoder`
	}
	t.Run("executor-id非数字", func(t *testing.T) {
		p, err := Parse(strings.Replace(base(), "--executor-id 100", "--executor-id abc", 1))
		if err != nil {
			t.Fatal(err)
		}
		err = p.Validate()
		if err == nil || !strings.Contains(err.Error(), `--executor-id："abc" 不是数字`) {
			t.Fatalf("错误定位不符：%v", err)
		}
	})
	t.Run("executor-id缺失", func(t *testing.T) {
		p, err := Parse(strings.Replace(base(), "--executor-id 100 ", "", 1))
		if err != nil {
			t.Fatal(err)
		}
		if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "--executor-id") {
			t.Fatalf("缺失应报错：%v", err)
		}
	})
	t.Run("token为空", func(t *testing.T) {
		cmd := strings.Replace(base(), "--token t", `--token ""`, 1)
		p, err := Parse(cmd)
		if err != nil {
			t.Fatal(err)
		}
		err = p.Validate()
		if err == nil || !strings.Contains(err.Error(), "--token") {
			t.Fatalf("空 token 应报错：%v", err)
		}
	})
	t.Run("provider非法与缺失", func(t *testing.T) {
		for _, cmd := range []string{
			strings.Replace(base(), "--provider qoder", "--provider gpt", 1),
			strings.Replace(base(), "--provider qoder", "", 1),
		} {
			p, err := Parse(cmd)
			if err != nil {
				t.Fatal(err)
			}
			if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "--provider") {
				t.Fatalf("provider 应报错：%v（命令：%s）", err, cmd)
			}
		}
	})
	t.Run("合法枚举全部通过", func(t *testing.T) {
		for _, prov := range Providers {
			cmd := strings.Replace(base(), "--provider qoder", "--provider "+prov, 1)
			p, err := Parse(cmd)
			if err != nil {
				t.Fatal(err)
			}
			if err := p.Validate(); err != nil {
				t.Fatalf("provider %s 应合法：%v", prov, err)
			}
		}
	})
	t.Run("多项问题聚合", func(t *testing.T) {
		cmd := strings.Replace(strings.Replace(base(), "--executor-id 100", "--executor-id x", 1), "--provider qoder", "", 1)
		p, err := Parse(cmd)
		if err != nil {
			t.Fatal(err)
		}
		err = p.Validate()
		if err == nil || !strings.Contains(err.Error(), "共 2 项问题") {
			t.Fatalf("应聚合 2 项问题：%v", err)
		}
	})
}
