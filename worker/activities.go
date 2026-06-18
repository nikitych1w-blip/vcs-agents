package main

import (
	"context"
	"fmt"
	"strings"

	"go.temporal.io/sdk/activity"

	"github.com/nikitych1w-blip/vcs-agents/worker/llm"
	"github.com/nikitych1w-blip/vcs-agents/worker/openspec"
	"github.com/nikitych1w-blip/vcs-agents/worker/sc"
	"github.com/nikitych1w-blip/vcs-agents/worker/vault"
)

type Activities struct {
	vault        *vault.Client
	llm          *llm.Client
	sc           *sc.Client // nil when SC_LOCAL_MCP_URL is not set
	defaultModel string
}

// ── vault activities ──────────────────────────────────────────────────────────

// ReadSpec reads spec.md and proposal.md for a given change and spec name.
// role is optional; vault-mcp auto-detects it when empty.
func (a *Activities) ReadSpec(ctx context.Context, input openspec.ReadInput) (openspec.Spec, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("reading spec", "change_id", input.ChangeID, "spec_name", input.SpecName)

	specContent, err := a.vault.ReadOpenSpecSpec(ctx, input.ChangeID, input.SpecName, input.Role)
	if err != nil {
		return openspec.Spec{}, fmt.Errorf("vault ReadOpenSpecSpec: %w", err)
	}

	proposal, err := a.vault.ReadOpenSpecProposal(ctx, input.ChangeID, input.Role)
	if err != nil {
		// Proposal is optional — log and continue
		logger.Warn("proposal not found", "change_id", input.ChangeID, "error", err)
	}

	return openspec.Spec{
		ChangeID: input.ChangeID,
		SpecName: input.SpecName,
		Role:     input.Role,
		Content:  specContent,
		Proposal: proposal,
	}, nil
}

// ContextData holds skills and knowledge retrieved from the vault.
type ContextData struct {
	Skills    map[string]string
	Knowledge []string
}

// FetchContext fetches skills and knowledge referenced in the workflow input.
func (a *Activities) FetchContext(ctx context.Context, input FetchContextInput) (ContextData, error) {
	logger := activity.GetLogger(ctx)
	result := ContextData{Skills: make(map[string]string)}

	for _, skillID := range input.Skills {
		logger.Info("fetching skill", "skill_id", skillID)
		content, err := a.vault.ReadSkill(ctx, skillID)
		if err != nil {
			logger.Warn("skill not found", "skill_id", skillID, "error", err)
			continue
		}
		result.Skills[skillID] = content
	}

	for _, query := range input.Knowledge {
		logger.Info("searching knowledge", "query", query)
		content, err := a.vault.SearchKnowledge(ctx, query)
		if err != nil {
			logger.Warn("knowledge search failed", "query", query, "error", err)
			continue
		}
		result.Knowledge = append(result.Knowledge, content)
	}

	return result, nil
}

// FetchContextInput carries optional skill IDs and knowledge queries.
type FetchContextInput struct {
	Skills    []string
	Knowledge []string
}

// ── llm activities ────────────────────────────────────────────────────────────

// GenerateCodeInput carries everything the LLM needs to implement the spec.
type GenerateCodeInput struct {
	Spec    openspec.Spec
	Context ContextData
	Model   string
}

// GenerateCodeResult holds the LLM output and the model used.
type GenerateCodeResult struct {
	Content string
	Model   string
}

// GenerateCode calls LiteLLM with the spec + proposal + context and returns
// a generated implementation.
func (a *Activities) GenerateCode(ctx context.Context, input GenerateCodeInput) (GenerateCodeResult, error) {
	logger := activity.GetLogger(ctx)

	model := input.Model
	if model == "" {
		model = a.defaultModel
	}
	logger.Info("generating code", "change_id", input.Spec.ChangeID, "spec_name", input.Spec.SpecName, "model", model)

	prompt := buildDevPrompt(input.Spec, input.Context)
	content, err := a.llm.Chat(ctx, model, []llm.Message{
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return GenerateCodeResult{}, fmt.Errorf("llm chat: %w", err)
	}

	logger.Info("generation complete", "chars", len(content))
	return GenerateCodeResult{Content: content, Model: model}, nil
}

// ── sc activities ─────────────────────────────────────────────────────────────

// WriteFilesInput specifies where to write the generated artifact.
type WriteFilesInput struct {
	Path    string // relative path inside the repo
	Content string // file content
}

// WriteFilesResult describes the outcome of a write operation.
type WriteFilesResult struct {
	Path    string
	Bytes   int
	Skipped bool // true when SC_LOCAL_MCP_URL is not set
}

// WriteFiles writes generated content to the local repo via sc-local MCP.
// When the sc client is not configured (nil), the step is skipped silently.
func (a *Activities) WriteFiles(ctx context.Context, input WriteFilesInput) (WriteFilesResult, error) {
	logger := activity.GetLogger(ctx)
	if a.sc == nil {
		logger.Info("sc MCP not configured, skipping write", "path", input.Path)
		return WriteFilesResult{Skipped: true}, nil
	}

	msg, err := a.sc.WriteFile(ctx, input.Path, input.Content)
	if err != nil {
		return WriteFilesResult{}, fmt.Errorf("sc WriteFile %q: %w", input.Path, err)
	}

	logger.Info("artifact written", "path", input.Path, "bytes", len(input.Content), "msg", msg)
	return WriteFilesResult{Path: input.Path, Bytes: len(input.Content)}, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func buildDevPrompt(spec openspec.Spec, ctx ContextData) string {
	var sb strings.Builder

	sb.WriteString("You are a software developer. Implement the following task based on the provided spec and context.\n\n")

	if spec.Proposal != "" {
		sb.WriteString("## Business intent (proposal)\n\n")
		sb.WriteString(spec.Proposal)
		sb.WriteString("\n\n")
	}

	sb.WriteString("## Requirements (spec)\n\n")
	sb.WriteString(spec.Content)
	sb.WriteString("\n\n")

	if len(ctx.Skills) > 0 {
		sb.WriteString("## Skills / patterns\n\n")
		for id, content := range ctx.Skills {
			fmt.Fprintf(&sb, "### %s\n\n%s\n\n", id, content)
		}
	}

	if len(ctx.Knowledge) > 0 {
		sb.WriteString("## Knowledge base\n\n")
		for _, k := range ctx.Knowledge {
			sb.WriteString(k)
			sb.WriteString("\n\n")
		}
	}

	sb.WriteString("## Expected output\n\n")
	sb.WriteString("Provide the implementation: file paths, code changes, and a brief explanation.")
	return sb.String()
}
