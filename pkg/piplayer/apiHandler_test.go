package piplayer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestAPIHandlerConcurrentRequests fires overlapping API calls at the shared
// APIHandler. It fails under -race while the decoded request lives on the
// handler itself, because every caller writes to the same struct - and one
// caller can answer with another's arguments.
func TestAPIHandlerConcurrentRequests(t *testing.T) {
	api := &APIHandler{}
	p := &Player{
		api:         api,
		conf:        &Config{},
		playlist:    &Playlist{},
		ConnViewer:  NewConnWS(),
		ConnControl: NewConnWS(),
	}
	handler := api.Handle(p)

	const callers = 50
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := fmt.Sprintf(`{"component":"caller%d","method":"noop","arguments":{"index":"%d"}}`, i, i)
			request := httptest.NewRequest(http.MethodPost, "/api", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, request)

			var res resMessage
			if err := json.NewDecoder(recorder.Body).Decode(&res); err != nil {
				t.Errorf("caller %d: decoding response failed: %v", i, err)
				return
			}
			// The response names the component it was called with. Seeing
			// another caller's component means the requests crossed over
			// inside the handler.
			want := fmt.Sprintf("Unsupported component: caller%d ", i)
			if got, ok := res.Message.(string); !ok || !strings.Contains(got, want) {
				t.Errorf("caller %d: response was %q, want it to contain %q", i, res.Message, want)
			}
		}(i)
	}
	wg.Wait()
}

func TestAPIHandlerRejectsNonJSON(t *testing.T) {
	api := &APIHandler{}
	p := &Player{api: api, conf: &Config{}, playlist: &Playlist{}, ConnViewer: NewConnWS(), ConnControl: NewConnWS()}

	request := httptest.NewRequest(http.MethodPost, "/api", strings.NewReader("hello"))
	request.Header.Set("Content-Type", "text/plain")
	recorder := httptest.NewRecorder()

	api.Handle(p).ServeHTTP(recorder, request)

	var res resMessage
	if err := json.NewDecoder(recorder.Body).Decode(&res); err != nil {
		t.Fatalf("decoding response failed: %v", err)
	}
	if res.Success {
		t.Error("a request with the wrong Content-Type reported success")
	}
	if recorder.Code != http.StatusUnsupportedMediaType {
		t.Errorf("got status %d, want %d", recorder.Code, http.StatusUnsupportedMediaType)
	}
}

func TestAPIHandlerStatusCodes(t *testing.T) {
	api := &APIHandler{}
	p := &Player{api: api, conf: &Config{}, playlist: &Playlist{}, ConnViewer: NewConnWS(), ConnControl: NewConnWS()}
	handler := api.Handle(p)

	cases := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{"undecodable json", `{not json`, http.StatusBadRequest},
		{"unknown component", `{"component":"nope","method":"x"}`, http.StatusNotFound},
		{"unknown player method", `{"component":"player","method":"explode"}`, http.StatusNotFound},
		{"unknown playlist method", `{"component":"playlist","method":"explode"}`, http.StatusNotFound},
		{"playlist setCurrent with no arguments", `{"component":"playlist","method":"setCurrent"}`, http.StatusBadRequest},
		{"playlist setCurrent out of range", `{"component":"playlist","method":"setCurrent","arguments":{"index":"99"}}`, http.StatusBadRequest},
		{"playlist getCurrent", `{"component":"playlist","method":"getCurrent"}`, http.StatusOK},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api", strings.NewReader(c.body))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, request)

			if recorder.Code != c.wantStatus {
				t.Errorf("got status %d, want %d (body %q)", recorder.Code, c.wantStatus, recorder.Body.String())
			}
		})
	}
}
