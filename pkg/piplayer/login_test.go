package piplayer

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
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
	// The installer creates an empty login object. The first submission must
	// initialize the default credentials and authenticate in the same request.
	p := newTestPlayer(t)
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
	_, authenticated, err := p.checkLogin(httptest.NewRecorder(), controlRequest)
	if err != nil {
		t.Fatalf("reading the first login session failed: %v", err)
	}
	if !authenticated {
		t.Fatal("first login cookie did not create an authenticated session")
	}
}

func TestControlPageLoadsWithoutViewerConnection(t *testing.T) {
	mediaDir := t.TempDir()
	p := newTestPlayer(t,
		withTemplates(map[string]string{"control.html": "control page"}),
		withMediaDir(mediaDir))

	sessionRequest := httptest.NewRequest(http.MethodGet, "http://piplayer.local/control", nil)
	session, err := p.store.Get(sessionRequest, "piplayer-session")
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
		p.handleControl(recorder, request)
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
	p := newTestPlayer(t)
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
	_, authenticated, err := p.checkLogin(httptest.NewRecorder(), controlRequest)
	if err != nil {
		t.Fatalf("reading the replacement session failed: %v", err)
	}
	if !authenticated {
		t.Fatal("the replacement cookie did not create an authenticated session")
	}
}

// TestSessionKeyIsPerDevice checks a cookie minted by one player is refused by
// another. With the key hardcoded in the source, every player on earth shared
// one and anyone could forge an authenticated session.
func TestSessionKeyIsPerDevice(t *testing.T) {
	first, err := newSessionKey()
	if err != nil {
		t.Fatalf("generating a session key failed: %v", err)
	}
	second, err := newSessionKey()
	if err != nil {
		t.Fatalf("generating a second session key failed: %v", err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("two generated session keys are identical")
	}

	mint := newTestPlayer(t, withSessionKey(first))
	other := newTestPlayer(t, withSessionKey(second))

	request := httptest.NewRequest(http.MethodGet, "http://piplayer.local/control", nil)
	session, err := mint.store.Get(request, "piplayer-session")
	if err != nil {
		t.Fatalf("creating a session failed: %v", err)
	}
	session.Values["authenticated"] = "test"
	recorder := httptest.NewRecorder()
	if err := session.Save(request, recorder); err != nil {
		t.Fatalf("saving the session failed: %v", err)
	}
	cookie := recorder.Result().Cookies()[0]

	// A separate request per check: gorilla caches the session on the request
	// by name, so reusing one request would hand back the first result.
	withCookie := func() *http.Request {
		request := httptest.NewRequest(http.MethodGet, "http://piplayer.local/control", nil)
		request.AddCookie(cookie)
		return request
	}

	if _, authenticated, _ := mint.checkLogin(httptest.NewRecorder(), withCookie()); !authenticated {
		t.Error("the player that signed the cookie did not accept it")
	}
	if _, authenticated, _ := other.checkLogin(httptest.NewRecorder(), withCookie()); authenticated {
		t.Error("a player with a different key accepted the forged cookie")
	}
}

// TestLoginClearsPreviousSessionValues checks a successful login doesn't carry
// over whatever the incoming cookie happened to hold.
func TestLoginClearsPreviousSessionValues(t *testing.T) {
	p := newTestPlayer(t)

	stale := httptest.NewRequest(http.MethodGet, "http://piplayer.local/login", nil)
	session, err := p.store.Get(stale, "piplayer-session")
	if err != nil {
		t.Fatalf("creating a session failed: %v", err)
	}
	session.Values["leftover"] = "should not survive"
	staleRecorder := httptest.NewRecorder()
	if err := session.Save(stale, staleRecorder); err != nil {
		t.Fatalf("saving the session failed: %v", err)
	}

	form := url.Values{"username": {"admin"}, "password": {"admin"}}.Encode()
	request := httptest.NewRequest(http.MethodPost, "http://piplayer.local/login", strings.NewReader(form))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(staleRecorder.Result().Cookies()[0])
	recorder := httptest.NewRecorder()

	loginHandler(p, func() error { return nil }).ServeHTTP(recorder, request)

	check := httptest.NewRequest(http.MethodGet, "http://piplayer.local/control", nil)
	check.AddCookie(recorder.Result().Cookies()[0])
	session, authenticated, err := p.checkLogin(httptest.NewRecorder(), check)
	if err != nil {
		t.Fatalf("reading the new session failed: %v", err)
	}
	if !authenticated {
		t.Fatal("the login did not authenticate the session")
	}
	if _, ok := session.Values["leftover"]; ok {
		t.Error("a value from the previous session survived the login")
	}
}

// TestLoginLimiterLocksOutAfterFailures covers the counting itself, on a
// stopped clock so the test doesn't have to wait out a real window.
func TestLoginLimiterLocksOutAfterFailures(t *testing.T) {
	now := time.Now()
	l := newLoginLimiter()
	l.now = func() time.Time { return now }

	for i := range loginMaxFailures {
		if ok, _ := l.allow("10.0.0.1"); !ok {
			t.Fatalf("refused attempt %d of %d", i+1, loginMaxFailures)
		}
		l.failed("10.0.0.1")
	}

	ok, retryAfter := l.allow("10.0.0.1")
	if ok {
		t.Error("the client was allowed to keep trying past the limit")
	}
	if retryAfter <= 0 || retryAfter > loginFailureWindow {
		t.Errorf("retryAfter is %s, want it inside the window", retryAfter)
	}

	// Another client is unaffected.
	if ok, _ := l.allow("10.0.0.2"); !ok {
		t.Error("a different client was locked out too")
	}

	// The window passes.
	now = now.Add(loginFailureWindow + time.Second)
	if ok, _ := l.allow("10.0.0.1"); !ok {
		t.Error("the client is still locked out after the window passed")
	}
}

func TestLoginLimiterResetsOnSuccess(t *testing.T) {
	l := newLoginLimiter()
	for range loginMaxFailures {
		l.failed("10.0.0.1")
	}
	if ok, _ := l.allow("10.0.0.1"); ok {
		t.Fatal("the client should be locked out at this point")
	}

	l.succeeded("10.0.0.1")

	if ok, _ := l.allow("10.0.0.1"); !ok {
		t.Error("a successful login did not clear the failures")
	}
}

// TestLoginLimiterForgetsOldClients checks the map doesn't grow without bound
// as clients come and go.
func TestLoginLimiterForgetsOldClients(t *testing.T) {
	now := time.Now()
	l := newLoginLimiter()
	l.now = func() time.Time { return now }

	for i := range 100 {
		l.failed(fmt.Sprintf("10.0.0.%d", i))
	}
	if len(l.failures) != 100 {
		t.Fatalf("tracking %d clients, want 100", len(l.failures))
	}

	now = now.Add(loginFailureWindow + time.Second)
	l.failed("10.0.1.1")

	if len(l.failures) != 1 {
		t.Errorf("tracking %d clients after the window passed, want only the recent one", len(l.failures))
	}
}

// TestLoginRefusesAfterRepeatedFailures drives the handler itself, and checks
// the refusal happens without hashing a password - the point is to stop a
// client from spending the device's CPU, not just to stop guessing.
func TestLoginRefusesAfterRepeatedFailures(t *testing.T) {
	p := newTestPlayer(t, withTemplates(map[string]string{"login.html": "login page"}))
	creds, err := newLogin()
	if err != nil {
		t.Fatalf("creating the default login failed: %v", err)
	}
	p.conf.setCredentials(creds)

	hashes := 0
	countingHandler := loginHandler(p, func() error { return nil })
	attempt := func(password string) *httptest.ResponseRecorder {
		t.Helper()
		form := url.Values{"username": {"admin"}, "password": {password}}.Encode()
		request := httptest.NewRequest(http.MethodPost, "http://piplayer.local/login", strings.NewReader(form))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.RemoteAddr = "192.168.1.50:41000"
		recorder := httptest.NewRecorder()
		countingHandler.ServeHTTP(recorder, request)
		return recorder
	}

	for i := range loginMaxFailures {
		if got := attempt("wrong").Code; got != http.StatusOK {
			t.Fatalf("attempt %d returned %d, want the login page back", i+1, got)
		}
		hashes++
	}

	locked := attempt("wrong")
	if locked.Code != http.StatusTooManyRequests {
		t.Fatalf("attempt %d returned %d, want %d", hashes+1, locked.Code, http.StatusTooManyRequests)
	}
	if locked.Header().Get("Retry-After") == "" {
		t.Error("the refusal has no Retry-After header")
	}

	// Even the right password is refused while the client is locked out.
	if got := attempt("admin").Code; got != http.StatusTooManyRequests {
		t.Errorf("the correct password returned %d while locked out, want %d", got, http.StatusTooManyRequests)
	}

	// A different client is unaffected.
	form := url.Values{"username": {"admin"}, "password": {"admin"}}.Encode()
	request := httptest.NewRequest(http.MethodPost, "http://piplayer.local/login", strings.NewReader(form))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.RemoteAddr = "192.168.1.51:41000"
	recorder := httptest.NewRecorder()
	countingHandler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusFound {
		t.Errorf("another client got %d, want a successful login", recorder.Code)
	}
}
