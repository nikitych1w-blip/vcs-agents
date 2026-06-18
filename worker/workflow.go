package main

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/nikitych1w-blip/vcs-agents/worker/openspec"
)

const TaskQueue = "openspec-execute"

// ExecuteOpenSpecInput identifies what to build.
// Skills and Knowledge are optional extra context passed at trigger time.
type ExecuteOpenSpecInput struct {
	ChangeID  string   `json:"change_id"`
	SpecName  string   `json:"spec_name"`
	Role      string   `json:"role,omitempty"`
	Model     string   `json:"model,omitempty"`
	Skills    []string `json:"skills,omitempty"`
	Knowledge []string `json:"knowledge,omitempty"`
}

type ExecuteOpenSpecResult struct {
	ChangeID string            `json:"change_id"`
	SpecName string            `json:"spec_name"`
	Outputs  map[string]string `json:"outputs"`
}

func ExecuteOpenSpecWorkflow(ctx workflow.Context, input ExecuteOpenSpecInput) (ExecuteOpenSpecResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("ExecuteOpenSpec started", "change_id", input.ChangeID, "spec_name", input.SpecName)

	result := ExecuteOpenSpecResult{
		ChangeID: input.ChangeID,
		SpecName: input.SpecName,
		Outputs:  make(map[string]string),
	}

	// nil — used only for Temporal method-name resolution, never called in workflow context
	var acts *Activities

	vaultCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 3},
	})
	llmCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 2,
			InitialInterval: 5 * time.Second,
		},
	})

	// 1. Read spec + proposal from vault
	var spec openspec.Spec
	if err := workflow.ExecuteActivity(vaultCtx, acts.ReadSpec, openspec.ReadInput{
		ChangeID: input.ChangeID,
		SpecName: input.SpecName,
		Role:     input.Role,
	}).Get(ctx, &spec); err != nil {
		return result, fmt.Errorf("ReadSpec: %w", err)
	}
	logger.Info("spec loaded", "change_id", spec.ChangeID, "spec_name", spec.SpecName)

	result.Outputs["read_spec"] = fmt.Sprintf(
		"loaded spec (%d chars) and proposal (%d chars)",
		len(spec.Content), len(spec.Proposal),
	)

	// 2. Fetch skills + knowledge (skip if nothing requested)
	var ctxData ContextData
	if len(input.Skills) > 0 || len(input.Knowledge) > 0 {
		if err := workflow.ExecuteActivity(vaultCtx, acts.FetchContext, FetchContextInput{
			Skills:    input.Skills,
			Knowledge: input.Knowledge,
		}).Get(ctx, &ctxData); err != nil {
			return result, fmt.Errorf("FetchContext: %w", err)
		}
		result.Outputs["fetch_context"] = fmt.Sprintf(
			"fetched %d skill(s), %d knowledge entry(ies)",
			len(ctxData.Skills), len(ctxData.Knowledge),
		)
		logger.Info("context fetched", "skills", len(ctxData.Skills), "knowledge", len(ctxData.Knowledge))
	}

	// 3. Generate implementation
	var genResult GenerateCodeResult
	if err := workflow.ExecuteActivity(llmCtx, acts.GenerateCode, GenerateCodeInput{
		Spec:    spec,
		Context: ctxData,
		Model:   input.Model,
	}).Get(ctx, &genResult); err != nil {
		return result, fmt.Errorf("GenerateCode: %w", err)
	}

	result.Outputs["generate_code"] = genResult.Content
	logger.Info("code generated", "model", genResult.Model, "chars", len(genResult.Content))

	// 4. Write artifact to local repo via sc-local MCP (skipped when not configured)
	artifactPath := fmt.Sprintf("output/%s/%s.md", input.ChangeID, input.SpecName)
	var writeResult WriteFilesResult
	if err := workflow.ExecuteActivity(vaultCtx, acts.WriteFiles, WriteFilesInput{
		Path:    artifactPath,
		Content: genResult.Content,
	}).Get(ctx, &writeResult); err != nil {
		return result, fmt.Errorf("WriteFiles: %w", err)
	}
	if !writeResult.Skipped {
		result.Outputs["write_files"] = fmt.Sprintf("%s (%d bytes)", writeResult.Path, writeResult.Bytes)
		logger.Info("artifact written", "path", writeResult.Path, "bytes", writeResult.Bytes)
	}

	return result, nil
}
