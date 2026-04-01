# ClawDesk - 构建与开发
#
# 使用方法:
#   make dev            启动开发模式 (wails dev)
#   make build          构建生产版本 (wails build)
#   make test           运行测试
#   make setup-cache    预下载嵌入资源到本地缓存（可选，应用首次启动也会自动下载）
#   make clean-cache    清理本地缓存的嵌入资源

# ============ 版本与下载源 ============

ORT_VERSION     := 1.24.4
ORT_BASE_URL    := https://github.com/microsoft/onnxruntime/releases/download/v$(ORT_VERSION)
MODEL_URL       := https://huggingface.co/xenova/multilingual-e5-small/resolve/main/onnx/model_quantized.onnx
TOKENIZER_URL   := https://huggingface.co/intfloat/multilingual-e5-small/resolve/main/tokenizer.json

# ============ 缓存路径 ============

CACHE_DIR       := $(HOME)/.clawdesk/cache/ort

# ============ 平台检测 ============

UNAME_S := $(shell uname -s)
UNAME_M := $(shell uname -m)

ifeq ($(UNAME_S),Darwin)
  ifeq ($(UNAME_M),arm64)
    ORT_ARCHIVE := onnxruntime-osx-arm64-$(ORT_VERSION).tgz
    ORT_LIB_IN_ARCHIVE := onnxruntime-osx-arm64-$(ORT_VERSION)/lib/libonnxruntime.$(ORT_VERSION).dylib
    ORT_LIB_NAME := libonnxruntime.dylib
    ORT_FORMAT := tgz
  endif
else ifeq ($(UNAME_S),Linux)
  ORT_ARCHIVE := onnxruntime-linux-x64-$(ORT_VERSION).tgz
  ORT_LIB_IN_ARCHIVE := onnxruntime-linux-x64-$(ORT_VERSION)/lib/libonnxruntime.so.$(ORT_VERSION)
  ORT_LIB_NAME := libonnxruntime.so
  ORT_FORMAT := tgz
endif

# ============ 主要目标 ============

.PHONY: dev build test setup-cache clean-cache help

help: ## 显示帮助
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

dev: ## 启动开发模式
	wails dev

build: ## 构建生产版本
	wails build

test: setup-cache ## 运行 memory 模块测试
	go test ./src/memory/ -v -count=1

setup-cache: $(CACHE_DIR)/.ready ## 预下载嵌入资源到本地缓存

clean-cache: ## 清理本地缓存的嵌入资源
	rm -rf $(CACHE_DIR)
	@echo "✅ 缓存已清理"

# ============ 缓存下载 ============

$(CACHE_DIR)/.ready: $(CACHE_DIR)/multilingual-e5-small-quantized.onnx $(CACHE_DIR)/tokenizer.json $(CACHE_DIR)/$(ORT_LIB_NAME)
	@echo "ok" > $@
	@echo "✅ 嵌入资源缓存准备完成: $(CACHE_DIR)"

$(CACHE_DIR)/multilingual-e5-small-quantized.onnx:
	@echo "⬇️  下载 ONNX 嵌入模型 (~113MB)..."
	@mkdir -p $(CACHE_DIR)
	@curl -L --progress-bar -o $@ "$(MODEL_URL)"

$(CACHE_DIR)/tokenizer.json:
	@echo "⬇️  下载 tokenizer.json (~16MB)..."
	@mkdir -p $(CACHE_DIR)
	@curl -L --progress-bar -o $@ "$(TOKENIZER_URL)"

$(CACHE_DIR)/$(ORT_LIB_NAME):
	@echo "⬇️  下载 ONNX Runtime..."
	@mkdir -p $(CACHE_DIR)
	@$(call download_ort)

# ============ 辅助函数 ============

define download_ort
	$(eval TMP_DIR := $(shell mktemp -d /tmp/ort-download-XXXXXX))
	curl -L --progress-bar -o $(TMP_DIR)/archive.$(ORT_FORMAT) "$(ORT_BASE_URL)/$(ORT_ARCHIVE)"
	tar xzf $(TMP_DIR)/archive.$(ORT_FORMAT) -C $(TMP_DIR)
	cp $(TMP_DIR)/$(ORT_LIB_IN_ARCHIVE) $(CACHE_DIR)/$(ORT_LIB_NAME)
	chmod 755 $(CACHE_DIR)/$(ORT_LIB_NAME)
	rm -rf $(TMP_DIR)
endef
