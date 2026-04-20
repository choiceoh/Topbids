// Package seed creates preset collections for Phaeton's domain (energy project management).
// Idempotent: checks for existence before creating.
package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/choiceoh/phaeton/backend/internal/migration"
	"github.com/choiceoh/phaeton/backend/internal/schema"
)

// Preset encodes a collection to create if it does not already exist.
// AccessConfig is optional; when nil, the collection has no role restrictions
// and no BidRole. Used by Topbids bid-domain presets to declare bid_role.
type Preset struct {
	Slug         string
	Label        string
	Description  string
	Icon         string
	IsSystem     bool
	Fields       []schema.CreateFieldIn
	AccessConfig *schema.AccessConfig
}

// Presets returns the built-in collection presets for Topbids.
// Bids collection is created separately in Run() because it needs relation
// target IDs from rfqs and suppliers (see applyBidRelations).
//
// Subsidiary/department data lives in auth.subsidiaries / auth.departments
// (see handler/subsidiary.go and handler/department.go) — not seeded as
// dynamic collections.
func Presets() []Preset {
	return BidPresets()
}

func orgSubsidiariesPreset() Preset {
	return Preset{
		Slug:        "_subsidiaries",
		Label:       "계열사",
		Description: "계열사(법인) 관리",
		Icon:        "building",
		IsSystem:    true,
		Fields: []schema.CreateFieldIn{
			{
				Slug:       "name",
				Label:      "계열사명",
				FieldType:  schema.FieldText,
				IsRequired: true,
				IsIndexed:  true,
			},
			{
				Slug:      "code",
				Label:     "코드",
				FieldType: schema.FieldText,
				IsUnique:  true,
			},
			{
				Slug:      "sort_order",
				Label:     "정렬 순서",
				FieldType: schema.FieldInteger,
			},
			{
				Slug:         "is_active",
				Label:        "활성",
				FieldType:    schema.FieldBoolean,
				DefaultValue: jsonRaw(true),
			},
		},
	}
}

func orgDepartmentsPreset() Preset {
	return Preset{
		Slug:        "_departments",
		Label:       "부서",
		Description: "부서 관리 (계층 구조)",
		Icon:        "building-2",
		IsSystem:    true,
		Fields: []schema.CreateFieldIn{
			{
				Slug:       "name",
				Label:      "부서명",
				FieldType:  schema.FieldText,
				IsRequired: true,
				IsIndexed:  true,
			},
			{
				Slug:      "code",
				Label:     "코드",
				FieldType: schema.FieldText,
				IsUnique:  true,
			},
			{
				Slug:      "sort_order",
				Label:     "정렬 순서",
				FieldType: schema.FieldInteger,
			},
			{
				Slug:         "is_active",
				Label:        "활성",
				FieldType:    schema.FieldBoolean,
				DefaultValue: jsonRaw(true),
			},
			// Relations (parent, subsidiary) added by applyOrgRefs() after creation.
		},
	}
}

func projectsPreset() Preset {
	return Preset{
		Slug:        "projects",
		Label:       "프로젝트",
		Description: "에너지 프로젝트 관리 — 태양광, 풍력, ESS, 하이브리드",
		Icon:        "chart",
		Fields: []schema.CreateFieldIn{
			{
				Slug:       "name",
				Label:      "프로젝트명",
				FieldType:  schema.FieldText,
				IsRequired: true,
				IsIndexed:  true,
			},
			{
				Slug:       "project_type",
				Label:      "유형",
				FieldType:  schema.FieldSelect,
				IsRequired: true,
				IsIndexed:  true,
				Options: jsonRaw(map[string]any{
					"choices": []string{"solar", "wind", "ess", "hybrid"},
				}),
			},
			{
				Slug:      "capacity_kw",
				Label:     "용량(kW)",
				FieldType: schema.FieldNumber,
			},
			{
				Slug:      "region",
				Label:     "지역",
				FieldType: schema.FieldText,
			},
			{
				Slug:      "status",
				Label:     "상태",
				FieldType: schema.FieldSelect,
				IsIndexed: true,
				Options: jsonRaw(map[string]any{
					"choices": []string{"planning", "permit", "construction", "testing", "cod"},
				}),
			},
			{
				Slug:      "cod_target",
				Label:     "COD 목표일",
				FieldType: schema.FieldDate,
			},
		},
	}
}

func milestonesPreset() Preset {
	return Preset{
		Slug:        "milestones",
		Label:       "마일스톤",
		Description: "프로젝트별 마일스톤 추적",
		Icon:        "check",
		Fields: []schema.CreateFieldIn{
			{
				Slug:       "name",
				Label:      "마일스톤명",
				FieldType:  schema.FieldText,
				IsRequired: true,
			},
			{
				Slug:      "seq_order",
				Label:     "순서",
				FieldType: schema.FieldInteger,
			},
			{
				Slug:      "status",
				Label:     "상태",
				FieldType: schema.FieldSelect,
				IsIndexed: true,
				Options: jsonRaw(map[string]any{
					"choices": []string{"pending", "active", "done", "blocked", "skipped"},
				}),
			},
			{
				Slug:      "due_date",
				Label:     "기한",
				FieldType: schema.FieldDate,
			},
			{
				Slug:      "completed_at",
				Label:     "완료일",
				FieldType: schema.FieldDatetime,
			},
			{
				Slug:      "is_critical",
				Label:     "중요",
				FieldType: schema.FieldBoolean,
			},
			// Relation filled in by applyProjectRef() after the projects collection is created.
		},
	}
}

func staffPreset() Preset {
	return Preset{
		Slug:        "staff",
		Label:       "인력 배치",
		Description: "프로젝트별 인력 투입 관리",
		Icon:        "tool",
		Fields: []schema.CreateFieldIn{
			{
				Slug:       "name",
				Label:      "이름",
				FieldType:  schema.FieldText,
				IsRequired: true,
			},
			{
				Slug:      "role",
				Label:     "역할",
				FieldType: schema.FieldText,
			},
			{
				Slug:      "start_date",
				Label:     "시작일",
				FieldType: schema.FieldDate,
			},
			{
				Slug:      "end_date",
				Label:     "종료일",
				FieldType: schema.FieldDate,
			},
			{
				Slug:      "allocation_pct",
				Label:     "배정률(%)",
				FieldType: schema.FieldNumber,
			},
			{
				Slug:      "is_active",
				Label:     "활성",
				FieldType: schema.FieldBoolean,
			},
		},
	}
}

func documentsPreset() Preset {
	return Preset{
		Slug:        "documents",
		Label:       "프로젝트 문서",
		Description: "인허가, 계약, 설계 등 프로젝트 문서 관리",
		Icon:        "file",
		Fields: []schema.CreateFieldIn{
			{
				Slug:      "doc_type",
				Label:     "유형",
				FieldType: schema.FieldSelect,
				IsIndexed: true,
				Options: jsonRaw(map[string]any{
					"choices": []string{"permit", "contract", "design", "report", "certificate", "other"},
				}),
			},
			{
				Slug:       "title",
				Label:      "제목",
				FieldType:  schema.FieldText,
				IsRequired: true,
			},
			{
				Slug:      "file",
				Label:     "파일",
				FieldType: schema.FieldFile,
			},
			{
				Slug:      "issued_at",
				Label:     "발급일",
				FieldType: schema.FieldDate,
			},
			{
				Slug:      "expires_at",
				Label:     "만료일",
				FieldType: schema.FieldDate,
			},
		},
	}
}

// Run creates presets through the migration engine. Skips any collection
// that already exists (matched by slug).
func Run(ctx context.Context, engine *migration.Engine, cache *schema.Cache) error {
	presets := Presets()

	// Track IDs of freshly-created collections so we can wire up relations.
	created := make(map[string]string)

	for _, p := range presets {
		if _, exists := cache.CollectionBySlug(p.Slug); exists {
			slog.Info("seed: collection exists, skipping", "slug", p.Slug)
			continue
		}

		req := &schema.CreateCollectionReq{
			Slug:         p.Slug,
			Label:        p.Label,
			Description:  p.Description,
			Icon:         p.Icon,
			IsSystem:     p.IsSystem,
			Fields:       p.Fields,
			AccessConfig: p.AccessConfig,
		}
		col, err := engine.CreateCollection(ctx, req)
		if err != nil {
			return fmt.Errorf("seed %s: %w", p.Slug, err)
		}
		created[p.Slug] = col.ID
		slog.Info("seed: created collection", "slug", p.Slug, "id", col.ID)
	}

	// After base collections exist, create bids with rfq/supplier relations.
	if err := applyBidRelations(ctx, engine, cache); err != nil {
		return fmt.Errorf("seed: apply bid relations: %w", err)
	}

	return nil
}

// applyOrgRefs adds subsidiary and self-referential parent relations to _departments.
func applyOrgRefs(ctx context.Context, engine *migration.Engine, cache *schema.Cache) error {
	depts, ok := cache.CollectionBySlug("_departments")
	if !ok {
		return nil
	}

	// Add subsidiary relation if not present.
	subs, subOK := cache.CollectionBySlug("_subsidiaries")
	if subOK {
		exists := false
		for _, f := range cache.Fields(depts.ID) {
			if f.Slug == "subsidiary" {
				exists = true
				break
			}
		}
		if !exists {
			req := &schema.CreateFieldIn{
				Slug:      "subsidiary",
				Label:     "소속 계열사",
				FieldType: schema.FieldRelation,
				IsIndexed: true,
				Relation: &schema.CreateRelIn{
					TargetCollectionID: subs.ID,
					RelationType:       schema.RelOneToMany,
					OnDelete:           "SET NULL",
				},
			}
			if _, _, err := engine.AddField(ctx, depts.ID, req, true); err != nil {
				return fmt.Errorf("add _departments.subsidiary: %w", err)
			}
			slog.Info("seed: added relation", "collection", "_departments", "field", "subsidiary")
		}
	}

	// Add self-referential parent relation if not present.
	exists := false
	for _, f := range cache.Fields(depts.ID) {
		if f.Slug == "parent" {
			exists = true
			break
		}
	}
	if !exists {
		req := &schema.CreateFieldIn{
			Slug:      "parent",
			Label:     "상위 부서",
			FieldType: schema.FieldRelation,
			IsIndexed: true,
			Relation: &schema.CreateRelIn{
				TargetCollectionID: depts.ID,
				RelationType:       schema.RelOneToMany,
				OnDelete:           "SET NULL",
			},
		}
		if _, _, err := engine.AddField(ctx, depts.ID, req, true); err != nil {
			return fmt.Errorf("add _departments.parent: %w", err)
		}
		slog.Info("seed: added relation", "collection", "_departments", "field", "parent")
	}

	return nil
}

// applyBidRelations creates the "bids" collection with relations to rfqs and
// suppliers, then sets each RFQ status's display ordering.
//
// bids is created here (not in Presets) because it needs rfqs/suppliers IDs
// for its relation fields — those IDs aren't known until those collections
// are seeded earlier in Run().
func applyBidRelations(ctx context.Context, engine *migration.Engine, cache *schema.Cache) error {
	rfqs, rfqOK := cache.CollectionBySlug("rfqs")
	suppliers, supOK := cache.CollectionBySlug("suppliers")
	if !rfqOK || !supOK {
		slog.Warn("seed: skipping bids, rfqs/suppliers not present")
		return nil
	}

	// Skip if bids already exists.
	if _, exists := cache.CollectionBySlug("bids"); exists {
		return nil
	}

	// Compose bids preset with relation fields prepended.
	preset := bidsPreset()
	relationFields := []schema.CreateFieldIn{
		{
			Slug:       "rfq",
			Label:      "입찰공고",
			FieldType:  schema.FieldRelation,
			IsRequired: true,
			IsIndexed:  true,
			Width:      3,
			Relation: &schema.CreateRelIn{
				TargetCollectionID: rfqs.ID,
				RelationType:       schema.RelOneToMany,
				OnDelete:           "CASCADE",
			},
		},
		{
			Slug:       "supplier",
			Label:      "공급사",
			FieldType:  schema.FieldRelation,
			IsRequired: true,
			IsIndexed:  true,
			Width:      3,
			Relation: &schema.CreateRelIn{
				TargetCollectionID: suppliers.ID,
				RelationType:       schema.RelOneToMany,
				OnDelete:           "CASCADE",
			},
		},
	}
	preset.Fields = append(relationFields, preset.Fields...)

	req := &schema.CreateCollectionReq{
		Slug:         preset.Slug,
		Label:        preset.Label,
		Description:  preset.Description,
		Icon:         preset.Icon,
		IsSystem:     preset.IsSystem,
		Fields:       preset.Fields,
		AccessConfig: preset.AccessConfig,
	}
	col, err := engine.CreateCollection(ctx, req)
	if err != nil {
		return fmt.Errorf("create bids: %w", err)
	}
	slog.Info("seed: created collection", "slug", "bids", "id", col.ID)
	return nil
}

func jsonRaw(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
