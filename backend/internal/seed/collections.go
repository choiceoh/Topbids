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

	// Upgrade access_config on pre-existing bid collections so deployments
	// from before the role-gating work (PR #20 era) pick up the new write
	// restrictions without requiring a DB wipe. Safe to run every boot:
	// compares current state to the preset and is a no-op when they agree.
	if err := syncBidAccessConfig(ctx, engine, cache); err != nil {
		return fmt.Errorf("seed: sync bid access_config: %w", err)
	}

	// Also backfill any new optional fields added to bid presets after the
	// initial install (attachments, estimated_price, min_win_rate, etc.).
	// AddField is idempotent by (collection, slug) so this is safe to run
	// on every boot.
	if err := syncBidFields(ctx, engine, cache); err != nil {
		return fmt.Errorf("seed: sync bid fields: %w", err)
	}

	return nil
}

// syncBidFields walks the bid preset fields and adds any that are missing
// from the current collection schema. Relation and required-without-default
// fields are skipped — the seed creates those at collection birth and adding
// them later would fail on existing rows.
//
// Only runs for collections with a BidRole set, so human-edited custom apps
// never get mutated by the seed.
func syncBidFields(ctx context.Context, engine *migration.Engine, cache *schema.Cache) error {
	presets := []func() Preset{
		rfqsPreset, bidsPreset, purchaseOrdersPreset, suppliersPreset,
		clarificationsPreset, supplierQualificationsPreset, bidEvaluationsPreset,
	}
	for _, pf := range presets {
		p := pf()
		col, exists := cache.CollectionBySlug(p.Slug)
		if !exists {
			continue
		}
		existing := make(map[string]bool, len(col.Fields))
		for _, f := range cache.Fields(col.ID) {
			existing[f.Slug] = true
		}
		for _, f := range p.Fields {
			if existing[f.Slug] {
				continue
			}
			// Can't retroactively add a NOT NULL column without a default on
			// a non-empty table. Existing bid rows would violate the constraint.
			// The preset authors are expected to mark new fields as optional.
			if f.IsRequired {
				slog.Warn("seed: skipping backfill of required field — add manually",
					"slug", p.Slug, "field", f.Slug)
				continue
			}
			req := f // copy
			if _, _, err := engine.AddField(ctx, col.ID, &req, true); err != nil {
				return fmt.Errorf("backfill %s.%s: %w", p.Slug, f.Slug, err)
			}
			slog.Info("seed: backfilled field", "collection", p.Slug, "field", f.Slug)
		}
	}
	return nil
}

// syncBidAccessConfig upgrades access_config on existing bid-domain
// collections to match the presets in bid_apps.go. Called on every seed
// run — idempotent because it only writes when the stored config differs
// from the preset's.
//
// Scope is narrowed to the bid presets (which have a BidRole set) so a
// human-edited access_config on unrelated user apps never gets clobbered.
func syncBidAccessConfig(ctx context.Context, engine *migration.Engine, cache *schema.Cache) error {
	presets := []func() Preset{
		suppliersPreset, rfqsPreset, bidsPreset, purchaseOrdersPreset,
		clarificationsPreset, supplierQualificationsPreset, bidEvaluationsPreset,
	}
	for _, pf := range presets {
		p := pf()
		if p.AccessConfig == nil {
			continue
		}
		col, exists := cache.CollectionBySlug(p.Slug)
		if !exists {
			continue // the create pass above will have made it fresh with the right config
		}
		if accessConfigMatches(col.AccessConfig, *p.AccessConfig) {
			continue
		}
		acCopy := *p.AccessConfig
		req := &schema.UpdateCollectionReq{AccessConfig: &acCopy}
		if _, err := engine.UpdateCollection(ctx, col.ID, req); err != nil {
			return fmt.Errorf("upgrade %s access_config: %w", p.Slug, err)
		}
		slog.Info("seed: upgraded access_config", "slug", p.Slug)
	}
	return nil
}

// accessConfigMatches reports whether two configs would produce identical
// runtime authorization decisions. Compares every field the bid presets
// populate. Nil and empty slices are treated as equivalent so JSON round-
// trips through pgx don't trigger spurious upgrades.
func accessConfigMatches(a, b schema.AccessConfig) bool {
	return a.BidRole == b.BidRole &&
		a.RLSMode == b.RLSMode &&
		stringSliceEqual(a.EntryView, b.EntryView) &&
		stringSliceEqual(a.EntryCreate, b.EntryCreate) &&
		stringSliceEqual(a.EntryEdit, b.EntryEdit) &&
		stringSliceEqual(a.EntryDelete, b.EntryDelete)
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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

// applyBidRelations creates the "bids" and "purchase_orders" collections
// with relations to their dependencies, after the base presets (rfqs,
// suppliers) are seeded.
//
// Both collections live here (not in Presets) because they need IDs from
// rfqs/suppliers/bids for relation fields — those IDs aren't known until
// earlier seed steps complete.
//
// Order: bids (depends on rfqs, suppliers) → purchase_orders (depends on
// rfqs, bids, suppliers).
func applyBidRelations(ctx context.Context, engine *migration.Engine, cache *schema.Cache) error {
	rfqs, rfqOK := cache.CollectionBySlug("rfqs")
	suppliers, supOK := cache.CollectionBySlug("suppliers")
	if !rfqOK || !supOK {
		slog.Warn("seed: skipping bids/purchase_orders, rfqs/suppliers not present")
		return nil
	}

	if err := createBidsCollection(ctx, engine, cache, rfqs.ID, suppliers.ID); err != nil {
		return err
	}

	bids, bidsOK := cache.CollectionBySlug("bids")
	if !bidsOK {
		return fmt.Errorf("bids collection unexpectedly missing after creation")
	}

	if err := createPurchaseOrdersCollection(ctx, engine, cache, rfqs.ID, bids.ID, suppliers.ID); err != nil {
		return err
	}

	if err := createClarificationsCollection(ctx, engine, cache, rfqs.ID); err != nil {
		return err
	}

	if err := createSupplierQualificationsCollection(ctx, engine, cache, suppliers.ID); err != nil {
		return err
	}

	return createBidEvaluationsCollection(ctx, engine, cache, bids.ID)
}

// createBidEvaluationsCollection seeds the multi-evaluator scoring collection
// with a relation back to bids. Populated by buyer staff after open_at; the
// award pickWeighted step averages tech_score across rows.
func createBidEvaluationsCollection(ctx context.Context, engine *migration.Engine, cache *schema.Cache, bidsID string) error {
	if _, exists := cache.CollectionBySlug("bid_evaluations"); exists {
		return nil
	}
	preset := bidEvaluationsPreset()
	preset.Fields = append([]schema.CreateFieldIn{
		{
			Slug: "bid", Label: "입찰서", FieldType: schema.FieldRelation,
			IsRequired: true, IsIndexed: true, Width: 3,
			Relation: &schema.CreateRelIn{
				TargetCollectionID: bidsID, RelationType: schema.RelOneToMany, OnDelete: "CASCADE",
			},
		},
		{
			Slug: "evaluator", Label: "평가자", FieldType: schema.FieldUser,
			IsRequired: true, IsIndexed: true, Width: 3,
		},
	}, preset.Fields...)

	col, err := engine.CreateCollection(ctx, presetToReq(preset))
	if err != nil {
		return fmt.Errorf("create bid_evaluations: %w", err)
	}
	slog.Info("seed: created collection", "slug", "bid_evaluations", "id", col.ID)
	return nil
}

// createClarificationsCollection seeds the RFQ Q&A thread collection with a
// relation back to rfqs. Separate from the base preset list because it
// needs the rfqs.id for the relation field.
func createClarificationsCollection(ctx context.Context, engine *migration.Engine, cache *schema.Cache, rfqsID string) error {
	if _, exists := cache.CollectionBySlug("rfq_clarifications"); exists {
		return nil
	}
	preset := clarificationsPreset()
	preset.Fields = append([]schema.CreateFieldIn{
		{
			Slug: "rfq", Label: "입찰공고", FieldType: schema.FieldRelation,
			IsRequired: true, IsIndexed: true, Width: 3,
			Relation: &schema.CreateRelIn{
				TargetCollectionID: rfqsID, RelationType: schema.RelOneToMany, OnDelete: "CASCADE",
			},
		},
	}, preset.Fields...)

	col, err := engine.CreateCollection(ctx, presetToReq(preset))
	if err != nil {
		return fmt.Errorf("create rfq_clarifications: %w", err)
	}
	slog.Info("seed: created collection", "slug", "rfq_clarifications", "id", col.ID)
	return nil
}

// createSupplierQualificationsCollection seeds the PQ collection with a
// relation back to suppliers. One row per (supplier, category).
func createSupplierQualificationsCollection(ctx context.Context, engine *migration.Engine, cache *schema.Cache, suppliersID string) error {
	if _, exists := cache.CollectionBySlug("supplier_qualifications"); exists {
		return nil
	}
	preset := supplierQualificationsPreset()
	preset.Fields = append([]schema.CreateFieldIn{
		{
			Slug: "supplier", Label: "공급사", FieldType: schema.FieldRelation,
			IsRequired: true, IsIndexed: true, Width: 3,
			Relation: &schema.CreateRelIn{
				TargetCollectionID: suppliersID, RelationType: schema.RelOneToMany, OnDelete: "CASCADE",
			},
		},
	}, preset.Fields...)

	col, err := engine.CreateCollection(ctx, presetToReq(preset))
	if err != nil {
		return fmt.Errorf("create supplier_qualifications: %w", err)
	}
	slog.Info("seed: created collection", "slug", "supplier_qualifications", "id", col.ID)
	return nil
}

func createBidsCollection(ctx context.Context, engine *migration.Engine, cache *schema.Cache, rfqsID, suppliersID string) error {
	if _, exists := cache.CollectionBySlug("bids"); exists {
		return nil
	}

	preset := bidsPreset()
	preset.Fields = append([]schema.CreateFieldIn{
		{
			Slug: "rfq", Label: "입찰공고", FieldType: schema.FieldRelation,
			IsRequired: true, IsIndexed: true, Width: 3,
			Relation: &schema.CreateRelIn{
				TargetCollectionID: rfqsID, RelationType: schema.RelOneToMany, OnDelete: "CASCADE",
			},
		},
		{
			Slug: "supplier", Label: "공급사", FieldType: schema.FieldRelation,
			IsRequired: true, IsIndexed: true, Width: 3,
			Relation: &schema.CreateRelIn{
				TargetCollectionID: suppliersID, RelationType: schema.RelOneToMany, OnDelete: "CASCADE",
			},
		},
	}, preset.Fields...)

	col, err := engine.CreateCollection(ctx, presetToReq(preset))
	if err != nil {
		return fmt.Errorf("create bids: %w", err)
	}
	slog.Info("seed: created collection", "slug", "bids", "id", col.ID)
	return nil
}

func createPurchaseOrdersCollection(ctx context.Context, engine *migration.Engine, cache *schema.Cache, rfqsID, bidsID, suppliersID string) error {
	if _, exists := cache.CollectionBySlug("purchase_orders"); exists {
		return nil
	}

	preset := purchaseOrdersPreset()
	preset.Fields = append([]schema.CreateFieldIn{
		{
			Slug: "rfq", Label: "입찰공고", FieldType: schema.FieldRelation,
			IsRequired: true, IsIndexed: true, Width: 3,
			Relation: &schema.CreateRelIn{
				TargetCollectionID: rfqsID, RelationType: schema.RelOneToMany, OnDelete: "RESTRICT",
			},
		},
		{
			Slug: "bid", Label: "낙찰 입찰서", FieldType: schema.FieldRelation,
			IsRequired: true, IsIndexed: true, Width: 3,
			Relation: &schema.CreateRelIn{
				TargetCollectionID: bidsID, RelationType: schema.RelOneToMany, OnDelete: "RESTRICT",
			},
		},
		{
			Slug: "supplier", Label: "공급사", FieldType: schema.FieldRelation,
			IsRequired: true, IsIndexed: true, Width: 3,
			Relation: &schema.CreateRelIn{
				TargetCollectionID: suppliersID, RelationType: schema.RelOneToMany, OnDelete: "RESTRICT",
			},
		},
	}, preset.Fields...)

	col, err := engine.CreateCollection(ctx, presetToReq(preset))
	if err != nil {
		return fmt.Errorf("create purchase_orders: %w", err)
	}
	slog.Info("seed: created collection", "slug", "purchase_orders", "id", col.ID)
	return nil
}

func presetToReq(p Preset) *schema.CreateCollectionReq {
	return &schema.CreateCollectionReq{
		Slug:         p.Slug,
		Label:        p.Label,
		Description:  p.Description,
		Icon:         p.Icon,
		IsSystem:     p.IsSystem,
		Fields:       p.Fields,
		AccessConfig: p.AccessConfig,
	}
}

func jsonRaw(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
