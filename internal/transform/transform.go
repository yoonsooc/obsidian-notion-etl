// Package transform turns an Obsidian note into a Notion page draft through a
// Transformer pipeline. Each note passes through the chain once, in
// registration order; assembly is done by the caller.
package transform

import "fmt"

// Note is the pipeline input (field-compatible with vault.Note, no dependency).
type Note struct {
	Filename    string            // e.g. "DN_251101.md"
	RelPath     string            // path relative to the target dir (used for the Obsidian URI)
	Frontmatter map[string]string // key -> value
	Body        string            // content after the frontmatter
}

// PageDraft is the Notion page draft the pipeline fills in.
type PageDraft struct {
	Title       string            // Name (title) property
	Date        string            // "2006-01-02"; empty when derivation fails
	ObsidianURI string            // url property
	Properties  map[string]string // Notion property name -> value (type resolution is the consumer's job)
	Warnings    []string          // per-item warnings collected during the pipeline (for logging)
}

// Transformer is one pipeline stage. Built-in stages come from this package's
// constructors; custom logic implements the same interface.
type Transformer interface {
	// Name returns the stage name used in logs and error messages.
	Name() string
	// Transform reads the note and fills the draft. An error means the note is skipped.
	Transform(note Note, draft *PageDraft) error
}

// Run applies the chain once in registration order to build the draft.
// The first stage error aborts the run, wrapped with the stage name.
func Run(note Note, chain []Transformer) (*PageDraft, error) {
	draft := &PageDraft{Properties: make(map[string]string)}
	for _, t := range chain {
		if err := t.Transform(note, draft); err != nil {
			return nil, fmt.Errorf("%s: %w", t.Name(), err)
		}
	}
	return draft, nil
}
