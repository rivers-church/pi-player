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
			// The generic echo response repeats the component it was called
			// with. Seeing another caller's component means the requests
			// crossed over inside the handler.
			want := fmt.Sprintf("component: caller%d\n", i)
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
}
