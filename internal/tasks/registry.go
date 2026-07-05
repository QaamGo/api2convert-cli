// Package tasks defines the high-level conversion verbs (compress, merge,
// thumbnail, resize) as thin presets over the conversion pipeline. Mappings are
// grounded in the live api2convert catalog (the `operation` category) and the
// documented merge option.
package tasks

// Kind controls how a verb maps onto the conversion pipeline.
type Kind int

const (
	// SameFormat converts each input to its own format (compress/optimize).
	SameFormat Kind = iota
	// ToTarget converts inputs to a single target (from --to or DefaultTarget).
	ToTarget
	// Merge combines multiple inputs into one output.
	Merge
)

// Verb is a high-level task exposed as its own subcommand.
type Verb struct {
	Name           string
	Aliases        []string
	Short          string
	Long           string
	Example        string
	Kind           Kind
	Category       string         // conversion category to set (e.g. "operation")
	DefaultTarget  string         // used when --to is omitted (ToTarget / Merge)
	DefaultOptions map[string]any // baseline options merged under any --option
	// Gated verbs are checked against the catalog before running; CapTarget is the
	// catalog target that must exist for the verb to be available (defaults to the
	// resolved target). A friendly "not available" message is shown otherwise.
	Gated     bool
	CapTarget string
}

// Registry returns the built-in verbs. Each maps to a real api2convert
// capability confirmed against the live catalog / API behaviour.
func Registry() []Verb {
	return []Verb{
		{
			Name:    "compress",
			Aliases: []string{"optimize"},
			Short:   "Compress or optimize a file (keeps the same format)",
			Long:    "Compress or optimize files by re-encoding each to its own format. Pass --to to change format, or --option quality=NN to tune.",
			Example: "  api2convert compress big-scan.pdf\n  api2convert compress *.jpg --option quality=75 --out-dir small/",
			Kind:    SameFormat,
		},
		{
			Name:           "merge",
			Short:          "Merge multiple inputs into one file",
			Long:           "Combine several inputs into a single output (e.g. many PDFs into one). Uses the target's merge support.",
			Example:        "  api2convert merge a.pdf b.pdf c.pdf --to pdf -o combined.pdf",
			Kind:           Merge,
			DefaultTarget:  "pdf",
			DefaultOptions: map[string]any{"merge": true},
		},
		{
			Name:          "thumbnail",
			Short:         "Create a thumbnail image",
			Long:          "Create a smaller preview image. Size it with --option width=NN --option height=NN.",
			Example:       "  api2convert thumbnail photo.png --option width=320",
			Kind:          ToTarget,
			Category:      "operation",
			DefaultTarget: "thumbnail",
			Gated:         true,
			CapTarget:     "thumbnail",
		},
		{
			Name:          "resize",
			Short:         "Resize an image",
			Long:          "Resize an image. Size it with --option width=NN --option height=NN.",
			Example:       "  api2convert resize banner.png --option width=1200",
			Kind:          ToTarget,
			Category:      "operation",
			DefaultTarget: "resize-image",
			Gated:         true,
			CapTarget:     "resize-image",
		},
	}
}
