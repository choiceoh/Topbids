# Topbids — 입찰 도메인 확장 설계

## Context

Topbids는 Topworks(노코드 업무앱 플랫폼)를 기반으로 한 그룹 통합 구매·입찰 시스템이다.
공고문 양식이 카테고리(자재/용역/시공)별로 다르므로 **RFQ 본문·공급사 양식은 노코드 앱으로 구성**하고,
입찰 무결성에 관련된 핵심 로직은 **고정 코드로 확장**한다.

### 노코드로 충분한 영역

- RFQ 공고문 필드 (카테고리별 커스터마이즈)
- 공급사 기본 정보, PQ 심사표
- 리스트/칸반/캘린더 뷰
- 승인 워크플로우 (automation)
- 파일 첨부, 댓글, 히스토리

### 코드로 고정해야 하는 영역

| 기능 | 이유 |
|------|------|
| 밀봉입찰 행 단위 접근 제어 | 노코드 권한은 필드·앱 단위. 행 단위 조건부 read 불가 |
| 개찰 시각 도달 시 상태 전이 | 스케줄러 필요, automation 트리거 범위 밖 |
| 가격 점수 자동 계산·낙찰 판정 | 경쟁 입찰서 전체를 봐야 함. Formula 필드는 단일 레코드 스코프 |
| PO 자동 분배 (계열사 수량 비례) | 복수 테이블 트랜잭션 쓰기. automation 액션 범위 밖 |
| 공급사 전용 포털 | 외부 사용자 UI 분리. 내부 직원용 SPA와 라우팅 분리 필요 |

---

## 스키마 확장점

### 필드 속성 추가

`works_fields` 테이블의 `options` JSON에 입찰 전용 키 추가:

```json
{
  "sealedUntilAt": "field:openAt",
  "unlockByStatus": ["opened", "evaluating", "awarded"]
}
```

- `sealedUntilAt`: 필드·앱명 참조 (`field:<name>`) 또는 절대 시각. 해당 시각 전에는 제출자 외 read 차단
- `unlockByStatus`: 대상 레코드의 상태 필드가 이 값 중 하나가 되면 해제

검증 규칙: 동일 앱에 `sealedUntilAt`을 가진 필드가 존재하면, 해당 앱은 "bid-like"로 분류되어 행 단위 접근 제어 미들웨어를 거친다.

### 앱 속성 추가

`works_apps`의 `meta` JSON:

```json
{
  "bidRole": "rfq" | "bid" | "supplier" | "award" | "po",
  "ownerField": "createdBy" | "supplier" | null
}
```

`bidRole=bid`인 앱에만 밀봉 접근 제어가 활성화된다. 스키마 설계 시 한 번 지정.

---

## 접근 제어 확장

### 기존: `middleware/collection_access.go`

앱 단위 read/write 권한. role별로 앱 전체 허용/차단.

### 추가: 행 단위 밀봉 필터

위치: `backend/internal/bid/access.go` (신규)

```go
// SealedReadFilter returns an additional WHERE clause for sealed bid rows.
// Called from data engine Query/Get after base RBAC passes.
func SealedReadFilter(user *User, appMeta AppMeta) (string, []any, error) {
  // bidRole != "bid" → no-op
  // user.Role == "admin" || "buyer" AND status in unlockByStatus → no-op
  // user.Role == "supplier" → WHERE owner_field = user.SupplierID
  // 그 외 → WHERE 1=0 (읽을 수 없음)
}
```

Data Engine(`backend/internal/engine/data.go`)의 `Query`/`Get`에서 `bidRole=bid`인 앱 조회 시 이 필터를 AND로 붙인다.

### 감사

모든 밀봉 레코드 read 시도는 `bid_audit_log`에 기록:

```sql
CREATE TABLE bid_audit_log (
  id bigserial PRIMARY KEY,
  actor_id bigint NOT NULL,
  action text NOT NULL,        -- 'read_sealed' | 'read_opened' | 'submit' | 'open' | 'award'
  app_slug text NOT NULL,
  row_id bigint,
  ip inet,
  created_at timestamptz NOT NULL DEFAULT now()
);
```

append-only. UPDATE/DELETE 권한 없음.

---

## 자동화 액션 추가

기존 `automation` 엔진에 입찰 전용 액션 3개 등록:

| 액션 | 트리거 | 동작 |
|------|--------|------|
| `openRfq` | `deadlineAt` 도달 (스케줄러) | RFQ status → `opened`, 모든 입찰서 잠금 해제, 알림 |
| `awardBid` | 평가 완료 버튼 (수동) | 최저가/최고점 입찰서 선정, 나머지 `rejected`, 낙찰 레코드 생성 |
| `distributePO` | 낙찰 직후 (연쇄) | `purchase_requests`의 계열사별 수량 비례로 PO 레코드 자동 생성 |

위치: `backend/internal/bid/actions.go`

스케줄러: 기존 엔진이 cron을 지원하지 않으면 `backend/internal/bid/scheduler.go` 신규 — 10초 폴링으로 `deadlineAt <= now() AND status = 'published'` 스캔.

---

## 공급사 Role + 포털

### role 확장

`users.role`에 `supplier` 값 추가. 기존 `admin`, `buyer`, `requester`와 병렬.

- `supplier` role 사용자는 `users.supplier_id` 필수
- 로그인 시 리다이렉트: `/portal/*` (내부 직원은 `/app/*`)

### 포털 라우팅

```
frontend/src/pages/portal/
  PortalLoginPage.tsx
  PortalRfqListPage.tsx      초청받은 RFQ만
  PortalBidSubmitPage.tsx    본인 입찰서 작성
  PortalBidHistoryPage.tsx   본인 과거 입찰 내역
```

shadcn/ui 컴포넌트 재사용. 내부용 네비게이션(AppBuilder 등)은 숨김.

---

## 시드 앱 3개

`backend/internal/seed/bid_apps.go` — 설치 시 기본 앱 자동 생성:

### 1. "입찰 공고" (`rfqs`)

```
- rfqNo          text, unique
- title          text, required
- category       select (material/service/construction/…)
- mode           select (open/invited/private)
- evalMethod     select (lowest/weighted)
- sealed         checkbox, default=true
- deadlineAt     datetime, required
- openAt         datetime, required
- status         select (draft/published/closed/opened/evaluating/awarded/failed/cancelled)
- sourceRequests relation→purchase-requests (multi)
- createdBy      user-ref
```

meta: `{ "bidRole": "rfq" }`

### 2. "입찰서" (`bids`)

```
- rfq            relation→rfqs, required
- supplier       relation→suppliers, required
- totalAmount    number      ← sealedUntilAt: "field:rfq.openAt"
- leadTime       number      ← sealedUntilAt: "field:rfq.openAt"
- techScore      number (평가자 입력)
- priceScore     number (자동)
- totalScore     number (자동)
- status         select
- submittedAt    datetime
```

meta: `{ "bidRole": "bid", "ownerField": "supplier" }`

### 3. "공급사" (`suppliers`)

```
- name, bizNo, ceo, email, phone, address
- categories     multi-select
- status         select (active/suspended/blacklisted)
```

meta: `{ "bidRole": "supplier" }`

+ 별도 앱 "PQ 심사" (`supplier-qualifications`) 스키마는 사용자가 카테고리별로 조립하도록 노코드 영역.

---

## 구현 순서

1. **docs/10-BID-EXTENSION.md 작성** ✓
2. 스키마 확장: `works_fields.options`에 `sealedUntilAt` 파싱/검증
3. `backend/internal/bid/access.go` — SealedReadFilter
4. Data Engine에 필터 연결
5. `backend/internal/bid/scheduler.go` + `actions.go` (openRfq)
6. `awardBid`, `distributePO` 액션
7. 시드 앱 3개
8. 공급사 role + `/portal/*` 프론트
9. 감사 로그 테이블 + 기록 훅
10. E2E 시나리오 테스트 (제출 → 개찰 → 낙찰 → PO)

---

## 비고

- 공공조달 수준 요구사항(전자서명·타임스탬프·암호화 입찰서)은 **범위 밖**. 내부 규정 수준의 접근 제어·감사로 충분하다는 전제.
- Phaeton 업스트림 변경분은 가능한 한 머지. `bid/` 패키지·`bidRole` 필드·`sealed*` 필드 속성은 본 프로젝트 전용.
