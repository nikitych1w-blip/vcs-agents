package main

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/nikitych1w-blip/vcs-agents/worker/dag"
	"github.com/nikitych1w-blip/vcs-agents/worker/openspec"
)

const TaskQueue = "openspec-execute"

// ExecuteOpenSpecInput identifies the change to process.
// ConfigRef pins the pipeline config to a specific git SHA; empty means "main".
type ExecuteOpenSpecInput struct {
	ChangeID  string   `json:"change_id"`
	SpecName  string   `json:"spec_name"`
	Role      string   `json:"role,omitempty"`
	Model     string   `json:"model,omitempty"`
	ConfigRef string   `json:"config_ref,omitempty"`
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

	orchCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 3},
	})

	// 1. Fetch pipeline config once and pin it for this workflow run.
	var configYAML string
	if err := workflow.ExecuteActivity(orchCtx, acts.FetchConfig, FetchConfigInput{
		Ref: input.ConfigRef,
	}).Get(ctx, &configYAML); err != nil {
		return result, fmt.Errorf("FetchConfig: %w", err)
	}

	// 2. Read spec from vault.
	var spec openspec.Spec
	if err := workflow.ExecuteActivity(orchCtx, acts.ReadSpec, openspec.ReadInput{
		ChangeID: input.ChangeID,
		SpecName: input.SpecName,
		Role:     input.Role,
	}).Get(ctx, &spec); err != nil {
		return result, fmt.Errorf("ReadSpec: %w", err)
	}
	logger.Info("spec loaded", "change_id", spec.ChangeID, "spec_name", spec.SpecName)

	// 3. Parse and validate DAG — pure, deterministic, no I/O.
	d, err := dag.ParseConfig(configYAML)
	if err != nil {
		return result, fmt.Errorf("parse pipeline config: %w", err)
	}
	if err := d.Validate(); err != nil {
		return result, fmt.Errorf("invalid pipeline config: %w", err)
	}

	waves := d.Waves()
	logger.Info("pipeline loaded", "steps", len(d.Steps()), "waves", len(waves))

	// 4. Execute waves; within each wave all steps run in parallel.
	completed := make(map[string]string)

	for waveIdx, wave := range waves {
		logger.Info("wave start", "wave", waveIdx, "steps", stepNames(wave))

		futures := make([]workflow.Future, len(wave))
		for i, step := range wave {
			stepCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
				StartToCloseTimeout: 30 * time.Minute,
				TaskQueue:           step.Config.Queue,
				RetryPolicy: &temporal.RetryPolicy{
					MaximumAttempts: 2,
					InitialInterval: 5 * time.Second,
				},
			})

			futures[i] = workflow.ExecuteActivity(stepCtx, acts.RunStep, RunStepInput{
				Step:     step,
				ChangeID: input.ChangeID,
				SpecName: input.SpecName,
				Spec:     spec,
				Context:  copyMap(completed),
				Model:    input.Model,
			})
		}

		for i, future := range futures {
			var stepResult RunStepResult
			if err := future.Get(ctx, &stepResult); err != nil {
				return result, fmt.Errorf("step %s: %w", wave[i].Name, err)
			}
			completed[stepResult.StepName] = stepResult.Content
			result.Outputs[stepResult.StepName] = stepResult.Content
			logger.Info("step done", "step", stepResult.StepName, "chars", len(stepResult.Content))
		}

		logger.Info("wave done", "wave", waveIdx)
	}

	return result, nil
}

func stepNames(steps []dag.Step) []string {
	names := make([]string, len(steps))
	for i, s := range steps {
		names[i] = s.Name
	}
	return names
}

func copyMap(m map[string]string) map[string]string {
	cp := make(map[string]string, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}
