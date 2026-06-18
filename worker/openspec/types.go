package openspec

// Spec holds the resolved content of an openspec change.
type Spec struct {
	ChangeID string
	SpecName string
	Role     string
	Content  string // spec.md raw markdown
	Proposal string // proposal.md raw markdown, may be empty
}

// ReadInput is the input for the ReadSpec activity.
type ReadInput struct {
	ChangeID string `json:"change_id"`
	SpecName string `json:"spec_name"`
	Role     string `json:"role,omitempty"`
}
