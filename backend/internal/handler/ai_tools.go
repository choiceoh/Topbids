package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/choiceoh/topbid/backend/internal/ai"
)

// workspaceTools returns the tool definitions available for AI chat and automation.
var workspaceTools = []ai.Tool{
	{
		Type: "function",
		Function: ai.ToolFunction{
			Name:        "list_collections",
			Description: "워크스페이스의 모든 앱 목록을 이름과 slug로 조회합니다",
			Parameters:  json.RawMessage(`{"type":"object","properties":{},"required":[]}`),
		},
	},
	{
		Type: "function",
		Function: ai.ToolFunction{
			Name:        "get_collection_fields",
			Description: "특정 앱의 항목 상세 정보를 조회합니다. slug로 지정합니다.",
			Parameters: json.RawMessage(`{
				"type":"object",
				"properties":{
					"slug":{"type":"string","description":"앱의 영문 ID (slug)"}
				},
				"required":["slug"]
			}`),
		},
	},
	{
		Type: "function",
		Function: ai.ToolFunction{
			Name:        "list_users",
			Description: "워크스페이스의 사용자 목록을 이름, ID로 조회합니다. 담당자/요청자 등 사용자 필드 값을 결정할 때 사용합니다.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{},"required":[]}`),
		},
	},
}

// newToolResolver creates a ToolResolver that can look up workspace data.
func (h *AIHandler) newToolResolver(ctx context.Context) ai.ToolResolver {
	return func(name, arguments string) (string, error) {
		switch name {
		case "list_collections":
			return h.resolveListCollections(ctx)
		case "get_collection_fields":
			var args struct {
				Slug string `json:"slug"`
			}
			if err := json.Unmarshal([]byte(arguments), &args); err != nil {
				return "", fmt.Errorf("parse arguments: %w", err)
			}
			return h.resolveGetCollectionFields(ctx, args.Slug)
		case "list_users":
			return h.resolveListUsers(ctx)
		default:
			return "", fmt.Errorf("unknown tool: %s", name)
		}
	}
}

func (h *AIHandler) resolveListCollections(ctx context.Context) (string, error) {
	collections, err := h.store.ListCollections(ctx)
	if err != nil {
		return "", fmt.Errorf("list collections: %w", err)
	}
	if len(collections) == 0 {
		return "워크스페이스에 앱이 없습니다.", nil
	}

	var sb strings.Builder
	for _, c := range collections {
		desc := ""
		if c.Description != "" {
			desc = " — " + c.Description
		}
		fmt.Fprintf(&sb, "- %s (slug: %s)%s\n", c.Label, c.Slug, desc)
	}
	return sb.String(), nil
}

func (h *AIHandler) resolveListUsers(ctx context.Context) (string, error) {
	rows, err := h.pool.Query(ctx,
		`SELECT id, name, email FROM users WHERE is_active = true ORDER BY name`)
	if err != nil {
		return "", fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var sb strings.Builder
	for rows.Next() {
		var id, name, email string
		if err := rows.Scan(&id, &name, &email); err != nil {
			continue
		}
		fmt.Fprintf(&sb, "- %s (id: %s, email: %s)\n", name, id, email)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("list users: rows iteration: %w", err)
	}
	if sb.Len() == 0 {
		return "사용자가 없습니다.", nil
	}
	return sb.String(), nil
}

func (h *AIHandler) resolveGetCollectionFields(ctx context.Context, slug string) (string, error) {
	col, err := h.store.GetCollectionBySlug(ctx, slug)
	if err != nil {
		return "", fmt.Errorf("get collection: %w", err)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "## %s (%s)\n", col.Label, col.Slug)
	if col.Description != "" {
		fmt.Fprintf(&sb, "설명: %s\n", col.Description)
	}
	if col.ProcessEnabled {
		sb.WriteString("프로세스(워크플로): 활성화됨\n")
	}
	sb.WriteString("\n항목:\n")
	for _, f := range col.Fields {
		if f.IsLayout {
			continue
		}
		opts := ""
		if len(f.Options) > 0 && string(f.Options) != "null" {
			opts = fmt.Sprintf(" options=%s", string(f.Options))
		}
		req := ""
		if f.IsRequired {
			req = " [필수]"
		}
		fmt.Fprintf(&sb, "- %s (slug: %q, type: %s)%s%s\n", f.Label, f.Slug, f.FieldType, req, opts)
	}
	return sb.String(), nil
}
