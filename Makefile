BINARY_NAME=pvectgen
BUILD_DIR=bin
PROTO_DIR=proto
INSTALL_DIR=/usr/local/bin
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

.PHONY: build build-local install lint test proto clean deploy

# Cross-compile for Linux (Proxmox)
build:
	rm -rf $(BUILD_DIR)
	mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/pvectgen
	cp -r cloudinit $(BUILD_DIR)/
	cp -r config $(BUILD_DIR)/

# Build for current platform
build-local:
	go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BINARY_NAME) ./cmd/pvectgen

# Build and install to /usr/local/bin (or INSTALL_DIR)
install: build-local
	@if [ -w "$(INSTALL_DIR)" ]; then \
		mv $(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME); \
	else \
		echo "Installing to $(INSTALL_DIR) (requires sudo)..."; \
		sudo mv $(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME); \
	fi
	@echo "Installed $(INSTALL_DIR)/$(BINARY_NAME) $(VERSION)"

lint:
	go vet ./...

test:
	go test ./...

proto:
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		$(PROTO_DIR)/pvectgen/v1/service.proto

clean:
	rm -rf $(BUILD_DIR) $(BINARY_NAME) dist/

# Deploy minion to a Proxmox node via SSH
# Usage: make deploy NODE=root@proxmox-ip
deploy: build
ifndef NODE
	$(error NODE is required. Usage: make deploy NODE=root@192.168.1.100)
endif
	ssh $(NODE) 'systemctl stop pvectgen-minion 2>/dev/null || true'
	scp $(BUILD_DIR)/$(BINARY_NAME) $(NODE):/usr/local/bin/$(BINARY_NAME)
	scp configs/pvectgen-minion.service $(NODE):/tmp/pvectgen-minion.service
	ssh $(NODE) '\
		mkdir -p /etc/pvectgen && \
		test -f /etc/pvectgen/minion.yaml || $(BINARY_NAME) minion connect --config /etc/pvectgen/minion.yaml 2>/dev/null; \
		cp /tmp/pvectgen-minion.service /etc/systemd/system/pvectgen-minion.service && \
		systemctl daemon-reload && \
		systemctl enable pvectgen-minion && \
		systemctl start pvectgen-minion && \
		echo "Minion deployed and started." && \
		pvectgen minion connect --config /etc/pvectgen/minion.yaml \
	'
