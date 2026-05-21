package mail

import _ "embed"

// Template names selectable through the mail.template configuration option.
const (
	// TemplateModern is the default template: a modern, card-based design.
	TemplateModern = "modern"
	// TemplateClassic mirrors the look of the stock Prometheus Alertmanager
	// e-mail notification.
	TemplateClassic = "classic"
)

// modernTemplate and classicTemplate are the embedded HTML body templates.
// They are rendered with html/template so every label and annotation value
// is automatically escaped.
//
//go:embed modern.html
var modernTemplate string

//go:embed classic.html
var classicTemplate string

// templateSources maps each template name to its embedded HTML source.
var templateSources = map[string]string{
	TemplateModern:  modernTemplate,
	TemplateClassic: classicTemplate,
}

// IsValidTemplate reports whether name is a known e-mail template.
func IsValidTemplate(name string) bool {
	_, ok := templateSources[name]
	return ok
}

// TemplateNames returns the known template names in a stable order.
func TemplateNames() []string {
	return []string{TemplateModern, TemplateClassic}
}
