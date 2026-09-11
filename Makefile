.PHONY: bot api web web-build build test lint

# Run the Discord bot (reads .env)
bot:
	set -a; . ./.env; set +a; go run ./cmd/bot

# Run the API server (reads .env)
api:
	set -a; . ./.env; set +a; go run ./cmd/api

# Run the dashboard dev server on :5173 (proxies /api to the API's PORT)
web:
	set -a; . ./.env; set +a; cd web && npm run dev

web-build:
	cd web && npm run build

build:
	go build -o bin/bot ./cmd/bot
	go build -o bin/api ./cmd/api

test:
	go test ./...

lint:
	go vet ./...
	cd web && npm run check
