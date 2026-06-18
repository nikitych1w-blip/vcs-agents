package dag

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// StepConfig holds the operational metadata derived from a schema artifact.
type StepConfig struct {
	Queue       string   `json:"queue"`        // derived from artifact.generates prefix
	DependsOn   []string `json:"depends_on"`   // from artifact.requires
	Instruction string   `json:"instruction"`  // from artifact.instruction
}

type Step struct {
	Name   string     `json:"name"`
	Config StepConfig `json:"config"`
}

type DAG struct {
	steps map[string]Step
}

// internal types matching schema.yaml structure
type artifact struct {
	ID          string   `yaml:"id"`
	Generates   string   `yaml:"generates"`
	Instruction string   `yaml:"instruction"`
	Requires    []string `yaml:"requires"`
}

type schemaConfig struct {
	Artifacts []artifact `yaml:"artifacts"`
}

// ParseConfig parses a schema.yaml string into a DAG.
func ParseConfig(yamlStr string) (*DAG, error) {
	var cfg schemaConfig
	if err := yaml.Unmarshal([]byte(yamlStr), &cfg); err != nil {
		return nil, fmt.Errorf("parse schema: %w", err)
	}

	d := &DAG{steps: make(map[string]Step, len(cfg.Artifacts))}
	for _, a := range cfg.Artifacts {
		d.steps[a.ID] = Step{
			Name: a.ID,
			Config: StepConfig{
				Queue:       queueFromGenerates(a.Generates),
				DependsOn:   a.Requires,
				Instruction: a.Instruction,
			},
		}
	}
	return d, nil
}

// queueFromGenerates derives the task queue from the artifact's generates path.
// "sa/proposal.md" → "openspec-sa", "be/design.md" → "openspec-be", etc.
func queueFromGenerates(generates string) string {
	parts := strings.SplitN(generates, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		return "openspec-execute"
	}
	return "openspec-" + parts[0]
}

// Validate checks that the DAG has no cycles and all depends_on reference existing steps.
func (d *DAG) Validate() error {
	for _, step := range d.steps {
		for _, dep := range step.Config.DependsOn {
			if _, ok := d.steps[dep]; !ok {
				return fmt.Errorf("step %q depends on unknown step %q", step.Name, dep)
			}
		}
	}

	const (
		unvisited = 0
		inStack   = 1
		done      = 2
	)
	state := make(map[string]int, len(d.steps))

	var dfs func(name string) error
	dfs = func(name string) error {
		switch state[name] {
		case done:
			return nil
		case inStack:
			return fmt.Errorf("cycle detected at step %q", name)
		}
		state[name] = inStack
		for _, dep := range d.steps[name].Config.DependsOn {
			if err := dfs(dep); err != nil {
				return err
			}
		}
		state[name] = done
		return nil
	}

	for name := range d.steps {
		if err := dfs(name); err != nil {
			return err
		}
	}
	return nil
}

// Waves returns topological layers (Kahn's algorithm).
// Steps within the same wave can run in parallel.
func (d *DAG) Waves() [][]Step {
	inDegree := make(map[string]int, len(d.steps))
	dependents := make(map[string][]string, len(d.steps))

	for name, step := range d.steps {
		inDegree[name] += 0
		for _, dep := range step.Config.DependsOn {
			inDegree[name]++
			dependents[dep] = append(dependents[dep], name)
		}
	}

	var queue []string
	for name, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, name)
		}
	}
	sort.Strings(queue)

	var waves [][]Step
	for len(queue) > 0 {
		wave := make([]Step, len(queue))
		for i, name := range queue {
			wave[i] = d.steps[name]
		}
		sort.Slice(wave, func(i, j int) bool { return wave[i].Name < wave[j].Name })
		waves = append(waves, wave)

		var next []string
		for _, name := range queue {
			for _, dep := range dependents[name] {
				inDegree[dep]--
				if inDegree[dep] == 0 {
					next = append(next, dep)
				}
			}
		}
		sort.Strings(next)
		queue = next
	}
	return waves
}

// Step returns a step by name.
func (d *DAG) Step(name string) (Step, bool) {
	s, ok := d.steps[name]
	return s, ok
}

// Steps returns all steps in deterministic order.
func (d *DAG) Steps() []Step {
	steps := make([]Step, 0, len(d.steps))
	for _, s := range d.steps {
		steps = append(steps, s)
	}
	sort.Slice(steps, func(i, j int) bool { return steps[i].Name < steps[j].Name })
	return steps
}
