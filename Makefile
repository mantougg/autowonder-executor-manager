# awem 构建工作流。
# 版本注入：make build VERSION=v0.1.0；缺省自动取 git describe（如
# v0.1.0-2-g1a2b3c4，无 git 或无 tag 的环境回退 0.0.0-dev）——本地开发
# 构建的 daemon 横幅也显示真实版本。发布流程见 T11（显式传 VERSION）。

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.0.0-dev)
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

# 发布构建（T11-F1）：五平台交叉编译 + checksums.txt（design §14）。
# 用法：make release VERSION=v0.1.0（产物在 release/）
RELEASE_DIR := release
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: release release-clean

release: web-dist
	@mkdir -p $(RELEASE_DIR)
	@set -e; for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		ext=""; [ "$$os" = "windows" ] && ext=".exe"; \
		out="$(RELEASE_DIR)/awem-$$os-$$arch$$ext"; \
		echo ">> 构建 $$out"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -ldflags "$(LDFLAGS)" -o $$out ./cmd/awem; \
	done
	@cd $(RELEASE_DIR) && sha256sum awem-* > checksums.txt && echo ">> 校验和已生成 $(RELEASE_DIR)/checksums.txt"
	@ls -lh $(RELEASE_DIR)

release-clean:
	rm -rf $(RELEASE_DIR)
