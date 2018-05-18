package piplayer

import (
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"sync"
)

// TemplateHandler handles rendering html templates
type TemplateHandler struct {
	once     sync.Once
	filename string
	templ    *template.Template
	data     map[string]interface{}
}

const templateDir = "pkg/piplayer/templates"

// NewTemplateHandler returns a new template handler for a specific page
func NewTemplateHandler(filename string) TemplateHandler {
	return TemplateHandler{filename: filename}
}

// ServeHTTP handles HTTP requests for the templates
func (t *TemplateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// once keeps track of which of these anonymous functions have already been called,
	// and stores their result. If they are called again it just returns the stored result.
	// t.once.Do(func(){
	t.templ = template.Must(template.ParseFiles(filepath.Join(templateDir, t.filename)))
	// // })
	// data := map[string]string{
	// 	"Host": r.Host,
	// }

	err := t.templ.Execute(w, t.data)
	if err != nil {
		log.Println("Error trying to render page: ", t.filename, err)
	}
}
