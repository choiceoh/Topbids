.PHONY: dev dev-api dev-ui build clean db db-stop lint lint-go lint-ui test test-go test-ui test-integration fmt check-fmt ci up down backup backup-start backup-stop backup-logs

ifneq (,$(wildcard .env))
include .env
export
endif

# 개발 (air + vite build --watch 통합 서버)
dev:
	@trap 'kill 0' EXIT; \
	(cd frontend && npx vite build --watch) & \
	sleep 3 && (cd backend && air) & \
	wait

# 빌드
build: build-ui build-api

build-ui:
	cd frontend && npm run build

build-api:
	cd backend && go build -o ../bin/topbid ./cmd/server

# 정리
clean:
	rm -rf bin/ backend/cmd/server/static/assets

# Docker — 전체 스택 (DB + App)
up:
	docker compose up --build -d

down:
	docker compose down

logs:
	docker compose logs -f app

# DB만 (로컬 개발용)
db:
	docker compose up -d db

db-stop:
	docker compose down

db-reset:
	docker compose down -v && docker compose up -d db

# 린트
lint: lint-go lint-ui

lint-go:
	cd backend && golangci-lint run ./...

lint-ui:
	cd frontend && npm run lint

# 테스트
test: test-go test-ui

test-go:
	cd backend && go test -short ./...

test-integration:
	cd backend && go test -p 1 -count=1 ./...

test-ui:
	cd frontend && npm run test

# 백업
backup:
	./scripts/backup-db.sh

backup-start:
	docker compose --profile backup up -d backup

backup-stop:
	docker compose --profile backup stop backup

backup-logs:
	docker compose --profile backup logs -f backup

# 포맷
fmt:
	cd backend && gofmt -w .
	cd frontend && npx eslint . --fix

# 포맷 검사 (CI용)
check-fmt:
	@cd backend && test -z "$$(gofmt -l .)" || (echo "gofmt 위반 파일:"; gofmt -l .; exit 1)

# 로컬 CI (PR 전 전체 검증)
ci: check-fmt lint test
	cd frontend && npx tsc --noEmit
	cd backend && go build ./cmd/server
