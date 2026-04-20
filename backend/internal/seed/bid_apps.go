package seed

import (
	"github.com/choiceoh/phaeton/backend/internal/schema"
)

// BidPresets returns the fixed Topbids domain collections (입찰 공고 / 입찰서 / 공급사).
// These collections are seeded on bootstrap and not user-editable via an app builder —
// they carry the bidding domain's invariants (sealed fields, bid roles).
//
// Order matters: suppliers before bids (bids references suppliers), rfqs before bids.
func BidPresets() []Preset {
	return []Preset{
		suppliersPreset(),
		rfqsPreset(),
		// bidsPreset() is added after rfqs+suppliers exist (needs their IDs for relations).
		// See applyBidRelations in collections.go.
	}
}

// suppliersPreset — 공급사 마스터.
func suppliersPreset() Preset {
	return Preset{
		Slug:        "suppliers",
		Label:       "공급사",
		Description: "입찰 참여 공급사 마스터",
		Icon:        "briefcase",
		AccessConfig: &schema.AccessConfig{
			BidRole: schema.BidRoleSupplier,
		},
		Fields: []schema.CreateFieldIn{
			{Slug: "name", Label: "공급사명", FieldType: schema.FieldText, IsRequired: true, IsIndexed: true, Width: 3},
			{Slug: "biz_no", Label: "사업자등록번호", FieldType: schema.FieldText, IsRequired: true, IsUnique: true, Width: 3},
			{Slug: "ceo", Label: "대표자", FieldType: schema.FieldText, Width: 3},
			{Slug: "email", Label: "이메일", FieldType: schema.FieldText, Width: 3},
			{Slug: "phone", Label: "연락처", FieldType: schema.FieldText, Width: 3},
			{Slug: "address", Label: "주소", FieldType: schema.FieldText, Width: 6},
			{
				Slug:      "categories",
				Label:     "취급 카테고리",
				FieldType: schema.FieldMultiselect,
				Width:     6,
				Options: jsonRaw(map[string]any{
					"choices": []string{"자재", "장비", "시공", "용역"},
				}),
			},
			{
				Slug:         "status",
				Label:        "상태",
				FieldType:    schema.FieldSelect,
				DefaultValue: jsonRaw("active"),
				Width:        3,
				Options: jsonRaw(map[string]any{
					"choices": []string{"active", "suspended", "blacklisted"},
				}),
			},
			{Slug: "note", Label: "비고", FieldType: schema.FieldTextarea, Width: 6},
		},
	}
}

// rfqsPreset — 입찰 공고.
func rfqsPreset() Preset {
	return Preset{
		Slug:        "rfqs",
		Label:       "입찰 공고",
		Description: "그룹 통합 구매 입찰 공고",
		Icon:        "file-text",
		AccessConfig: &schema.AccessConfig{
			BidRole: schema.BidRoleRfq,
		},
		Fields: []schema.CreateFieldIn{
			{Slug: "rfq_no", Label: "공고번호", FieldType: schema.FieldText, IsRequired: true, IsUnique: true, IsIndexed: true, Width: 3},
			{Slug: "title", Label: "제목", FieldType: schema.FieldText, IsRequired: true, IsIndexed: true, Width: 3},
			{Slug: "description", Label: "공고 내용", FieldType: schema.FieldTextarea, Width: 6},
			{
				Slug:      "category",
				Label:     "카테고리",
				FieldType: schema.FieldSelect,
				Width:     3,
				Options: jsonRaw(map[string]any{
					"choices": []string{"자재", "장비", "시공", "용역"},
				}),
			},
			{
				Slug:         "mode",
				Label:        "입찰방식",
				FieldType:    schema.FieldSelect,
				IsRequired:   true,
				DefaultValue: jsonRaw("open"),
				Width:        3,
				Options: jsonRaw(map[string]any{
					// Internal codes; UI maps to Korean labels.
					"choices": []string{"open", "invited", "private"},
				}),
			},
			{
				Slug:         "eval_method",
				Label:        "평가방식",
				FieldType:    schema.FieldSelect,
				IsRequired:   true,
				DefaultValue: jsonRaw("lowest"),
				Width:        3,
				Options: jsonRaw(map[string]any{
					"choices": []string{"lowest", "weighted"},
				}),
			},
			{
				Slug:         "sealed",
				Label:        "밀봉입찰",
				FieldType:    schema.FieldBoolean,
				DefaultValue: jsonRaw(true),
				Width:        3,
			},
			{Slug: "published_at", Label: "공고일시", FieldType: schema.FieldDatetime, Width: 3},
			{Slug: "deadline_at", Label: "입찰마감", FieldType: schema.FieldDatetime, IsRequired: true, Width: 3},
			{Slug: "open_at", Label: "개찰일시", FieldType: schema.FieldDatetime, IsRequired: true, Width: 3},
			{
				Slug:         "status",
				Label:        "상태",
				FieldType:    schema.FieldSelect,
				DefaultValue: jsonRaw("draft"),
				Width:        3,
				Options: jsonRaw(map[string]any{
					// Internal codes; scheduler (bid.Scheduler) transitions
					// draft → published → closed → opened. evaluating/awarded
					// /failed/cancelled are set by user actions.
					"choices": []string{"draft", "published", "closed", "opened", "evaluating", "awarded", "failed", "cancelled"},
				}),
			},
		},
	}
}

// purchaseOrdersPreset — 발주서. rfq/bid/supplier relation fields are added
// by applyBidRelations after dependent collections exist.
//
// subsidiary/subsidiary_name are plain text columns (not relations) because
// subsidiaries live in auth.subsidiaries (a system table), outside the
// dynamic schema engine. distributePO writes the subsidiary's UUID and a
// denormalized name for UI display.
func purchaseOrdersPreset() Preset {
	return Preset{
		Slug:        "purchase_orders",
		Label:       "발주서",
		Description: "낙찰 후 계열사별 자동 분배된 발주 내역",
		Icon:        "package",
		AccessConfig: &schema.AccessConfig{
			BidRole: schema.BidRolePO,
		},
		Fields: []schema.CreateFieldIn{
			{Slug: "po_no", Label: "발주번호", FieldType: schema.FieldText, IsRequired: true, IsUnique: true, IsIndexed: true, Width: 3},
			{Slug: "subsidiary", Label: "계열사 ID", FieldType: schema.FieldText, IsIndexed: true, Width: 3},
			{Slug: "subsidiary_name", Label: "계열사명", FieldType: schema.FieldText, Width: 3},
			{Slug: "allocated_amount", Label: "배정금액", FieldType: schema.FieldNumber, Width: 3},
			{Slug: "allocation_ratio", Label: "배정비율", FieldType: schema.FieldNumber, Width: 3,
				Options: jsonRaw(map[string]any{"min": 0, "max": 1}),
			},
			{
				Slug:         "status",
				Label:        "상태",
				FieldType:    schema.FieldSelect,
				DefaultValue: jsonRaw("draft"),
				Width:        3,
				Options: jsonRaw(map[string]any{
					"choices": []string{"draft", "confirmed", "shipped", "received", "completed", "cancelled"},
				}),
			},
			{Slug: "po_date", Label: "발주일", FieldType: schema.FieldDatetime, Width: 3},
			{Slug: "note", Label: "비고", FieldType: schema.FieldTextarea, Width: 6},
		},
	}
}

// bidsPreset — 입찰서. Relation fields to rfqs/suppliers are added by
// applyBidRelations after both collections exist.
//
// Fields carrying sealed options: total_amount, lead_time. sealed_until_at
// references the parent RFQ row's open_at (resolved at read time by
// SealedReadFilter via the "rfq" relation).
func bidsPreset() Preset {
	return Preset{
		Slug:        "bids",
		Label:       "입찰서",
		Description: "공급사별 입찰 제출 내역",
		Icon:        "gavel",
		AccessConfig: &schema.AccessConfig{
			BidRole: schema.BidRoleBid,
		},
		Fields: []schema.CreateFieldIn{
			{
				Slug:       "total_amount",
				Label:      "입찰금액",
				FieldType:  schema.FieldNumber,
				IsRequired: true,
				Width:      3,
				Options: jsonRaw(map[string]any{
					"sealed_until_at":  "field:rfq.open_at",
					"unlock_by_status": []string{"opened", "evaluating", "awarded"},
				}),
			},
			{
				Slug:      "lead_time",
				Label:     "납기(일)",
				FieldType: schema.FieldInteger,
				Width:     3,
				Options: jsonRaw(map[string]any{
					"sealed_until_at":  "field:rfq.open_at",
					"unlock_by_status": []string{"opened", "evaluating", "awarded"},
				}),
			},
			{
				Slug:         "valid_days",
				Label:        "견적 유효기간(일)",
				FieldType:    schema.FieldInteger,
				DefaultValue: jsonRaw(30),
				Width:        3,
			},
			{Slug: "tech_score", Label: "기술점수", FieldType: schema.FieldNumber, Width: 3},
			{Slug: "price_score", Label: "가격점수", FieldType: schema.FieldNumber, Width: 3},
			{Slug: "total_score", Label: "종합점수", FieldType: schema.FieldNumber, Width: 3},
			{
				Slug:         "status",
				Label:        "상태",
				FieldType:    schema.FieldSelect,
				DefaultValue: jsonRaw("draft"),
				Width:        3,
				Options: jsonRaw(map[string]any{
					"choices": []string{"draft", "submitted", "opened", "evaluated", "awarded", "rejected"},
				}),
			},
			{Slug: "submitted_at", Label: "제출일시", FieldType: schema.FieldDatetime, Width: 3},
			{Slug: "note", Label: "비고", FieldType: schema.FieldTextarea, Width: 6},
		},
	}
}
