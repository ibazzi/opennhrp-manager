.PHONY: all build build-frontend build-server build-agent clean dev-server dev-frontend opennhrp-manager opennhrp-agent install deb-agent rpm-agent

GO ?= go
GOOS ?= linux
CGO_ENABLED ?= 0
BINDIR ?= bin
LDFLAGS ?= -s -w
VERSION ?= 1.0.0

all: build

opennhrp-manager: build-server
opennhrp-agent: build-agent

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
	mkdir -p $(BINDIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) GOMIPS=$(GOMIPS) GOARM=$(GOARM) \
		$(GO) build -ldflags="$(LDFLAGS)" -o $(BINDIR)/opennhrp-manager main.go

build-agent:
	mkdir -p $(BINDIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) GOMIPS=$(GOMIPS) GOARM=$(GOARM) \
		$(GO) build -ldflags="$(LDFLAGS)" -o $(BINDIR)/opennhrp-agent cmd/agent/main.go

build: build-server build-agent

install:
	mkdir -p $(DESTDIR)/usr/sbin
	@if [ -f "$(BINDIR)/opennhrp-manager" ]; then \
		install -m 755 $(BINDIR)/opennhrp-manager $(DESTDIR)/usr/sbin/opennhrp-manager; \
	fi
	@if [ -f "$(BINDIR)/opennhrp-agent" ]; then \
		install -m 755 $(BINDIR)/opennhrp-agent $(DESTDIR)/usr/sbin/opennhrp-agent; \
	fi

clean:
	rm -rf bin frontend/dist

deb-agent: build-agent
	rm -rf build/deb-agent
	install -d -m 755 build/deb-agent/DEBIAN build/deb-agent/usr/sbin build/deb-agent/lib/systemd/system build/deb-agent/etc/opennhrp-agent build/deb
	install -m 755 bin/opennhrp-agent build/deb-agent/usr/sbin/opennhrp-agent
	install -m 644 packaging/opennhrp-agent.service build/deb-agent/lib/systemd/system/opennhrp-agent.service
	install -m 600 packaging/agent.env build/deb-agent/etc/opennhrp-agent/agent.env
	sed -e 's/@VERSION@/$(VERSION)/' -e 's/@ARCH@/'"$$(dpkg --print-architecture)"'/' packaging/deb-control > build/deb-agent/DEBIAN/control
	printf '/etc/opennhrp-agent/agent.env\n' > build/deb-agent/DEBIAN/conffiles
	dpkg-deb --build --root-owner-group build/deb-agent build/deb/opennhrp-agent_$(VERSION)_$$(dpkg --print-architecture).deb

rpm-agent: build-agent
	rm -rf build/rpmbuild
	install -d -m 755 build/rpmbuild/BUILD build/rpmbuild/BUILDROOT build/rpmbuild/RPMS build/rpmbuild/SOURCES build/rpmbuild/SPECS build/rpmbuild/SRPMS
	install -m 755 bin/opennhrp-agent build/rpmbuild/SOURCES/opennhrp-agent
	install -m 644 packaging/opennhrp-agent.service build/rpmbuild/SOURCES/opennhrp-agent.service
	install -m 600 packaging/agent.env build/rpmbuild/SOURCES/agent.env
	rpmbuild -bb packaging/opennhrp-agent.spec --define '_topdir $(CURDIR)/build/rpmbuild' --define 'version $(VERSION)'

# =======================================================
# 开发与快速调试模式 (Development with Vite HMR)
# =======================================================
dev-frontend:
	cd frontend && pnpm dev

dev-server:
	go run main.go --port 8080
