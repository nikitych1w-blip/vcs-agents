package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RepoWriter provides read-write access to a local repository mount.
// In prod, swap for a Gitea API client without changing the Handler.
type RepoWriter struct {
	Root string // e.g. /repo (mounted local git repo clone)
}

func (h *Handler) callTool(_ context.Context, p toolCallParams) (toolCallResult, error) {
	switch p.Name {
	case "write_file":
		return h.writeFile(p.Arguments)
	case "read_file":
		return h.readFile(p.Arguments)
	case "list_files":
		return h.listFiles(p.Arguments)
	default:
		return toolCallResult{}, fmt.Errorf("unknown tool: %s", p.Name)
	}
}

// write_file creates or overwrites a file. Parent directories are created as needed.
func (h *Handler) writeFile(args map[string]any) (toolCallResult, error) {
	path, ok := stringArg(args, "path")
	if !ok {
		return toolCallResult{}, fmt.Errorf("path is required")
	}
	// content may be empty — don't use stringArg which rejects empty strings
	rawContent, exists := args["content"]
	if !exists {
		return toolCallResult{}, fmt.Errorf("content is required")
	}
	content, ok := rawContent.(string)
	if !ok {
		return toolCallResult{}, fmt.Errorf("content must be a string")
	}

	safePath, err := sanitizeWritePath(path)
	if err != nil {
		return toolCallResult{}, err
	}

	fullPath := filepath.Join(h.repo.Root, safePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return toolCallResult{}, fmt.Errorf("mkdir %s: %w", filepath.Dir(safePath), err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		return toolCallResult{}, fmt.Errorf("write %s: %w", safePath, err)
	}

	return ok1(fmt.Sprintf("wrote %d bytes to %s", len(content), safePath)), nil
}

// read_file returns the contents of a file.
func (h *Handler) readFile(args map[string]any) (toolCallResult, error) {
	path, ok := stringArg(args, "path")
	if !ok {
		return toolCallResult{}, fmt.Errorf("path is required")
	}
	safePath, err := sanitizeWritePath(path)
	if err != nil {
		return toolCallResult{}, err
	}

	data, err := os.ReadFile(filepath.Join(h.repo.Root, safePath))
	if err != nil {
		return toolCallResult{}, fmt.Errorf("file %q not found", safePath)
	}
	return ok1(fmt.Sprintf("# %s\n\n%s", safePath, data)), nil
}

// list_files returns a directory listing. Defaults to the repo root.
func (h *Handler) listFiles(args map[string]any) (toolCallResult, error) {
	path, _ := stringArg(args, "path") // optional

	dir := h.repo.Root
	displayPath := "."
	if path != "" {
		safePath, err := sanitizeWritePath(path)
		if err != nil {
			return toolCallResult{}, err
		}
		dir = filepath.Join(h.repo.Root, safePath)
		displayPath = safePath
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return ok1(fmt.Sprintf("%s: (empty)", displayPath)), nil
		}
		return toolCallResult{}, fmt.Errorf("list %s: %w", displayPath, err)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s\n\n", displayPath)
	for _, e := range entries {
		if e.IsDir() {
			fmt.Fprintf(&sb, "  %s/\n", e.Name())
		} else {
			fmt.Fprintf(&sb, "  %s\n", e.Name())
		}
	}
	if len(entries) == 0 {
		sb.WriteString("  (empty)\n")
	}
	return ok1(sb.String()), nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// sanitizeWritePath rejects absolute paths and directory traversal.
// Subdirectories like "src/api/routes.yaml" are allowed.
func sanitizeWritePath(path string) (string, error) {
	clean := filepath.Clean(path)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes repo root", path)
	}
	return clean, nil
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
