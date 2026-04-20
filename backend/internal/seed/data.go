package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/choiceoh/phaeton/backend/internal/schema"
)

// SeedData inserts sample records, views, automations, and charts
// into preset collections. Idempotent: skips if records already exist.
func SeedData(ctx context.Context, pool *pgxpool.Pool, store *schema.Store, cache *schema.Cache) error {
	// Get admin user ID
	var userID string
	err := pool.QueryRow(ctx, `SELECT id FROM auth.users WHERE email = 'choiceoh@topsolar.kr'`).Scan(&userID)
	if err != nil {
		return fmt.Errorf("get admin user: %w", err)
	}

	// ── 1. 프로젝트 레코드 ──
	projects, ok := cache.CollectionBySlug("projects")
	if !ok {
		return fmt.Errorf("projects collection not found")
	}
	projectIDs, err := seedRecords(ctx, pool, "projects", userID, []map[string]any{
		{"name": "영암 태양광 1단지", "project_type": "solar", "capacity_kw": 9800, "region": "전남 영암", "status": "construction", "cod_target": "2026-09-15"},
		{"name": "새만금 풍력 발전", "project_type": "wind", "capacity_kw": 30000, "region": "전북 군산", "status": "permit", "cod_target": "2027-03-01"},
		{"name": "제주 ESS 연계", "project_type": "ess", "capacity_kw": 5000, "region": "제주", "status": "planning", "cod_target": "2027-06-30"},
		{"name": "해남 하이브리드", "project_type": "hybrid", "capacity_kw": 15000, "region": "전남 해남", "status": "testing", "cod_target": "2026-05-20"},
		{"name": "당진 태양광 2단지", "project_type": "solar", "capacity_kw": 4500, "region": "충남 당진", "status": "cod", "cod_target": "2025-12-01"},
		{"name": "서산 태양광", "project_type": "solar", "capacity_kw": 7200, "region": "충남 서산", "status": "construction", "cod_target": "2026-11-30"},
		{"name": "포항 풍력", "project_type": "wind", "capacity_kw": 20000, "region": "경북 포항", "status": "planning", "cod_target": "2028-01-15"},
	})
	if err != nil {
		return fmt.Errorf("seed projects: %w", err)
	}

	// ── 2. 마일스톤 레코드 ──
	if _, ok := cache.CollectionBySlug("milestones"); ok && len(projectIDs) >= 4 {
		_, err = seedRecords(ctx, pool, "milestones", userID, []map[string]any{
			{"name": "인허가 접수", "seq_order": 1, "status": "done", "due_date": "2025-11-01", "is_critical": true, "project": projectIDs[0]},
			{"name": "토목 공사", "seq_order": 2, "status": "active", "due_date": "2026-03-01", "is_critical": true, "project": projectIDs[0]},
			{"name": "모듈 설치", "seq_order": 3, "status": "pending", "due_date": "2026-06-15", "is_critical": true, "project": projectIDs[0]},
			{"name": "계통 연계", "seq_order": 4, "status": "pending", "due_date": "2026-08-30", "is_critical": true, "project": projectIDs[0]},
			{"name": "환경영향평가", "seq_order": 1, "status": "active", "due_date": "2026-06-01", "is_critical": true, "project": projectIDs[1]},
			{"name": "부지 조성", "seq_order": 2, "status": "pending", "due_date": "2026-12-01", "is_critical": false, "project": projectIDs[1]},
			{"name": "기본 설계", "seq_order": 1, "status": "done", "due_date": "2026-01-15", "is_critical": false, "project": projectIDs[2]},
			{"name": "시운전", "seq_order": 1, "status": "active", "due_date": "2026-04-30", "is_critical": true, "project": projectIDs[3]},
			{"name": "준공 검사", "seq_order": 2, "status": "pending", "due_date": "2026-05-15", "is_critical": true, "project": projectIDs[3]},
		})
		if err != nil {
			return fmt.Errorf("seed milestones: %w", err)
		}
	}

	// ── 3. 인력 배치 레코드 ──
	if _, ok := cache.CollectionBySlug("staff"); ok && len(projectIDs) >= 4 {
		_, err = seedRecords(ctx, pool, "staff", userID, []map[string]any{
			{"name": "김태양", "role": "PM", "start_date": "2025-09-01", "allocation_pct": 100, "is_active": true, "project": projectIDs[0]},
			{"name": "이풍력", "role": "설계 엔지니어", "start_date": "2025-10-01", "allocation_pct": 80, "is_active": true, "project": projectIDs[1]},
			{"name": "박전기", "role": "전기 기사", "start_date": "2026-01-15", "allocation_pct": 100, "is_active": true, "project": projectIDs[0]},
			{"name": "최구조", "role": "토목 기사", "start_date": "2025-11-01", "end_date": "2026-06-30", "allocation_pct": 60, "is_active": true, "project": projectIDs[0]},
			{"name": "정안전", "role": "안전 관리자", "start_date": "2025-12-01", "allocation_pct": 50, "is_active": true, "project": projectIDs[3]},
			{"name": "한검수", "role": "QA", "start_date": "2026-02-01", "allocation_pct": 40, "is_active": false},
		})
		if err != nil {
			return fmt.Errorf("seed staff: %w", err)
		}
	}

	// ── 4. 고객 관리 레코드 ──
	if _, ok := cache.CollectionBySlug("customers"); ok {
		_, err = seedRecords(ctx, pool, "customers", userID, []map[string]any{
			{"name": "한국전력공사", "company": "한국전력공사", "email": "kepco@kepco.co.kr", "phone": "061-345-3114", "grade": "vip", "is_active": true},
			{"name": "에너지관리공단", "company": "한국에너지공단", "email": "info@energy.or.kr", "phone": "052-920-0114", "grade": "gold", "is_active": true},
			{"name": "태양에너지(주)", "company": "태양에너지", "email": "sales@sunpower.kr", "phone": "02-555-1234", "grade": "gold", "is_active": true},
			{"name": "그린파워텍", "company": "그린파워텍", "email": "gpt@greenpower.co.kr", "phone": "031-777-5678", "grade": "silver", "is_active": true},
			{"name": "윈드코리아", "company": "윈드코리아(주)", "email": "biz@windkorea.com", "phone": "033-444-9999", "grade": "normal", "is_active": false, "note": "2025년 계약 종료"},
		})
		if err != nil {
			return fmt.Errorf("seed customers: %w", err)
		}
	}

	// ── 5. 재고 관리 레코드 ──
	if _, ok := cache.CollectionBySlug("inventory"); ok {
		_, err = seedRecords(ctx, pool, "inventory", userID, []map[string]any{
			{"item_name": "태양광 모듈 550W", "category": "component", "sku": "PV-550W-001", "quantity": 1200, "unit_price": 185000, "location": "영암 창고 A", "min_stock": 200, "last_checked": "2026-04-08"},
			{"item_name": "인버터 100kW", "category": "equipment", "sku": "INV-100K-001", "quantity": 15, "unit_price": 12500000, "location": "본사 창고", "min_stock": 3, "last_checked": "2026-04-05"},
			{"item_name": "케이블 XLPE 22.9kV", "category": "raw_material", "sku": "CBL-229-001", "quantity": 5000, "unit_price": 25000, "location": "영암 창고 B", "min_stock": 500, "last_checked": "2026-04-07"},
			{"item_name": "접속함 16구", "category": "component", "sku": "JB-16P-001", "quantity": 80, "unit_price": 350000, "location": "본사 창고", "min_stock": 20, "last_checked": "2026-04-01"},
			{"item_name": "볼트 세트 M12", "category": "consumable", "sku": "BLT-M12-001", "quantity": 3000, "unit_price": 500, "location": "영암 창고 A", "min_stock": 500, "last_checked": "2026-03-28"},
			{"item_name": "ESS 배터리 모듈 280Ah", "category": "component", "sku": "ESS-280A-001", "quantity": 50, "unit_price": 4800000, "location": "제주 창고", "min_stock": 10, "last_checked": "2026-04-09"},
		})
		if err != nil {
			return fmt.Errorf("seed inventory: %w", err)
		}
	}

	// ── 6. 앱 요청 레코드 ──
	if _, ok := cache.CollectionBySlug("requests"); ok {
		_, err = seedRecords(ctx, pool, "requests", userID, []map[string]any{
			{"title": "영암 현장 인터넷 설치", "category": "it", "priority": "high", "status": "in_progress", "requester": "김태양", "description": "영암 1단지 현장 사무소에 인터넷 회선 설치 필요", "due_date": "2026-04-15"},
			{"title": "안전모 100개 추가 구매", "category": "purchase", "priority": "normal", "status": "open", "requester": "정안전", "description": "하반기 공사 인력 증가 대비 안전모 추가 구매", "due_date": "2026-04-20"},
			{"title": "신규 직원 계정 발급", "category": "hr", "priority": "urgent", "status": "resolved", "requester": "한검수", "description": "4월 입사자 3명 시스템 계정 발급", "due_date": "2026-04-05", "completed_at": "2026-04-04T14:30:00+09:00"},
			{"title": "사무실 에어컨 점검", "category": "facility", "priority": "low", "status": "open", "requester": "최구조", "description": "여름 대비 본사 사무실 에어컨 사전 점검", "due_date": "2026-05-01"},
			{"title": "프로젝트 관리 소프트웨어 갱신", "category": "it", "priority": "normal", "status": "closed", "requester": "이풍력", "description": "MS Project 라이선스 갱신", "due_date": "2026-03-31", "completed_at": "2026-03-28T10:00:00+09:00"},
		})
		if err != nil {
			return fmt.Errorf("seed requests: %w", err)
		}
	}

	// ── 7. 비용 정산 레코드 ──
	if _, ok := cache.CollectionBySlug("expenses"); ok {
		_, err = seedRecords(ctx, pool, "expenses", userID, []map[string]any{
			{"title": "영암 현장 출장비", "category": "travel", "amount": 285000, "spent_date": "2026-04-02", "status": "approved"},
			{"title": "현장 중식대 (15명)", "category": "meal", "amount": 150000, "spent_date": "2026-04-03", "status": "approved"},
			{"title": "측량 장비 렌탈", "category": "equipment", "amount": 1500000, "spent_date": "2026-03-25", "status": "approved"},
			{"title": "사무용품 구입", "category": "supplies", "amount": 87000, "spent_date": "2026-04-07", "status": "submitted"},
			{"title": "전기안전검사 수수료", "category": "service", "amount": 550000, "spent_date": "2026-04-01", "status": "approved"},
			{"title": "해남 현장 출장비", "category": "travel", "amount": 320000, "spent_date": "2026-04-08", "status": "submitted"},
			{"title": "기술 교육비", "category": "service", "amount": 2000000, "spent_date": "2026-03-20", "status": "approved"},
			{"title": "접대비", "category": "meal", "amount": 450000, "spent_date": "2026-04-05", "status": "draft", "note": "한전 담당자 식사"},
		})
		if err != nil {
			return fmt.Errorf("seed expenses: %w", err)
		}
	}

	// ── 8. 회의록 레코드 ──
	if _, ok := cache.CollectionBySlug("meetings"); ok {
		_, err = seedRecords(ctx, pool, "meetings", userID, []map[string]any{
			{
				"title": "영암 1단지 주간 공정 회의", "meeting_date": "2026-04-07T10:00:00+09:00",
				"location": "영암 현장 사무소", "attendees": "김태양, 박전기, 최구조",
				"agenda":       "1. 주간 공정률 확인\n2. 자재 입고 일정\n3. 안전 점검 결과",
				"decisions":    "자재 입고 일정 1주 앞당김, 안전 교육 추가 실시",
				"action_items": "- 자재 발주 확인 (박전기, 4/9)\n- 안전교육 일정 수립 (정안전, 4/10)",
			},
			{
				"title": "2분기 사업 계획 검토", "meeting_date": "2026-04-01T14:00:00+09:00",
				"location": "본사 대회의실", "attendees": "관리자, 김태양, 이풍력, 정안전",
				"agenda":       "1. 1분기 실적 리뷰\n2. 2분기 목표 설정\n3. 신규 프로젝트 검토",
				"decisions":    "포항 풍력 프로젝트 타당성 조사 착수",
				"action_items": "- 포항 풍력 사전조사 보고서 (이풍력, 4/30)",
			},
		})
		if err != nil {
			return fmt.Errorf("seed meetings: %w", err)
		}
	}

	// ── 9. 뷰 생성 ──
	if err := seedViews(ctx, store, cache, projects.ID); err != nil {
		return fmt.Errorf("seed views: %w", err)
	}

	// ── 10. 자동화 생성 ──
	if err := seedAutomations(ctx, pool, cache, userID); err != nil {
		return fmt.Errorf("seed automations: %w", err)
	}

	// ── 11. 차트 생성 ──
	if err := seedCharts(ctx, store, cache, userID); err != nil {
		return fmt.Errorf("seed charts: %w", err)
	}

	slog.Info("seed: sample data seeded")
	return nil
}

// seedRecords inserts rows into a dynamic table. Returns inserted IDs.
// Skips if the table already has data.
func seedRecords(ctx context.Context, pool *pgxpool.Pool, slug, userID string, rows []map[string]any) ([]string, error) {
	// Check if data already exists
	var count int
	err := pool.QueryRow(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM "data".%q`, slug)).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("count %s: %w", slug, err)
	}
	if count > 0 {
		// Return existing IDs for relation wiring
		existingRows, err := pool.Query(ctx, fmt.Sprintf(`SELECT id FROM "data".%q ORDER BY created_at`, slug))
		if err != nil {
			return nil, err
		}
		defer existingRows.Close()
		var ids []string
		for existingRows.Next() {
			var id string
			if err := existingRows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		slog.Info("seed: data exists, skipping", "slug", slug, "count", count)
		return ids, existingRows.Err()
	}

	var ids []string
	for _, row := range rows {
		cols := `"created_by"`
		vals := "$1"
		args := []any{userID}
		idx := 2

		for k, v := range row {
			cols += fmt.Sprintf(", %q", k)
			vals += fmt.Sprintf(", $%d", idx)
			args = append(args, v)
			idx++
		}

		sql := fmt.Sprintf(`INSERT INTO "data".%q (%s) VALUES (%s) RETURNING id`, slug, cols, vals)
		var id string
		if err := pool.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
			return nil, fmt.Errorf("insert %s: %w", slug, err)
		}
		ids = append(ids, id)
	}

	slog.Info("seed: inserted records", "slug", slug, "count", len(rows))
	return ids, nil
}

// seedViews creates view configurations for key collections.
func seedViews(ctx context.Context, store *schema.Store, cache *schema.Cache, projectsID string) error {
	type viewSeed struct {
		slug  string
		views []schema.CreateViewReq
	}

	seeds := []viewSeed{
		{
			slug: "projects",
			views: []schema.CreateViewReq{
				{Name: "목록", ViewType: "list", SortOrder: 0, IsDefault: true},
				{Name: "상태별 칸반", ViewType: "kanban", SortOrder: 1, Config: jsonRaw(map[string]any{
					"groupBy": "status",
				})},
				{Name: "COD 캘린더", ViewType: "calendar", SortOrder: 2, Config: jsonRaw(map[string]any{
					"dateField": "cod_target",
				})},
				{Name: "갤러리", ViewType: "gallery", SortOrder: 3},
			},
		},
		{
			slug: "requests",
			views: []schema.CreateViewReq{
				{Name: "전체 목록", ViewType: "list", SortOrder: 0, IsDefault: true},
				{Name: "상태별 보드", ViewType: "kanban", SortOrder: 1, Config: jsonRaw(map[string]any{
					"groupBy": "status",
				})},
				{Name: "기한 캘린더", ViewType: "calendar", SortOrder: 2, Config: jsonRaw(map[string]any{
					"dateField": "due_date",
				})},
			},
		},
		{
			slug: "expenses",
			views: []schema.CreateViewReq{
				{Name: "전체 내역", ViewType: "list", SortOrder: 0, IsDefault: true},
				{Name: "상태별 보드", ViewType: "kanban", SortOrder: 1, Config: jsonRaw(map[string]any{
					"groupBy": "status",
				})},
			},
		},
		{
			slug: "milestones",
			views: []schema.CreateViewReq{
				{Name: "전체 목록", ViewType: "list", SortOrder: 0, IsDefault: true},
				{Name: "진행 상태 보드", ViewType: "kanban", SortOrder: 1, Config: jsonRaw(map[string]any{
					"groupBy": "status",
				})},
			},
		},
		{
			slug: "inventory",
			views: []schema.CreateViewReq{
				{Name: "재고 목록", ViewType: "list", SortOrder: 0, IsDefault: true},
				{Name: "분류별 갤러리", ViewType: "gallery", SortOrder: 1},
			},
		},
	}

	for _, s := range seeds {
		col, ok := cache.CollectionBySlug(s.slug)
		if !ok {
			continue
		}

		existing, err := store.ListViews(ctx, col.ID)
		if err != nil {
			return fmt.Errorf("list views %s: %w", s.slug, err)
		}
		if len(existing) > 0 {
			slog.Info("seed: views exist, skipping", "slug", s.slug)
			continue
		}

		for _, v := range s.views {
			if _, err := store.CreateView(ctx, col.ID, &v); err != nil {
				return fmt.Errorf("create view %s/%s: %w", s.slug, v.Name, err)
			}
		}
		slog.Info("seed: created views", "slug", s.slug, "count", len(s.views))
	}

	return nil
}

// seedAutomations creates automation rules for key collections.
func seedAutomations(ctx context.Context, pool *pgxpool.Pool, cache *schema.Cache, userID string) error {
	type conditionSeed struct {
		fieldSlug string
		operator  string
		value     string
	}
	type actionSeed struct {
		actionType string
		config     json.RawMessage
	}
	type automationSeed struct {
		collectionSlug string
		name           string
		triggerType    string
		triggerConfig  json.RawMessage
		conditions     []conditionSeed
		actions        []actionSeed
	}

	seeds := []automationSeed{
		{
			collectionSlug: "requests",
			name:           "긴급 요청 알림",
			triggerType:    "record_created",
			triggerConfig:  json.RawMessage(`{}`),
			conditions: []conditionSeed{
				{fieldSlug: "priority", operator: "equals", value: "urgent"},
			},
			actions: []actionSeed{
				{
					actionType: "send_notification",
					config: jsonRaw(map[string]any{
						"recipient": "record_creator",
						"title":     "긴급 요청 접수",
						"body":      "긴급 우선순위 요청이 등록되었습니다. 즉시 확인해주세요.",
					}),
				},
			},
		},
		{
			collectionSlug: "requests",
			name:           "요청 완료 시 상태 업데이트",
			triggerType:    "status_change",
			triggerConfig: jsonRaw(map[string]any{
				"field_slug":  "status",
				"from_status": "in_progress",
				"to_status":   "resolved",
			}),
			conditions: nil,
			actions: []actionSeed{
				{
					actionType: "send_notification",
					config: jsonRaw(map[string]any{
						"recipient": "record_creator",
						"title":     "요청 처리 완료",
						"body":      "요청하신 건이 처리 완료되었습니다.",
					}),
				},
			},
		},
		{
			collectionSlug: "expenses",
			name:           "고액 경비 알림",
			triggerType:    "record_created",
			triggerConfig:  json.RawMessage(`{}`),
			conditions: []conditionSeed{
				{fieldSlug: "amount", operator: "gt", value: "1000000"},
			},
			actions: []actionSeed{
				{
					actionType: "send_notification",
					config: jsonRaw(map[string]any{
						"recipient": "record_creator",
						"title":     "고액 경비 등록",
						"body":      "100만원 이상의 경비가 등록되었습니다. 관리자 승인이 필요합니다.",
					}),
				},
			},
		},
		{
			collectionSlug: "projects",
			name:           "프로젝트 상태 변경 알림",
			triggerType:    "record_updated",
			triggerConfig:  json.RawMessage(`{}`),
			conditions: []conditionSeed{
				{fieldSlug: "status", operator: "is_not_empty", value: ""},
			},
			actions: []actionSeed{
				{
					actionType: "send_notification",
					config: jsonRaw(map[string]any{
						"recipient": "record_creator",
						"title":     "프로젝트 업데이트",
						"body":      "프로젝트 정보가 변경되었습니다.",
					}),
				},
			},
		},
	}

	for _, s := range seeds {
		col, ok := cache.CollectionBySlug(s.collectionSlug)
		if !ok {
			continue
		}

		// Check if automations already exist
		var count int
		err := pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM _meta.automations WHERE collection_id = $1`, col.ID,
		).Scan(&count)
		if err != nil {
			return fmt.Errorf("count automations %s: %w", s.collectionSlug, err)
		}
		if count > 0 {
			continue
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}

		var automationID string
		err = tx.QueryRow(ctx, `
			INSERT INTO _meta.automations (collection_id, name, is_enabled, trigger_type, trigger_config, created_by)
			VALUES ($1, $2, true, $3, $4, $5)
			RETURNING id`,
			col.ID, s.name, s.triggerType, s.triggerConfig, userID,
		).Scan(&automationID)
		if err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("insert automation %s: %w", s.name, err)
		}

		for i, c := range s.conditions {
			_, err := tx.Exec(ctx, `
				INSERT INTO _meta.automation_conditions (automation_id, field_slug, operator, value, sort_order)
				VALUES ($1, $2, $3, $4, $5)`,
				automationID, c.fieldSlug, c.operator, c.value, i,
			)
			if err != nil {
				tx.Rollback(ctx)
				return fmt.Errorf("insert condition: %w", err)
			}
		}

		for i, a := range s.actions {
			_, err := tx.Exec(ctx, `
				INSERT INTO _meta.automation_actions (automation_id, action_type, action_config, sort_order)
				VALUES ($1, $2, $3, $4)`,
				automationID, a.actionType, a.config, i,
			)
			if err != nil {
				tx.Rollback(ctx)
				return fmt.Errorf("insert action: %w", err)
			}
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit automation %s: %w", s.name, err)
		}
		slog.Info("seed: created automation", "collection", s.collectionSlug, "name", s.name)
	}

	return nil
}

// seedCharts creates chart configurations for key collections.
func seedCharts(ctx context.Context, store *schema.Store, cache *schema.Cache, userID string) error {
	type chartSeed struct {
		slug   string
		charts []schema.CreateChartReq
	}

	seeds := []chartSeed{
		{
			slug: "projects",
			charts: []schema.CreateChartReq{
				{
					Name: "유형별 프로젝트 수", ChartType: "pie", SortOrder: 0,
					Config: jsonRaw(map[string]any{
						"labelField": "project_type",
						"aggregate":  "count",
					}),
				},
				{
					Name: "상태별 용량(kW)", ChartType: "bar", SortOrder: 1,
					Config: jsonRaw(map[string]any{
						"labelField": "status",
						"dataField":  "capacity_kw",
						"aggregate":  "sum",
					}),
				},
				{
					Name: "지역별 용량 분포", ChartType: "doughnut", SortOrder: 2,
					Config: jsonRaw(map[string]any{
						"labelField": "region",
						"dataField":  "capacity_kw",
						"aggregate":  "sum",
					}),
				},
			},
		},
		{
			slug: "expenses",
			charts: []schema.CreateChartReq{
				{
					Name: "분류별 지출", ChartType: "pie", SortOrder: 0,
					Config: jsonRaw(map[string]any{
						"labelField": "category",
						"dataField":  "amount",
						"aggregate":  "sum",
					}),
				},
				{
					Name: "월별 지출 추이", ChartType: "line", SortOrder: 1,
					Config: jsonRaw(map[string]any{
						"dateField": "spent_date",
						"dataField": "amount",
						"aggregate": "sum",
						"groupBy":   "month",
					}),
				},
				{
					Name: "상태별 건수", ChartType: "bar", SortOrder: 2,
					Config: jsonRaw(map[string]any{
						"labelField": "status",
						"aggregate":  "count",
					}),
				},
			},
		},
		{
			slug: "requests",
			charts: []schema.CreateChartReq{
				{
					Name: "분류별 요청", ChartType: "pie", SortOrder: 0,
					Config: jsonRaw(map[string]any{
						"labelField": "category",
						"aggregate":  "count",
					}),
				},
				{
					Name: "우선순위별 현황", ChartType: "bar", SortOrder: 1,
					Config: jsonRaw(map[string]any{
						"labelField": "priority",
						"aggregate":  "count",
					}),
				},
			},
		},
		{
			slug: "inventory",
			charts: []schema.CreateChartReq{
				{
					Name: "분류별 재고 금액", ChartType: "bar", SortOrder: 0,
					Config: jsonRaw(map[string]any{
						"labelField": "category",
						"dataField":  "unit_price",
						"aggregate":  "sum",
					}),
				},
				{
					Name: "분류별 품목 수", ChartType: "doughnut", SortOrder: 1,
					Config: jsonRaw(map[string]any{
						"labelField": "category",
						"aggregate":  "count",
					}),
				},
			},
		},
	}

	for _, s := range seeds {
		col, ok := cache.CollectionBySlug(s.slug)
		if !ok {
			continue
		}

		existing, err := store.ListCharts(ctx, col.ID)
		if err != nil {
			return fmt.Errorf("list charts %s: %w", s.slug, err)
		}
		if len(existing) > 0 {
			slog.Info("seed: charts exist, skipping", "slug", s.slug)
			continue
		}

		for _, c := range s.charts {
			if _, err := store.CreateChart(ctx, col.ID, c, userID); err != nil {
				return fmt.Errorf("create chart %s/%s: %w", s.slug, c.Name, err)
			}
		}
		slog.Info("seed: created charts", "slug", s.slug, "count", len(s.charts))
	}

	return nil
}
