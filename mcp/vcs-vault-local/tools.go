package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// VaultReader reads files from a local vcs-vault mount.
// In prod, swap for a Gitea API client without changing the Handler.
type VaultReader struct {
	Root string // e.g. /vault (mounted from vcs-vault repo)
}

func (h *Handler) callTool(ctx context.Context, p toolCallParams) (toolCallResult, error) {
	switch p.Name {
	case "read_skill":
		return h.readSkill(p.Arguments)
	case "list_openspec_changes":
		return h.listOpenSpecChanges(p.Arguments)
	case "read_openspec_proposal":
		return h.readOpenSpecProposal(p.Arguments)
	case "read_openspec_spec":
		return h.readOpenSpecSpec(p.Arguments)
	case "search_knowledge":
		return h.searchKnowledge(p.Arguments)
	default:
		return toolCallResult{}, fmt.Errorf("unknown tool: %s", p.Name)
	}
}

// read_skill — reads skills/{skill_id}.md
func (h *Handler) readSkill(args map[string]any) (toolCallResult, error) {
	id, ok := stringArg(args, "skill_id")
	if !ok {
		return toolCallResult{}, fmt.Errorf("skill_id is required")
	}
	path := filepath.Join(h.vault.Root, "skills", sanitizeName(id)+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return toolCallResult{}, fmt.Errorf("skill %q not found", id)
	}
	return ok1(fmt.Sprintf("# Skill: %s\n\n%s", id, data)), nil
}

// list_openspec_changes — lists all changes in openspec/changes/
// Returns each change_id with its roles and spec names.
func (h *Handler) listOpenSpecChanges(_ map[string]any) (toolCallResult, error) {
	changesDir := filepath.Join(h.vault.Root, "openspec", "changes")
	changes, err := os.ReadDir(changesDir)
	if err != nil {
		return toolCallResult{}, fmt.Errorf("openspec/changes not found: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("# OpenSpec Changes\n\n")

	for _, changeEntry := range changes {
		if !changeEntry.IsDir() {
			continue
		}
		changeID := changeEntry.Name()
		sb.WriteString(fmt.Sprintf("## %s\n", changeID))

		roles, _ := os.ReadDir(filepath.Join(changesDir, changeID))
		for _, roleEntry := range roles {
			if !roleEntry.IsDir() {
				continue
			}
			role := roleEntry.Name()
			sb.WriteString(fmt.Sprintf("  role: %s\n", role))

			specsDir := filepath.Join(changesDir, changeID, role, "specs")
			specs, _ := os.ReadDir(specsDir)
			for _, specEntry := range specs {
				if specEntry.IsDir() {
					sb.WriteString(fmt.Sprintf("    spec: %s\n", specEntry.Name()))
				}
			}
		}
		sb.WriteString("\n")
	}

	if sb.Len() == len("# OpenSpec Changes\n\n") {
		return ok1("No openspec changes found."), nil
	}
	return ok1(sb.String()), nil
}

// read_openspec_proposal — reads openspec/changes/{change_id}/{role}/proposal.md
// role is optional; if omitted, the first role directory is used.
func (h *Handler) readOpenSpecProposal(args map[string]any) (toolCallResult, error) {
	changeID, ok := stringArg(args, "change_id")
	if !ok {
		return toolCallResult{}, fmt.Errorf("change_id is required")
	}
	role, _ := stringArg(args, "role")

	changeDir := filepath.Join(h.vault.Root, "openspec", "changes", sanitizeName(changeID))
	resolvedRole, err := resolveRole(changeDir, role)
	if err != nil {
		return toolCallResult{}, err
	}

	path := filepath.Join(changeDir, resolvedRole, "proposal.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return toolCallResult{}, fmt.Errorf("proposal for %q/%q not found", changeID, resolvedRole)
	}
	return ok1(fmt.Sprintf("# Proposal: %s (%s)\n\n%s", changeID, resolvedRole, data)), nil
}

// read_openspec_spec — reads openspec/changes/{change_id}/{role}/specs/{spec_name}/spec.md
// role is optional; if omitted, the first role directory is used.
func (h *Handler) readOpenSpecSpec(args map[string]any) (toolCallResult, error) {
	changeID, ok := stringArg(args, "change_id")
	if !ok {
		return toolCallResult{}, fmt.Errorf("change_id is required")
	}
	specName, ok := stringArg(args, "spec_name")
	if !ok {
		return toolCallResult{}, fmt.Errorf("spec_name is required")
	}
	role, _ := stringArg(args, "role")

	changeDir := filepath.Join(h.vault.Root, "openspec", "changes", sanitizeName(changeID))
	resolvedRole, err := resolveRole(changeDir, role)
	if err != nil {
		return toolCallResult{}, err
	}

	path := filepath.Join(changeDir, resolvedRole, "specs", sanitizeName(specName), "spec.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return toolCallResult{}, fmt.Errorf("spec %q/%q not found under role %q", changeID, specName, resolvedRole)
	}
	return ok1(fmt.Sprintf("# Spec: %s / %s (%s)\n\n%s", changeID, specName, resolvedRole, data)), nil
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

		if strings.Contains(strings.ToLower(d.Name()), queryLower) {
			data, readErr := os.ReadFile(path)
			if readErr == nil {
				results = append(results, fmt.Sprintf("## %s\n%s", rel, truncate(string(data), 300)))
			}
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if strings.Contains(strings.ToLower(string(data)), queryLower) {
			results = append(results, fmt.Sprintf("## %s\n%s", rel, truncate(string(data), 300)))
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

// ── helpers ───────────────────────────────────────────────────────────────────

// resolveRole returns the given role name (sanitized) or auto-detects the first
// role subdirectory in changeDir when role is empty.
func resolveRole(changeDir, role string) (string, error) {
	if role != "" {
		return sanitizeName(role), nil
	}
	entries, err := os.ReadDir(changeDir)
	if err != nil {
		return "", fmt.Errorf("change directory not found: %s", changeDir)
	}
	for _, e := range entries {
		if e.IsDir() {
			return e.Name(), nil
		}
	}
	return "", fmt.Errorf("no role directory found in %s", changeDir)
}

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

// sanitizeName allows alphanumeric, dash, underscore, dot — blocks path traversal.
func sanitizeName(s string) string {
	s = filepath.Base(s)
	s = strings.ReplaceAll(s, "..", "")
	return s
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
