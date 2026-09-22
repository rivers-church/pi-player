package piplayer

import (
	"bytes"
	"html/template"
	"net/http"
)

// TemplateHandler renders one of the parsed html templates.
type TemplateHandler struct {
	filename  string
	data      map[string]any
	templates *template.Template
}

// ServeHTTP handles HTTP requests for the templates
func (t *TemplateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if t.templates == nil {
		logger.Error("no templates available to render", "page", t.filename)
		http.Error(w, "Could not render the page.", http.StatusInternalServerError)
		return
	}

	// Render into a buffer first. Writing straight to the response commits a
	// 200 and part of the page before a failure halfway through the template
	// can be reported, which leaves the browser with a truncated page and the
	// error only in the log.
	var page bytes.Buffer
	if err := t.templates.ExecuteTemplate(&page, t.filename, t.data); err != nil {
		logger.Error("rendering the page failed", "page", t.filename, "error", err)
		http.Error(w, "Could not render the page.", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := page.WriteTo(w); err != nil {
		logger.Error("writing the page failed", "page", t.filename, "error", err)
	}
}
