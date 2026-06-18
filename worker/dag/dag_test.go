package dag

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// schemaYAML mirrors the vcs schema.yaml format used in vcs-vault.
const schemaYAML = `
name: vcs
artifacts:
  - id: proposal
    generates: sa/proposal.md
    requires: []
  - id: sa-specs
    generates: sa/specs/example.md
    requires: [proposal]
  - id: be-design
    generates: be/design.md
    requires: [sa-specs]
  - id: fe-design
    generates: fe/design.md
    requires: [sa-specs]
  - id: be-tasks
    generates: be/tasks.md
    requires: [be-design]
  - id: qa-plan
    generates: qa/test-plan.md
    requires: [be-tasks, fe-design]
  - id: qaa-tasks
    generates: qaa/tasks.md
    requires: [qa-plan]
`

func stepNames(steps []Step) []string {
	names := make([]string, len(steps))
	for i, s := range steps {
		names[i] = s.Name
	}
	return names
}

func TestWaves_FullPipeline(t *testing.T) {
	d, err := ParseConfig(schemaYAML)
	require.NoError(t, err)
	require.NoError(t, d.Validate())

	waves := d.Waves()
	require.Len(t, waves, 6)

	assert.Equal(t, []string{"proposal"}, stepNames(waves[0]))
	assert.Equal(t, []string{"sa-specs"}, stepNames(waves[1]))
	assert.Equal(t, []string{"be-design", "fe-design"}, stepNames(waves[2]), "parallel wave")
	assert.Equal(t, []string{"be-tasks"}, stepNames(waves[3]))
	assert.Equal(t, []string{"qa-plan"}, stepNames(waves[4]))
	assert.Equal(t, []string{"qaa-tasks"}, stepNames(waves[5]))
}

func TestQueueDerivation(t *testing.T) {
	d, err := ParseConfig(schemaYAML)
	require.NoError(t, err)

	cases := map[string]string{
		"proposal":  "openspec-sa",
		"sa-specs":  "openspec-sa",
		"be-design": "openspec-be",
		"fe-design": "openspec-fe",
		"be-tasks":  "openspec-be",
		"qa-plan":   "openspec-qa",
		"qaa-tasks": "openspec-qaa",
	}
	for stepName, wantQueue := range cases {
		s, ok := d.Step(stepName)
		require.True(t, ok, "step %q not found", stepName)
		assert.Equal(t, wantQueue, s.Config.Queue, "step %q", stepName)
	}
}

func TestValidate_Cycle(t *testing.T) {
	cycleYAML := `
name: test
artifacts:
  - id: a
    generates: sa/a.md
    requires: [c]
  - id: b
    generates: sa/b.md
    requires: [a]
  - id: c
    generates: sa/c.md
    requires: [b]
`
	d, err := ParseConfig(cycleYAML)
	require.NoError(t, err)
	require.ErrorContains(t, d.Validate(), "cycle")
}

func TestValidate_UnknownDep(t *testing.T) {
	unknownYAML := `
name: test
artifacts:
  - id: step-a
    generates: sa/a.md
    requires: [nonexistent]
`
	d, err := ParseConfig(unknownYAML)
	require.NoError(t, err)
	require.ErrorContains(t, d.Validate(), "nonexistent")
}

func TestParseConfig_Empty(t *testing.T) {
	d, err := ParseConfig(`name: test`)
	require.NoError(t, err)
	require.NoError(t, d.Validate())
	assert.Empty(t, d.Steps())
	assert.Empty(t, d.Waves())
}
