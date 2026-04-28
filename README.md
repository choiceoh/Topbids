# Topbid v2

노코드 업무앱 플랫폼. 사용자가 코드 없이 앱(폼+데이터+뷰)을 만들고 ERP 기능을 직접 구성합니다.

각 앱 = PostgreSQL 테이블 (동적 DDL). Go 백엔드 + Vite React SPA.

## 스택

| 레이어 | 기술 |
|--------|------|
| 백엔드 | Go 1.26, chi, pgx v5, golang-jwt, goose |
| 프론트엔드 | Vite, React 19, shadcn/ui (Radix), Tailwind v4, React Router v7 |
| DB | PostgreSQL 16 |
| 배포 | Go 싱글 바이너리 (embed.FS로 SPA 내장) + Docker Compose |

## 주요 기능

- **앱 빌더** -- 드래그앤드롭으로 필드 추가, 3-패널 UI (팔레트/미리보기/속성)
- **다중 뷰** -- 리스트, 칸반, 캘린더, 갤러리, 간트, 차트
- **자동화** -- 트리거 기반 규칙 엔진 + AI 자동화
- **AI** -- 자연어 앱 생성, 챗 어시스턴트
- **수식 필드** -- 계산/참조 수식
- **프로세스** -- 상태 머신 기반 워크플로우
- **실시간** -- SSE 이벤트 스트리밍
- **RBAC** -- 역할 기반 접근 제어 (director/pm/engineer/viewer)

## 시작하기

### 사전 요구사항

- Go 1.26+
- Node.js 20+
- PostgreSQL 16 (또는 Docker)

### 로컬 개발

```bash
# DB 시작 (Docker)
make db

# 백엔드 (localhost:8080)
make dev-api

# 프론트엔드 (localhost:5173, /api -> :8080 프록시)
make dev-ui
```

### 빌드

```bash
make build          # 프론트 빌드 + Go 바이너리
```

### Docker 배포

```bash
docker compose up --build -d        # 전체 스택 (DB + App)
docker compose up --build -d app    # App만 리빌드 (DB 유지)
docker compose logs -f app          # 로그 확인
```

## 테스트

```bash
make test               # Go + UI 전체
make test-go            # Go 단위 테스트
make test-integration   # Go 통합 테스트
make test-ui            # 프론트엔드 테스트
```

## 린트

```bash
make lint       # Go + UI 전체
make fmt        # 자동 포맷
```

## 프로젝트 구조

```
backend/
  cmd/server/       진입점 + SPA 서빙
  internal/
    ai/             AI 클라이언트 (vLLM)
    automation/     자동화 규칙 엔진
    db/             pgx 풀 + goose 마이그레이션
    events/         SSE 이벤트 브로커
    formula/        수식 필드 파서
    handler/        HTTP 핸들러
    infra/          로깅, 에러, 메트릭스, 유틸
    middleware/     JWT, CORS, RBAC, 레이트리미터
    migration/      DDL 마이그레이션
    schema/         앱/뷰/프로세스 스토어
  pkg/              atomicfile, httputil, jsonutil

frontend/
  src/
    pages/          페이지 컴포넌트 (19개)
    components/
      works/        앱 빌더, 필드, 뷰 컴포넌트
      common/       공통 컴포넌트 (DataTable, ErrorBoundary 등)
      ui/           shadcn/ui 컴포넌트
    hooks/          React Query 기반 커스텀 훅
    lib/            API 클라이언트, 타입, 유틸
```

## 문서

- [전체 설계](docs/08-TOPBID-V2-DESIGN.md) -- 아키텍처, 데이터 모델, API
- [Data Engine 가이드](docs/09-DATA-ENGINE-GUIDE.md) -- CRUD/쿼리 구현
- [경쟁 분석](docs/10-COMPETITIVE-ANALYSIS.md) -- 로드맵
- [디자인 시스템](docs/06-DESIGN-SYSTEM.md) -- 색상, 타이포, 컴포넌트
- [린트 규칙](docs/07-LINT-RULES.md) -- ESLint, Prettier
- [태양광 도메인](docs/05-SOLAR-DOMAIN.md) -- 인허가/공사 가이드
