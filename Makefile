BIN       := clearinghouse-bootstrap
CMD       := ./cmd/clearinghouse-bootstrap
GOFLAGS   ?=
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64
ENV_FILE  ?= .env
COMPOSE   := docker compose -f deploy/docker-compose.yml --env-file $(ENV_FILE)

.PHONY: build test clean cross stack-up stack-down stack-logs

build:
	go build $(GOFLAGS) -o $(BIN) $(CMD)

test:
	go test ./... -count=1

clean:
	rm -f $(BIN) dist/*

cross:
	@mkdir -p dist
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		echo "Building $$os/$$arch..."; \
		GOOS=$$os GOARCH=$$arch go build $(GOFLAGS) -o dist/$(BIN)-$$os-$$arch$$ext $(CMD); \
	done

stack-up:
	$(COMPOSE) up -d --build

stack-down:
	$(COMPOSE) down

stack-logs:
	$(COMPOSE) logs -f
