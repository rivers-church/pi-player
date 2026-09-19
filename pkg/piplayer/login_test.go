package piplayer

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestHashAndCheckHash(t *testing.T) {
	const password = "correct horse battery staple"

	h, err := hash(password)
	if err != nil {
		t.Fatalf("hash() returned error: %v", err)
	}
	if h == password {
		t.Error("hash() returned the plaintext password unchanged")
	}

	if !checkHash(password, h) {
		t.Error("checkHash() returned false for the correct password")
	}

	if checkHash("wrong password", h) {
		t.Error("checkHash() returned true for an incorrect password")
	}
}

func TestHashIsSalted(t *testing.T) {
	const password = "same password"

	h1, err := hash(password)
	if err != nil {
		t.Fatalf("hash() returned error: %v", err)
	}
	h2, err := hash(password)
	if err != nil {
		t.Fatalf("hash() returned error: %v", err)
	}

	if h1 == h2 {
		t.Error("two hashes of the same password are identical; expected bcrypt salt to differ")
	}

	// Both independently-salted hashes must still verify.
	if !checkHash(password, h1) || !checkHash(password, h2) {
		t.Error("checkHash() failed to verify a salted hash of the correct password")
	}
}

func TestLoginWorksOnFirstAttemptOverHTTP(t *testing.T) {
	// The installer creates an empty Login object. The first submission must
	// initialize the default credentials and authenticate in the same request.
	p := &Player{conf: &Config{}}
	form := url.Values{
		"username": {"admin"},
		"password": {"admin"},
	}.Encode()
	request := httptest.NewRequest(http.MethodPost, "http://piplayer.local/login", strings.NewReader(form))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()

	loginHandler(p, func() error { return nil }).ServeHTTP(recorder, request)
	response := recorder.Result()
	defer response.Body.Close()

	if response.StatusCode != http.StatusFound || response.Header.Get("Location") != "/control" {
		t.Fatalf("first login returned status %d and location %q; want redirect to /control", response.StatusCode, response.Header.Get("Location"))
	}

	cookies := response.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("first login set %d cookies; want 1", len(cookies))
	}
	if cookies[0].Secure {
		t.Fatal("first login set a Secure cookie that browsers will not return to the HTTP server")
	}

	controlRequest := httptest.NewRequest(http.MethodGet, "http://piplayer.local/control", nil)
	controlRequest.AddCookie(cookies[0])
	_, authenticated, err := CheckLogin(httptest.NewRecorder(), controlRequest)
	if err != nil {
		t.Fatalf("reading the first login session failed: %v", err)
	}
	if !authenticated {
		t.Fatal("first login cookie did not create an authenticated session")
	}
}

func TestControlPageLoadsWithoutViewerConnection(t *testing.T) {
	mediaDir := t.TempDir()
	p := &Player{
		api: &APIHandler{
			statTemplates: fstest.MapFS{
				"control.html": &fstest.MapFile{Data: []byte("control page")},
			},
		},
		conf:       &Config{Mount: mount{Dir: mediaDir}},
		playlist:   &Playlist{},
		ConnViewer: NewConnWS(),
	}

	sessionRequest := httptest.NewRequest(http.MethodGet, "http://piplayer.local/control", nil)
	session, err := store.Get(sessionRequest, "piplayer-session")
	if err != nil {
		t.Fatalf("creating authenticated session failed: %v", err)
	}
	session.Values["authenticated"] = "test"
	sessionRecorder := httptest.NewRecorder()
	if err := session.Save(sessionRequest, sessionRecorder); err != nil {
		t.Fatalf("saving authenticated session failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "http://piplayer.local/control", nil)
	request.AddCookie(sessionRecorder.Result().Cookies()[0])
	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		p.HandleControl(recorder, request)
		close(done)
	}()

	select {
	case <-done:
		if recorder.Code != http.StatusOK || strings.TrimSpace(recorder.Body.String()) != "control page" {
			t.Fatalf("control page returned status %d and body %q", recorder.Code, recorder.Body.String())
		}
	case <-time.After(time.Second):
		t.Fatal("control page blocked while no viewer WebSocket was connected")
	}
}

func TestLoginPageRecoversFromUndecodableCookie(t *testing.T) {
	// A cookie signed with a different secret (or one that has outlived the
	// codec's MaxAge) must not turn the login page into a 500, or the user is
	// locked out until they clear their cookies by hand.
	p := &Player{conf: &Config{}}
	form := url.Values{
		"username": {"admin"},
		"password": {"admin"},
	}.Encode()
	request := httptest.NewRequest(http.MethodPost, "http://piplayer.local/login", strings.NewReader(form))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(&http.Cookie{Name: "piplayer-session", Value: "not-a-valid-session-cookie"})
	recorder := httptest.NewRecorder()

	loginHandler(p, func() error { return nil }).ServeHTTP(recorder, request)
	response := recorder.Result()
	defer response.Body.Close()

	if response.StatusCode != http.StatusFound || response.Header.Get("Location") != "/control" {
		t.Fatalf("login with a stale cookie returned status %d and location %q; want redirect to /control", response.StatusCode, response.Header.Get("Location"))
	}

	cookies := response.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("login with a stale cookie set %d cookies; want 1 replacement", len(cookies))
	}

	controlRequest := httptest.NewRequest(http.MethodGet, "http://piplayer.local/control", nil)
	controlRequest.AddCookie(cookies[0])
	_, authenticated, err := CheckLogin(httptest.NewRecorder(), controlRequest)
	if err != nil {
		t.Fatalf("reading the replacement session failed: %v", err)
	}
	if !authenticated {
		t.Fatal("the replacement cookie did not create an authenticated session")
	}
}
