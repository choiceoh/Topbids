# Data Engine 구현 가이드

## 목표

사용자가 만든 동적 테이블(`wd_*`)에 대한 CRUD + 쿼리 레이어. Schema Engine이 테이블 구조(DDL)를 책임지고, Data Engine은 그 위에서 **안전하고 빠른 데이터 조작**을 담당한다.

핵심 제약 4가지:

1. **SQL 인젝션 절대 금지** — 컬럼/테이블명은 `works_fields`/`works_apps` 화이트리스트 검증 후 `pgx.Identifier{}.Sanitize()`로만 사용. 값은 무조건 `$1, $2` 파라미터 바인딩.
2. **타입 강제** — 사용자가 JSON으로 보내는 `any` 값을 필드 타입(text/number/date/select/checkbox/file/app-ref/user-ref)에 맞게 강제 변환·검증.
3. **N+1 방지** — 스키마(works_fields)를 매 쿼리마다 로드하지 말 것. `sync.Map` 기반 캐시 + Schema Engine의 변경 훅으로 무효화.
4. **성능 목표** — 5만 건 동적 테이블에서 필터+페이지네이션 쿼리 < 100ms.

---

## 위치

```
backend/internal/engine/
  engine.go            Engine 구조체 (이미 존재)
  schema.go            Schema Engine — 병렬 세션
  data.go              Data Engine — 이 가이드 대상
  validation.go        타입 검증/강제 로직
  querybuilder.go      안전한 SELECT/WHERE/ORDER BY 조립
  cache.go             앱 스키마 캐시
```

---

## 엔진 구조체 확장

```go
type Engine struct {
    pool   *pgxpool.Pool
    logger *slog.Logger
    cache  *schemaCache  // 추가
}

type schemaCache struct {
    mu    sync.RWMutex
    apps  map[int]*cachedApp  // appID → 스키마
}

type cachedApp struct {
    tableName string
    fields    []model.Field
    byName    map[string]*model.Field   // works_fields.name → Field
    byColumn  map[string]*model.Field   // works_fields.column_name → Field
    loadedAt  time.Time
}
```

캐시 TTL은 두지 말고, Schema Engine이 `CreateApp`/`AddField`/`RemoveField`/`UpdateField`/`DeleteApp` 성공 시 `eng.cache.invalidate(appID)` 호출. 테스트 가능하도록 `Engine.InvalidateSchema(appID int)` 공개 메서드 제공.

---

## 핵심 함수 시그니처

```go
// CreateEntry — 동적 테이블에 row INSERT
// data: {fieldName: value} — 필드 이름 기준 (column_name 아님)
// 반환: 새로 생성된 row의 id
func (e *Engine) CreateEntry(ctx context.Context, appID int, data map[string]any, userID int) (int, error)

// UpdateEntry — 기존 row UPDATE (부분 업데이트)
// data에 포함된 필드만 UPDATE, 나머지는 유지
func (e *Engine) UpdateEntry(ctx context.Context, appID int, entryID int, data map[string]any) error

// DeleteEntry — row DELETE
func (e *Engine) DeleteEntry(ctx context.Context, appID int, entryID int) error

// GetEntry — 단일 row 조회
// 반환: {column_name: value, _created_at: ..., _created_by: ...}
// 존재하지 않으면 (nil, nil)
func (e *Engine) GetEntry(ctx context.Context, appID int, entryID int) (map[string]any, error)

// QueryEntries — 필터/정렬/페이지네이션 조회
func (e *Engine) QueryEntries(ctx context.Context, appID int, q model.QueryParams) (*model.PagedResult, error)

// AggregateEntries — Kanban/리포트 뷰용 GROUP BY
// groupField: 그룹핑할 필드 이름 (select 타입)
// 반환: [{value: "high", count: 12}, ...]
func (e *Engine) AggregateEntries(ctx context.Context, appID int, groupField string) ([]model.AggResult, error)
```

---

## 데이터 흐름 (CreateEntry 예시)

```
요청: POST /api/apps/42/entries
Body: {"title": "인허가 신청", "dueDate": "2026-05-01", "priority": "high", "isDone": false}

1. handler.CreateEntry
   └─ engine.CreateEntry(ctx, 42, data, userID)

2. engine.loadAppSchema(42)
   ├─ 캐시 히트: cachedApp 리턴
   └─ 캐시 미스: 
        SELECT FROM works_apps WHERE id=42       → table_name="wd_permit_checklist"
        SELECT FROM works_fields WHERE app_id=42 → [title(text), due_date(date), priority(select), is_done(checkbox)]
        캐시에 저장

3. engine.validateAndCoerce(schema, data)
   ├─ required 필드 체크
   ├─ 각 값 → 필드 타입으로 강제 변환
   │   ├─ "인허가 신청" → string (text)
   │   ├─ "2026-05-01"  → time.Time (date, parse)
   │   ├─ "high"        → string, select 옵션 {low,medium,high}에 존재?
   │   └─ false         → bool (checkbox)
   ├─ 알 수 없는 필드는 무시 (조용히 drop)
   └─ 반환: map[column_name]any (name → column_name 변환됨)
      {"title": "인허가 신청", "due_date": time.Time{...}, "priority": "high", "is_done": false}

4. engine.buildInsertSQL(tableName, coercedData, userID)
   SQL: INSERT INTO "wd_permit_checklist" 
        ("title","due_date","priority","is_done","_created_by")
        VALUES ($1,$2,$3,$4,$5) RETURNING id
   Args: ["인허가 신청", 2026-05-01, "high", false, 1]

5. pool.QueryRow(...).Scan(&id) → 반환
```

---

## 핵심 구현 포인트

### 1. 식별자 안전성 (`querybuilder.go`)

```go
// 컬럼명 화이트리스트 검증 후 안전하게 quote
func quoteIdent(name string) string {
    return pgx.Identifier{name}.Sanitize()
}

// 여러 컬럼 한 번에
func quoteIdents(names []string) []string {
    out := make([]string, len(names))
    for i, n := range names {
        out[i] = quoteIdent(n)
    }
    return out
}
```

**절대 금지:**
```go
sql := fmt.Sprintf("SELECT %s FROM %s", col, table)  // ❌ NEVER
```

**반드시:**
```go
sql := fmt.Sprintf("SELECT %s FROM %s", quoteIdent(col), quoteIdent(table))  // ✅
```

단, 컬럼명이 화이트리스트(works_fields에 존재)에 있는지 먼저 확인.

### 2. 타입 강제 (`validation.go`)

```go
func coerceValue(field *model.Field, raw any) (any, error) {
    if raw == nil {
        if field.IsRequired {
            return nil, fmt.Errorf("%s is required", field.Label)
        }
        return nil, nil
    }
    
    switch field.FieldType {
    case model.FieldText, model.FieldTextarea:
        s, ok := raw.(string)
        if !ok {
            return nil, fmt.Errorf("%s must be string", field.Label)
        }
        return s, nil
    
    case model.FieldNumber:
        // JSON은 숫자를 float64로 디코딩
        switch v := raw.(type) {
        case float64:
            return v, nil
        case int:
            return float64(v), nil
        case string:
            return strconv.ParseFloat(v, 64)
        }
        return nil, fmt.Errorf("%s must be number", field.Label)
    
    case model.FieldDate:
        s, ok := raw.(string)
        if !ok {
            return nil, fmt.Errorf("%s must be date string", field.Label)
        }
        // ISO 8601 또는 "YYYY-MM-DD"
        return time.Parse("2006-01-02", s)
    
    case model.FieldCheckbox:
        b, ok := raw.(bool)
        if !ok {
            return nil, fmt.Errorf("%s must be boolean", field.Label)
        }
        return b, nil
    
    case model.FieldSelect:
        s, ok := raw.(string)
        if !ok {
            return nil, fmt.Errorf("%s must be string", field.Label)
        }
        // options에 있는지 확인
        if !isValidSelectOption(field, s) {
            return nil, fmt.Errorf("%s: invalid option %q", field.Label, s)
        }
        return s, nil
    
    case model.FieldAppRef, model.FieldUserRef:
        // JSON 숫자 → int
        switch v := raw.(type) {
        case float64:
            return int(v), nil
        case int:
            return v, nil
        }
        return nil, fmt.Errorf("%s must be reference id", field.Label)
    
    case model.FieldFile:
        // 파일 URL/경로 (업로드 엔드포인트가 반환한 값)
        return raw.(string), nil
    }
    
    return nil, fmt.Errorf("unknown field type: %s", field.FieldType)
}
```

### 3. 필터 빌더 (`querybuilder.go`)

```go
type filterClause struct {
    sql  string   // "due_date >= $1"
    args []any    // [time.Time{...}]
}

func buildWhereClause(schema *cachedApp, filters []model.Filter, argOffset int) (*filterClause, error) {
    if len(filters) == 0 {
        return &filterClause{sql: "", args: nil}, nil
    }
    
    var parts []string
    var args []any
    argIdx := argOffset
    
    for _, f := range filters {
        field, ok := schema.byName[f.Field]
        if !ok {
            return nil, fmt.Errorf("unknown field: %s", f.Field)
        }
        
        col := quoteIdent(field.ColumnName)
        argIdx++
        
        switch f.Operator {
        case "eq":
            parts = append(parts, fmt.Sprintf("%s = $%d", col, argIdx))
            args = append(args, f.Value)
        case "neq":
            parts = append(parts, fmt.Sprintf("%s != $%d", col, argIdx))
            args = append(args, f.Value)
        case "gt":
            parts = append(parts, fmt.Sprintf("%s > $%d", col, argIdx))
            args = append(args, f.Value)
        case "lt":
            parts = append(parts, fmt.Sprintf("%s < $%d", col, argIdx))
            args = append(args, f.Value)
        case "gte":
            parts = append(parts, fmt.Sprintf("%s >= $%d", col, argIdx))
            args = append(args, f.Value)
        case "lte":
            parts = append(parts, fmt.Sprintf("%s <= $%d", col, argIdx))
            args = append(args, f.Value)
        case "contains":
            parts = append(parts, fmt.Sprintf("%s ILIKE $%d", col, argIdx))
            args = append(args, "%"+fmt.Sprint(f.Value)+"%")
        case "in":
            // ANY($1::text[]) 패턴
            parts = append(parts, fmt.Sprintf("%s = ANY($%d)", col, argIdx))
            args = append(args, f.Value)
        default:
            return nil, fmt.Errorf("unknown operator: %s", f.Operator)
        }
    }
    
    return &filterClause{
        sql:  "WHERE " + strings.Join(parts, " AND "),
        args: args,
    }, nil
}
```

### 4. QueryEntries 조립

```go
func (e *Engine) QueryEntries(ctx context.Context, appID int, q model.QueryParams) (*model.PagedResult, error) {
    schema, err := e.loadAppSchema(ctx, appID)
    if err != nil {
        return nil, err
    }
    
    // 1. WHERE
    where, err := buildWhereClause(schema, q.Filters, 0)
    if err != nil {
        return nil, err
    }
    
    // 2. ORDER BY
    orderBy := "_created_at DESC"  // 기본
    if q.Sort != nil {
        field, ok := schema.byName[q.Sort.Field]
        if !ok {
            return nil, fmt.Errorf("unknown sort field: %s", q.Sort.Field)
        }
        dir := "ASC"
        if strings.ToLower(q.Sort.Direction) == "desc" {
            dir = "DESC"
        }
        orderBy = fmt.Sprintf("%s %s", quoteIdent(field.ColumnName), dir)
    }
    
    // 3. LIMIT/OFFSET
    if q.Limit <= 0 || q.Limit > 200 {
        q.Limit = 50
    }
    if q.Page < 1 {
        q.Page = 1
    }
    offset := (q.Page - 1) * q.Limit
    
    // 4. COUNT(*) for total
    countSQL := fmt.Sprintf("SELECT COUNT(*) FROM %s %s", quoteIdent(schema.tableName), where.sql)
    var total int
    if err := e.pool.QueryRow(ctx, countSQL, where.args...).Scan(&total); err != nil {
        return nil, err
    }
    
    // 5. SELECT with pagination
    cols := []string{"id", "_created_by", "_created_at", "_updated_at"}
    for _, f := range schema.fields {
        cols = append(cols, quoteIdent(f.ColumnName))
    }
    selectSQL := fmt.Sprintf(
        "SELECT %s FROM %s %s ORDER BY %s LIMIT $%d OFFSET $%d",
        strings.Join(cols, ","),
        quoteIdent(schema.tableName),
        where.sql,
        orderBy,
        len(where.args)+1,
        len(where.args)+2,
    )
    args := append(where.args, q.Limit, offset)
    
    rows, err := e.pool.Query(ctx, selectSQL, args...)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    
    // 6. Scan into []map[string]any
    data := make([]map[string]any, 0, q.Limit)
    for rows.Next() {
        values, err := rows.Values()
        if err != nil {
            return nil, err
        }
        row := make(map[string]any, len(cols))
        // cols는 여기서 "id", "_created_by" 같은 quoted 문자열이므로 언쿼트 필요
        // 실제로는 schema.fields 순서대로 매핑하는 것이 안전
        // ... (구현 시 주의)
        _ = values
        data = append(data, row)
    }
    
    return &model.PagedResult{
        Data:       data,
        Total:      total,
        Page:       q.Page,
        Limit:      q.Limit,
        TotalPages: (total + q.Limit - 1) / q.Limit,
    }, nil
}
```

### 5. 스키마 캐시 (`cache.go`)

```go
func (e *Engine) loadAppSchema(ctx context.Context, appID int) (*cachedApp, error) {
    e.cache.mu.RLock()
    if c, ok := e.cache.apps[appID]; ok {
        e.cache.mu.RUnlock()
        return c, nil
    }
    e.cache.mu.RUnlock()
    
    // Load from DB
    e.cache.mu.Lock()
    defer e.cache.mu.Unlock()
    
    // Double-check (race 방지)
    if c, ok := e.cache.apps[appID]; ok {
        return c, nil
    }
    
    var tableName string
    err := e.pool.QueryRow(ctx, `SELECT table_name FROM works_apps WHERE id = $1`, appID).
        Scan(&tableName)
    if err != nil {
        return nil, err
    }
    
    rows, err := e.pool.Query(ctx,
        `SELECT id, app_id, name, label, field_type, column_name, is_required, options, position, width
         FROM works_fields WHERE app_id = $1 ORDER BY position`, appID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    
    var fields []model.Field
    byName := make(map[string]*model.Field)
    byColumn := make(map[string]*model.Field)
    
    for rows.Next() {
        var f model.Field
        if err := rows.Scan(&f.ID, &f.AppID, &f.Name, &f.Label, &f.FieldType,
            &f.ColumnName, &f.IsRequired, &f.Options, &f.Position, &f.Width); err != nil {
            return nil, err
        }
        fields = append(fields, f)
    }
    
    // 주소 안정성을 위해 fields는 append 완료 후에 byName/byColumn에 등록
    for i := range fields {
        byName[fields[i].Name] = &fields[i]
        byColumn[fields[i].ColumnName] = &fields[i]
    }
    
    c := &cachedApp{
        tableName: tableName,
        fields:    fields,
        byName:    byName,
        byColumn:  byColumn,
        loadedAt:  time.Now(),
    }
    e.cache.apps[appID] = c
    return c, nil
}

func (e *Engine) InvalidateSchema(appID int) {
    e.cache.mu.Lock()
    delete(e.cache.apps, appID)
    e.cache.mu.Unlock()
}
```

---

## 체크리스트 (완료 기준)

### 기능
- [ ] `CreateEntry` — 값 검증, 타입 강제, INSERT, 트랜잭션 없음 (단일 쿼리)
- [ ] `UpdateEntry` — 부분 업데이트 (data에 있는 필드만), `_updated_at = now()` 자동 갱신
- [ ] `DeleteEntry` — DELETE WHERE id, 없으면 `apierr.NotFound`
- [ ] `GetEntry` — 단일 조회, nil 리턴 가능
- [ ] `QueryEntries` — 필터, 정렬, 페이지네이션, 총 건수
- [ ] `AggregateEntries` — GROUP BY + COUNT (Kanban 뷰용)
- [ ] 스키마 캐시 + `InvalidateSchema`

### 보안
- [ ] 모든 컬럼명이 `works_fields`에서 검증됨
- [ ] 테이블명이 `works_apps.table_name`에서만 옴
- [ ] 모든 값이 `$1, $2` 파라미터 바인딩
- [ ] `fmt.Sprintf("... %s ...", userInput)` 없음
- [ ] `pgx.Identifier.Sanitize()` 사용

### 검증
- [ ] required 필드 누락 시 400
- [ ] 타입 불일치 시 400 (문자열 상세 포함)
- [ ] select 옵션 화이트리스트 검증
- [ ] 알 수 없는 필드는 조용히 drop (에러 X)
- [ ] 날짜 파싱 실패 시 400
- [ ] app-ref/user-ref의 FK 존재 여부는 DB 제약에 위임 (엔진에서 확인 X)

### 성능
- [ ] 스키마 캐시 동작 (첫 호출만 DB 조회)
- [ ] `Limit` 기본값 50, 상한 200
- [ ] `Page` 0 이하는 1로 보정
- [ ] COUNT(*) + SELECT 병렬 실행 고려 (선택)
- [ ] `_created_at` 인덱스 활용 (마이그레이션에 이미 있음)

### 에러 처리
- [ ] DB 연결 실패 → 500 + 로깅
- [ ] 존재하지 않는 appID → 404
- [ ] 존재하지 않는 entryID → 404
- [ ] 검증 실패 → 400 (상세 메시지)
- [ ] `apierr` 패키지 일관 사용

### 테스트
- [ ] `TestCreateEntry_happy_path`
- [ ] `TestCreateEntry_missing_required`
- [ ] `TestCreateEntry_invalid_type`
- [ ] `TestCreateEntry_invalid_select_option`
- [ ] `TestQueryEntries_filter_eq`
- [ ] `TestQueryEntries_filter_contains`
- [ ] `TestQueryEntries_sort`
- [ ] `TestQueryEntries_pagination`
- [ ] `TestSchemaCache_invalidation`

---

## 핸들러 연결 (이미 작성됨)

`handler/entries.go`에 이미 다음이 있음:
- `CreateEntry(eng)` — body JSON → `eng.CreateEntry`
- `QueryEntries(eng)` — 쿼리스트링 파싱 TODO
- `GetEntry`, `UpdateEntry`, `DeleteEntry` — 그대로 사용

핸들러에서 해야 할 추가 작업:
1. `QueryEntries` 핸들러에서 쿼리스트링 파싱 (`?filter[status]=done&sort=dueDate:asc&page=2&limit=20`)
2. `apierr.Validation` 등으로 엔진 에러 매핑

---

## Schema Engine과의 협업 포인트

Schema Engine이 다음 함수를 성공적으로 실행한 직후 **반드시** `eng.InvalidateSchema(appID)` 호출:

- `CreateApp` — 새 앱, 캐시에 없으니 무효화 불필요하지만 안전하게
- `AddField` — 필드 추가, 캐시 무효화 필수
- `UpdateField` — 라벨/옵션 변경, 무효화 필수 (옵션은 검증에 사용됨)
- `RemoveField` — 필드 삭제, 무효화 필수
- `DeleteApp` — 앱 삭제, 무효화 필수

이 호출이 빠지면 오래된 스키마로 데이터 검증을 계속하게 됨.

---

## 확장 포인트 (Phase 2)

- **Full-text search**: 컬럼에 `tsvector` 추가, `contains` 연산자를 `@@` 로 업그레이드
- **관계 조인**: `app-ref` 필드가 있을 때 자동 JOIN 옵션 (`?expand=projectId`)
- **집계 함수**: `AggregateEntries`를 AVG, SUM, MIN, MAX로 확장 (리포트 뷰)
- **Soft delete**: `_deleted_at` 컬럼 추가 고려 (현재는 hard delete)
- **Audit log**: UPDATE/DELETE 시 이전 값 별도 테이블 기록 (`works_audit`)
