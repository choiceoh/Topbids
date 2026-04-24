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
//
// Access: suppliers are master data managed by internal buyers. Everyone
// authenticated can read (so suppliers can view their own profile and
// internal users can resolve relation labels), but only director/pm can
// mutate. Leaving a write list empty would fall through to "open to all"
// in AllowsRole — we enumerate explicitly so supplier-role can't impersonate
// other companies by editing suppliers rows.
func suppliersPreset() Preset {
	return Preset{
		Slug:        "suppliers",
		Label:       "공급사",
		Description: "입찰 참여 공급사 마스터",
		Icon:        "briefcase",
		AccessConfig: &schema.AccessConfig{
			BidRole:     schema.BidRoleSupplier,
			EntryView:   []string{"director", "pm", "engineer", "viewer", "supplier"},
			EntryCreate: []string{"director", "pm"},
			EntryEdit:   []string{"director", "pm"},
			EntryDelete: []string{"director", "pm"},
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
//
// Access: suppliers must read RFQs to decide whether to bid, but must not
// create/edit/delete them (that would let a bidder forge public notices).
// Internal buyers (pm/engineer) author and edit; only director/pm can delete
// (to preserve history).
func rfqsPreset() Preset {
	return Preset{
		Slug:        "rfqs",
		Label:       "입찰 공고",
		Description: "그룹 통합 구매 입찰 공고",
		Icon:        "file-text",
		AccessConfig: &schema.AccessConfig{
			BidRole:     schema.BidRoleRfq,
			EntryView:   []string{"director", "pm", "engineer", "viewer", "supplier"},
			EntryCreate: []string{"director", "pm", "engineer"},
			EntryEdit:   []string{"director", "pm", "engineer"},
			EntryDelete: []string{"director", "pm"},
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
					// "auction" = live reverse auction — sealing disabled,
					// current best price is visible to all bidders in real time.
					"choices": []string{"open", "invited", "private", "auction"},
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
			// Three-tier price fields following Korean procurement convention:
			//   base_amount: 발주자 내부 기초금액 (비공개, 예가 산출용)
			//   estimated_price: 공급사에게 공개되는 추정가
			//   planned_price: 개찰 시 결정되는 예정가 (복수예가 방식에서 계산)
			// Award floor uses estimated_price * min_win_rate.
			{Slug: "base_amount", Label: "기초금액(내부)", FieldType: schema.FieldNumber, Width: 3},
			{Slug: "estimated_price", Label: "추정가(공개)", FieldType: schema.FieldNumber, Width: 3},
			{Slug: "planned_price", Label: "예정가(산출)", FieldType: schema.FieldNumber, Width: 3},
			{Slug: "min_win_rate", Label: "낙찰하한율", FieldType: schema.FieldNumber, Width: 3,
				Options: jsonRaw(map[string]any{"min": 0, "max": 1})},
			// Multiple reserve prices (복수예가): pre-generated 15 candidates.
			// When reserve_method='multiple', bidders pick 2 indices; the
			// 4 most-picked become the basis for 예정가 at open time.
			{Slug: "reserve_method", Label: "예가방식", FieldType: schema.FieldSelect,
				DefaultValue: jsonRaw("single"), Width: 3,
				Options: jsonRaw(map[string]any{
					"choices": []string{"single", "multiple", "none"},
				})},
			{Slug: "reserve_prices", Label: "예비가격 15개", FieldType: schema.FieldJSON, Width: 6},
			// Classification — RFQ is price-led, RFP is proposal-led, RFI is
			// info-only. We tag the record for filtering/workflows; scoring
			// logic today only applies to rfq.
			{Slug: "rfx_type", Label: "요청 유형", FieldType: schema.FieldSelect,
				DefaultValue: jsonRaw("rfq"), Width: 3,
				Options: jsonRaw(map[string]any{
					"choices": []string{"rfq", "rfp", "rfi"},
				})},
			// Template marker — duplicatable shells excluded from discovery.
			{Slug: "is_template", Label: "템플릿 여부", FieldType: schema.FieldBoolean,
				DefaultValue: jsonRaw(false), Width: 3},
			// Amendment tracking. amendment_count bumps on each AmendRFQ;
			// last_amended_at shows suppliers what's fresh.
			{Slug: "amendment_count", Label: "변경 횟수", FieldType: schema.FieldInteger,
				DefaultValue: jsonRaw(0), Width: 3},
			{Slug: "last_amended_at", Label: "최종 변경", FieldType: schema.FieldDatetime, Width: 3},
			{Slug: "amendment_note", Label: "변경 내용", FieldType: schema.FieldTextarea, Width: 6},
			// RFQ attachments (specs, drawings, terms). Stored as an array of
			// upload file references — buyers attach during publish, suppliers
			// download from the portal detail page.
			{Slug: "attachments", Label: "첨부 파일", FieldType: schema.FieldFile, Width: 6},
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
			// POs are authored by DistributePO (server-side SQL); no API
			// create/delete. Only director/pm can edit (e.g. adjust status or
			// note after shipping). Suppliers need read to track their own POs
			// — SupplierRowFilter keeps them to their own rows.
			EntryView:   []string{"director", "pm", "engineer", "viewer", "supplier"},
			EntryCreate: []string{"director", "pm"},
			EntryEdit:   []string{"director", "pm"},
			EntryDelete: []string{"director", "pm"},
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

// clarificationsPreset — 입찰 공고 Q&A. Suppliers post questions during the
// bidding period; buyers answer and the answer is visible to all bidders.
//
// Access: everyone authenticated reads (transparency). Suppliers create
// their own questions; director/pm edit to add answers. Relation to rfqs
// is added by applyBidRelations once rfqs exists.
func clarificationsPreset() Preset {
	return Preset{
		Slug:        "rfq_clarifications",
		Label:       "입찰 Q&A",
		Description: "공고별 질의응답 — 모든 참여 공급사가 공유",
		Icon:        "message-circle",
		AccessConfig: &schema.AccessConfig{
			BidRole:     schema.BidRoleRfqClarify,
			EntryView:   []string{"director", "pm", "engineer", "viewer", "supplier"},
			EntryCreate: []string{"director", "pm", "engineer", "supplier"},
			EntryEdit:   []string{"director", "pm", "engineer"},
			EntryDelete: []string{"director", "pm"},
		},
		Fields: []schema.CreateFieldIn{
			{Slug: "question", Label: "질문", FieldType: schema.FieldTextarea, IsRequired: true, Width: 6},
			{Slug: "asked_at", Label: "질문일시", FieldType: schema.FieldDatetime, Width: 3},
			{Slug: "answer", Label: "답변", FieldType: schema.FieldTextarea, Width: 6},
			{Slug: "answered_at", Label: "답변일시", FieldType: schema.FieldDatetime, Width: 3},
			{
				Slug: "status", Label: "상태", FieldType: schema.FieldSelect,
				DefaultValue: jsonRaw("pending"), Width: 3,
				Options: jsonRaw(map[string]any{
					"choices": []string{"pending", "answered"},
				}),
			},
		},
	}
}

// supplierQualificationsPreset — PQ 심사 레코드.
//
// One row per (supplier, category) combination. Buyers review financial /
// technical / compliance dimensions and mark approved with an expiry date;
// the write guard later blocks bid submission when no valid qualification
// exists for the RFQ's category.
//
// Access: suppliers see only their own qualifications (SupplierRowFilter
// via BidRolePQ). Buyers read all, create/edit. Director deletes.
func supplierQualificationsPreset() Preset {
	return Preset{
		Slug:        "supplier_qualifications",
		Label:       "PQ 심사",
		Description: "공급사 사전자격심사 — 카테고리별 유효기간 관리",
		Icon:        "shield-check",
		AccessConfig: &schema.AccessConfig{
			BidRole:     schema.BidRolePQ,
			EntryView:   []string{"director", "pm", "engineer", "viewer", "supplier"},
			EntryCreate: []string{"director", "pm"},
			EntryEdit:   []string{"director", "pm"},
			EntryDelete: []string{"director"},
		},
		Fields: []schema.CreateFieldIn{
			{
				Slug: "category", Label: "카테고리", FieldType: schema.FieldSelect,
				IsRequired: true, Width: 3,
				Options: jsonRaw(map[string]any{
					"choices": []string{"자재", "장비", "시공", "용역"},
				}),
			},
			{
				Slug: "status", Label: "심사상태", FieldType: schema.FieldSelect,
				IsRequired: true, DefaultValue: jsonRaw("pending"), Width: 3,
				Options: jsonRaw(map[string]any{
					"choices": []string{"pending", "approved", "rejected", "expired"},
				}),
			},
			{Slug: "financial_score", Label: "재무점수", FieldType: schema.FieldNumber, Width: 3,
				Options: jsonRaw(map[string]any{"min": 0, "max": 100})},
			{Slug: "technical_score", Label: "기술점수", FieldType: schema.FieldNumber, Width: 3,
				Options: jsonRaw(map[string]any{"min": 0, "max": 100})},
			{Slug: "compliance_score", Label: "준법점수", FieldType: schema.FieldNumber, Width: 3,
				Options: jsonRaw(map[string]any{"min": 0, "max": 100})},
			{Slug: "reviewed_at", Label: "심사일", FieldType: schema.FieldDate, Width: 3},
			{Slug: "valid_until", Label: "유효기간", FieldType: schema.FieldDate, Width: 3},
			{Slug: "note", Label: "심사 메모", FieldType: schema.FieldTextarea, Width: 6},
		},
	}
}

// bidEvaluationsPreset — 다중 평가자 블라인드 평가.
//
// One row per (bid, evaluator) pair. Buyers score each bid independently;
// pickWeighted aggregates the evaluator scores into bid.tech_score at
// award time. Keeps individual scores + comments for audit.
//
// Access: evaluators (director/pm/engineer) create/edit their own rows.
// Suppliers never see evaluations — EntryView excludes supplier role.
// Relation to bids added by applyBidRelations.
func bidEvaluationsPreset() Preset {
	return Preset{
		Slug:        "bid_evaluations",
		Label:       "입찰 평가",
		Description: "다중 평가자 블라인드 기술점수",
		Icon:        "gauge",
		AccessConfig: &schema.AccessConfig{
			BidRole:     schema.BidRoleEvaluation,
			EntryView:   []string{"director", "pm", "engineer", "viewer"},
			EntryCreate: []string{"director", "pm", "engineer"},
			EntryEdit:   []string{"director", "pm", "engineer"},
			EntryDelete: []string{"director", "pm"},
		},
		Fields: []schema.CreateFieldIn{
			{Slug: "tech_score", Label: "기술점수", FieldType: schema.FieldNumber,
				IsRequired: true, Width: 3,
				Options: jsonRaw(map[string]any{"min": 0, "max": 100})},
			{Slug: "comment", Label: "평가 의견", FieldType: schema.FieldTextarea, Width: 6},
			{Slug: "scored_at", Label: "평가일시", FieldType: schema.FieldDatetime, Width: 3},
		},
	}
}

// bidsPreset — 입찰서. Relation fields to rfqs/suppliers are added by
// applyBidRelations after both collections exist.
//
// Fields carrying sealed options: total_amount, lead_time. sealed_until_at
// references the parent RFQ row's open_at (resolved at read time by
// SealedReadFilter via the "rfq" relation).
//
// Access: suppliers submit and edit their own bids (row-ownership enforced
// by bid.EnforceBidWriteOwnership — AccessConfig only gates role-level).
// Internal buyers read all for evaluation. Deletion is restricted to
// director/pm for audit integrity; suppliers cannot retract once submitted.
func bidsPreset() Preset {
	return Preset{
		Slug:        "bids",
		Label:       "입찰서",
		Description: "공급사별 입찰 제출 내역",
		Icon:        "gavel",
		AccessConfig: &schema.AccessConfig{
			BidRole:     schema.BidRoleBid,
			EntryView:   []string{"director", "pm", "engineer", "viewer", "supplier"},
			EntryCreate: []string{"director", "pm", "engineer", "supplier"},
			EntryEdit:   []string{"director", "pm", "engineer", "supplier"},
			EntryDelete: []string{"director", "pm"},
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
			// Aggregate tech_score — populated either manually (legacy) or as
			// the average of bid_evaluations rows on award time. Kept on the
			// bid row so read paths don't need to join.
			{Slug: "tech_score", Label: "기술점수(평균)", FieldType: schema.FieldNumber, Width: 3},
			// Bidder's picks into rfqs.reserve_prices (복수예가 투찰).
			// 2 indices in [0,14]. Used when reserve_method='multiple'.
			{Slug: "reserve_picks", Label: "예가 선택", FieldType: schema.FieldJSON, Width: 3},
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
