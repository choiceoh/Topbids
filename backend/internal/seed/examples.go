package seed

import "github.com/choiceoh/phaeton/backend/internal/schema"

// ── 고객 관리 (CRM) ──────────────────────────────────────────────

func customersPreset() Preset {
	return Preset{
		Slug:        "customers",
		Label:       "고객 관리",
		Description: "거래처·고객 정보 및 등급 관리",
		Icon:        "users",
		Fields: []schema.CreateFieldIn{
			{Slug: "name", Label: "고객명", FieldType: schema.FieldText, IsRequired: true, IsIndexed: true},
			{Slug: "company", Label: "회사명", FieldType: schema.FieldText, IsIndexed: true},
			{Slug: "email", Label: "이메일", FieldType: schema.FieldText, IsUnique: true},
			{Slug: "phone", Label: "연락처", FieldType: schema.FieldText},
			{
				Slug: "grade", Label: "등급", FieldType: schema.FieldSelect, IsIndexed: true,
				Options: jsonRaw(map[string]any{
					"choices": []string{"vip", "gold", "silver", "normal"},
				}),
			},
			{Slug: "note", Label: "메모", FieldType: schema.FieldTextarea},
			{Slug: "is_active", Label: "활성", FieldType: schema.FieldBoolean},
		},
	}
}

// ── 재고 관리 ────────────────────────────────────────────────────

func inventoryPreset() Preset {
	return Preset{
		Slug:        "inventory",
		Label:       "재고 관리",
		Description: "자재·부품 재고 수량 및 입출고 관리",
		Icon:        "box",
		Fields: []schema.CreateFieldIn{
			{Slug: "item_name", Label: "품목명", FieldType: schema.FieldText, IsRequired: true, IsIndexed: true},
			{
				Slug: "category", Label: "분류", FieldType: schema.FieldSelect, IsIndexed: true,
				Options: jsonRaw(map[string]any{
					"choices": []string{"raw_material", "component", "consumable", "equipment", "other"},
				}),
			},
			{Slug: "sku", Label: "품번", FieldType: schema.FieldText, IsUnique: true},
			{Slug: "quantity", Label: "수량", FieldType: schema.FieldInteger},
			{Slug: "unit_price", Label: "단가(원)", FieldType: schema.FieldNumber},
			{Slug: "location", Label: "보관 위치", FieldType: schema.FieldText},
			{Slug: "min_stock", Label: "안전 재고", FieldType: schema.FieldInteger},
			{Slug: "last_checked", Label: "최종 점검일", FieldType: schema.FieldDate},
		},
	}
}

// ── 요청 관리 ────────────────────────────────────────────────────

func requestsPreset() Preset {
	return Preset{
		Slug:        "requests",
		Label:       "요청 관리",
		Description: "부서 간 요청·처리 현황 추적",
		Icon:        "inbox",
		Fields: []schema.CreateFieldIn{
			{Slug: "title", Label: "제목", FieldType: schema.FieldText, IsRequired: true},
			{
				Slug: "category", Label: "분류", FieldType: schema.FieldSelect, IsIndexed: true,
				Options: jsonRaw(map[string]any{
					"choices": []string{"it", "hr", "facility", "purchase", "other"},
				}),
			},
			{
				Slug: "priority", Label: "우선순위", FieldType: schema.FieldSelect, IsIndexed: true,
				Options: jsonRaw(map[string]any{
					"choices": []string{"urgent", "high", "normal", "low"},
				}),
			},
			{
				Slug: "status", Label: "상태", FieldType: schema.FieldSelect, IsIndexed: true,
				Options: jsonRaw(map[string]any{
					"choices": []string{"open", "in_progress", "resolved", "closed"},
				}),
			},
			{Slug: "requester", Label: "요청자", FieldType: schema.FieldText},
			{Slug: "description", Label: "상세 내용", FieldType: schema.FieldTextarea},
			{Slug: "due_date", Label: "기한", FieldType: schema.FieldDate},
			{Slug: "completed_at", Label: "완료일", FieldType: schema.FieldDatetime},
		},
	}
}

// ── 회의록 ───────────────────────────────────────────────────────

func meetingsPreset() Preset {
	return Preset{
		Slug:        "meetings",
		Label:       "회의록",
		Description: "회의 일정·참석자·안건·결정 사항 기록",
		Icon:        "message-square",
		Fields: []schema.CreateFieldIn{
			{Slug: "title", Label: "회의명", FieldType: schema.FieldText, IsRequired: true},
			{Slug: "meeting_date", Label: "일시", FieldType: schema.FieldDatetime, IsIndexed: true},
			{Slug: "location", Label: "장소", FieldType: schema.FieldText},
			{Slug: "attendees", Label: "참석자", FieldType: schema.FieldTextarea},
			{Slug: "agenda", Label: "안건", FieldType: schema.FieldTextarea},
			{Slug: "decisions", Label: "결정 사항", FieldType: schema.FieldTextarea},
			{Slug: "action_items", Label: "후속 조치", FieldType: schema.FieldTextarea},
		},
	}
}

// ── 비용 정산 ────────────────────────────────────────────────────

func expensesPreset() Preset {
	return Preset{
		Slug:        "expenses",
		Label:       "비용 정산",
		Description: "경비·비용 청구 및 승인 관리",
		Icon:        "receipt",
		Fields: []schema.CreateFieldIn{
			{Slug: "title", Label: "항목", FieldType: schema.FieldText, IsRequired: true},
			{
				Slug: "category", Label: "분류", FieldType: schema.FieldSelect, IsIndexed: true,
				Options: jsonRaw(map[string]any{
					"choices": []string{"travel", "meal", "supplies", "equipment", "service", "other"},
				}),
			},
			{Slug: "amount", Label: "금액(원)", FieldType: schema.FieldNumber, IsRequired: true},
			{Slug: "spent_date", Label: "지출일", FieldType: schema.FieldDate, IsIndexed: true},
			{Slug: "receipt", Label: "영수증", FieldType: schema.FieldFile},
			{
				Slug: "status", Label: "상태", FieldType: schema.FieldSelect, IsIndexed: true,
				Options: jsonRaw(map[string]any{
					"choices": []string{"draft", "submitted", "approved", "rejected"},
				}),
			},
			{Slug: "note", Label: "비고", FieldType: schema.FieldTextarea},
		},
	}
}
