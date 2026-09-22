.PHONY: all build build-frontend build-server clean dev-server dev-frontend opennhrp-manager install

GO ?= go
GOOS ?= linux
CGO_ENABLED ?= 0
LDFLAGS ?= -s -w

all: build

opennhrp-manager: build-server

# =======================================================
# 前端构建 (Frontend Build)
# =======================================================
deps-frontend:
	@if [ ! -d "frontend/node_modules" ]; then \
		(cd frontend && pnpm install) || (cd frontend && npm install); \
	fi

build-frontend:
	@if [ ! -f "frontend/dist/index.html" ]; then \
		$(MAKE) deps-frontend && ( (cd frontend && pnpm build) || (cd frontend && npm run build) ); \
	fi

# =======================================================
# 生产二进制编译 (Go Cross-Compilation)
# =======================================================
build-server: build-frontend
	mkdir -p build
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) GOMIPS=$(GOMIPS) GOARM=$(GOARM) \
		$(GO) build -ldflags="$(LDFLAGS)" -o build/opennhrp-manager main.go

build: build-server

install:
	mkdir -p $(DESTDIR)/usr/sbin
	@if [ -f "build/opennhrp-manager" ]; then \
		install -m 755 build/opennhrp-manager $(DESTDIR)/usr/sbin/opennhrp-manager; \
	fi

clean:
	rm -rf build frontend/dist

# =======================================================
# 开发与快速调试模式 (Development with Vite HMR)
# =======================================================
dev-frontend:
	cd frontend && pnpm dev

dev-server:
	go run main.go --port 8080
