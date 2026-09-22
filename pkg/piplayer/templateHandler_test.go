package piplayer

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestTemplateHandlerReportsRenderFailure covers a template that parses but
// fails halfway through rendering. It used to write a 200 and a truncated page
// before logging the error; the browser had no way to tell it went wrong.
func TestTemplateHandlerReportsRenderFailure(t *testing.T) {
	api := testAPIHandler(t, map[string]string{
		"broken.html": "<p>{{len .number}}</p>",
	})

	th := templateHandler{
		filename:  "broken.html",
		templates: api.templates,
		// len of an int is an execution error, so the render fails after the
		// template has already started writing.
		data: map[string]any{"number": 42},
	}
	recorder := httptest.NewRecorder()

	th.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/control", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("a failed render returned status %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if strings.Contains(recorder.Body.String(), "<p>") {
		t.Errorf("a failed render sent part of the page: %q", recorder.Body.String())
	}
}

// TestTemplateHandlerMissingTemplate checks a name that isn't in the parsed
// set reports an error instead of panicking, which is what the old code did.
func TestTemplateHandlerMissingTemplate(t *testing.T) {
	api := testAPIHandler(t, map[string]string{"control.html": "ok"})

	th := templateHandler{filename: "nosuch.html", templates: api.templates}
	recorder := httptest.NewRecorder()

	th.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/control", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("a missing template returned status %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}
