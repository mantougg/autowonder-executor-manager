# awem 构建工作流。
# 版本注入：make build VERSION=v0.1.0（默认 0.0.0-dev；发布流程见 T11）。

VERSION ?= 0.0.0-dev
BINARY  := awem
LDFLAGS := -X awem/internal/version.Version=$(VERSION)

.PHONY: build test vet clean web-dist

# 构建：先保证 web/dist/ 存在（前端构建钩子），再编译单二进制
build: web-dist
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/awem

# web 前端构建钩子：
#  - web/ 前端工程（T09）建立且已装依赖 → 跑前端构建产出 web/dist/
#  - 工程存在但未装依赖 → 跳过并提示，沿用现有 web/dist/
#  - 工程尚未建立（当前阶段）→ 复制 web/placeholder/ 占位页
web-dist:
	@if [ -f web/package.json ] && [ -d web/node_modules ]; then \
		echo ">> 构建前端（web/，npm run build）…"; npm --prefix web run build; \
	elif [ -f web/package.json ]; then \
		echo ">> web/ 未安装依赖（缺 web/node_modules），跳过前端构建，沿用现有 web/dist/"; \
	else \
		echo ">> web/ 前端工程未建立，使用占位页"; \
	fi
	@if [ ! -f web/dist/index.html ]; then \
		mkdir -p web/dist && cp web/placeholder/index.html web/dist/index.html; \
		echo ">> 已从 web/placeholder/ 生成 web/dist/ 占位页"; \
	fi

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -f $(BINARY) $(BINARY).exe
