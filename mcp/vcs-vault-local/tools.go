package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// VaultReader reads files from a local vcs-vcs-vault-local mount.
// In prod, swap for a Gitea API client without changing the Handler.
type VaultReader struct {
	Root string // e.g. /vcs-vault-local (mounted from vcs-vcs-vault-local repo)
}

func (h *Handler) callTool(ctx context.Context, p toolCallParams) (toolCallResult, error) {
	switch p.Name {
	case "read_skill":
		return h.readSkill(p.Arguments)
	case "read_openspec":
		return h.readOpenSpec(p.Arguments)
	case "search_knowledge":
		return h.searchKnowledge(p.Arguments)
	default:
		return toolCallResult{}, fmt.Errorf("unknown tool: %s", p.Name)
	}
}

// read_skill — reads skills/{skill_id}.md from vault root
func (h *Handler) readSkill(args map[string]any) (toolCallResult, error) {
	id, ok := stringArg(args, "skill_id")
	if !ok {
		return toolCallResult{}, fmt.Errorf("skill_id is required")
	}
	// sanitize: strip path separators so agents can't escape the vcs-vault-local
	id = sanitizePath(id)
	path := filepath.Join(h.vault.Root, "skills", id+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return toolCallResult{}, fmt.Errorf("skill %q not found (looked at %s)", id, path)
	}
	return ok1(fmt.Sprintf("# Skill: %s\n\n%s", id, data)), nil
}

// read_openspec — reads openspecs/{spec_id}.yaml from vault root
func (h *Handler) readOpenSpec(args map[string]any) (toolCallResult, error) {
	id, ok := stringArg(args, "spec_id")
	if !ok {
		return toolCallResult{}, fmt.Errorf("spec_id is required")
	}
	id = sanitizePath(id)
	path := filepath.Join(h.vault.Root, "openspecs", id+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		// try .yml extension as fallback
		path = filepath.Join(h.vault.Root, "openspecs", id+".yml")
		data, err = os.ReadFile(path)
		if err != nil {
			return toolCallResult{}, fmt.Errorf("openspec %q not found", id)
		}
	}
	return ok1(fmt.Sprintf("# OpenSpec: %s\n\n```yaml\n%s\n```", id, data)), nil
}

// search_knowledge — walks knowledge/ dir and returns files matching the query
func (h *Handler) searchKnowledge(args map[string]any) (toolCallResult, error) {
	query, ok := stringArg(args, "query")
	if !ok {
		return toolCallResult{}, fmt.Errorf("query is required")
	}
	queryLower := strings.ToLower(query)
	knowledgeDir := filepath.Join(h.vault.Root, "knowledge")

	var results []string
	err := filepath.WalkDir(knowledgeDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(h.vault.Root, path)

		// match on filename first (cheap)
		if strings.Contains(strings.ToLower(d.Name()), queryLower) {
			data, readErr := os.ReadFile(path)
			if readErr == nil {
				snippet := truncate(string(data), 300)
				results = append(results, fmt.Sprintf("## %s\n%s", rel, snippet))
			}
			return nil
		}
		// then match on content
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if strings.Contains(strings.ToLower(string(data)), queryLower) {
			snippet := truncate(string(data), 300)
			results = append(results, fmt.Sprintf("## %s\n%s", rel, snippet))
		}
		return nil
	})
	if err != nil {
		return toolCallResult{}, fmt.Errorf("search error: %w", err)
	}
	if len(results) == 0 {
		return ok1(fmt.Sprintf("No knowledge files found matching %q", query)), nil
	}
	return ok1(fmt.Sprintf("Found %d result(s) for %q:\n\n%s",
		len(results), query, strings.Join(results, "\n\n---\n\n"))), nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func ok1(text string) toolCallResult {
	return toolCallResult{Content: []content{{Type: "text", Text: text}}}
}

func stringArg(args map[string]any, key string) (string, bool) {
	v, ok := args[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok && s != ""
}

// sanitizePath strips anything that could escape the vcs-vault-local root
func sanitizePath(s string) string {
	s = filepath.Base(s)                // take only the last component
	s = strings.ReplaceAll(s, "..", "") // belt-and-suspenders
	return s
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
