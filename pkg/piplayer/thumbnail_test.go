package piplayer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeExtractor stands in for ffmpeg so the suite never needs it installed.
type fakeExtractor struct {
	calls   atomic.Int64
	err     error
	empty   bool          // exit cleanly having written nothing
	delay   time.Duration // pretend to be slow
	started chan struct{}
}

func (f *fakeExtractor) extract(ctx context.Context, _, dst string, _ int) error {
	f.calls.Add(1)
	if f.started != nil {
		select {
		case f.started <- struct{}{}:
		default:
		}
	}
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if f.err != nil {
		return f.err
	}
	if f.empty {
		return os.WriteFile(dst, nil, 0o644)
	}
	return os.WriteFile(dst, []byte("jpeg-ish"), 0o644)
}

func newTestThumbnailer(t *testing.T, fake *fakeExtractor) *thumbnailer {
	t.Helper()
	tn := newThumbnailer(t.TempDir())
	if tn == nil {
		t.Fatal("could not build a thumbnailer over a temp dir")
	}
	tn.extract = fake.extract
	return tn
}

// mediaFile writes a file and returns its path and info.
func mediaFile(t *testing.T, dir, name, content string) (string, os.FileInfo) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return path, info
}

func TestThumbnailIsGeneratedOnceAndThenCached(t *testing.T) {
	fake := &fakeExtractor{}
	tn := newTestThumbnailer(t, fake)
	src, info := mediaFile(t, t.TempDir(), "clip.mp4", "video")

	first, err := tn.get(t.Context(), src, info)
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	second, err := tn.get(t.Context(), src, info)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}

	if first != second {
		t.Errorf("got two different cache paths, %q and %q", first, second)
	}
	if got := fake.calls.Load(); got != 1 {
		t.Errorf("the extractor ran %d times, want once - the second request should be served from cache", got)
	}
}

// TestConcurrentRequestsShareOneGeneration is the reason for the pending map:
// a control page with fifty rows must not start fifty ffmpeg runs of the same
// file.
func TestConcurrentRequestsShareOneGeneration(t *testing.T) {
	fake := &fakeExtractor{delay: 50 * time.Millisecond}
	tn := newTestThumbnailer(t, fake)
	src, info := mediaFile(t, t.TempDir(), "clip.mp4", "video")

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := tn.get(t.Context(), src, info); err != nil {
				t.Errorf("concurrent request failed: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := fake.calls.Load(); got != 1 {
		t.Errorf("the extractor ran %d times for ten concurrent requests, want once", got)
	}
}

// TestReplacedFileGetsANewThumbnail is the case the media directory actually
// produces: a file swapped for a different one under the same name, sometimes
// daily. A cache keyed on the name alone would serve yesterday's frame.
func TestReplacedFileGetsANewThumbnail(t *testing.T) {
	dir := t.TempDir()
	src, before := mediaFile(t, dir, "notices.mp4", "monday")

	if thumbToken("notices.mp4", before) == "" {
		t.Fatal("empty token")
	}

	// Replace it with different content, and make sure the timestamp really
	// moved - a same-second replacement is entirely possible.
	_, after := mediaFile(t, dir, "notices.mp4", "tuesday, and longer")
	if err := os.Chtimes(src, time.Now().Add(time.Minute), time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}

	if thumbToken("notices.mp4", before) == thumbToken("notices.mp4", after) {
		t.Error("the replaced file kept its old token, so the page would show the previous thumbnail")
	}
	if thumbURL("notices.mp4", before) == thumbURL("notices.mp4", after) {
		t.Error("the URL did not change, so a cached browser would not refetch")
	}
}

func TestThumbnailFailureIsRememberedBriefly(t *testing.T) {
	fake := &fakeExtractor{err: os.ErrInvalid}
	tn := newTestThumbnailer(t, fake)
	src, info := mediaFile(t, t.TempDir(), "broken.mp4", "not a video")

	for range 3 {
		if _, err := tn.get(t.Context(), src, info); err == nil {
			t.Fatal("a failing extraction reported success")
		}
	}

	if got := fake.calls.Load(); got != 1 {
		t.Errorf("the extractor ran %d times, want once - a broken file should not be retried on every page load", got)
	}
}

// TestEmptyOutputCountsAsFailure: an extractor that exits cleanly having
// written nothing would otherwise leave a zero-byte JPEG to serve for ever.
func TestEmptyOutputCountsAsFailure(t *testing.T) {
	fake := &fakeExtractor{empty: true}
	tn := newTestThumbnailer(t, fake)
	src, info := mediaFile(t, t.TempDir(), "clip.mp4", "video")

	if _, err := tn.get(t.Context(), src, info); err == nil {
		t.Fatal("an empty thumbnail was reported as success")
	}

	entries, err := os.ReadDir(tn.dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".jpg" {
			t.Errorf("an empty thumbnail was left in the cache: %s", entry.Name())
		}
	}
}

// TestMissingFFmpegIsNotTheFilesFault: no marker, so the thumbnail appears as
// soon as the package lands on the device rather than an hour later.
func TestMissingFFmpegIsNotTheFilesFault(t *testing.T) {
	fake := &fakeExtractor{err: exec.ErrNotFound}
	tn := newTestThumbnailer(t, fake)
	src, info := mediaFile(t, t.TempDir(), "clip.mp4", "video")

	if _, err := tn.get(t.Context(), src, info); err == nil {
		t.Fatal("reported success with no extractor")
	}

	token := thumbToken("clip.mp4", info)
	if _, err := os.Stat(tn.failPath(token)); err == nil {
		t.Error("a missing ffmpeg marked the file as failed; it would stay iconless after ffmpeg is installed")
	}
}

// TestSlowGenerationDoesNotBlockTheRequest: the request gives up, generation
// carries on, and the next page load finds it cached.
func TestSlowGenerationDoesNotBlockTheRequest(t *testing.T) {
	fake := &fakeExtractor{delay: 2 * thumbWait, started: make(chan struct{}, 1)}
	tn := newTestThumbnailer(t, fake)
	src, info := mediaFile(t, t.TempDir(), "slow.mp4", "video")

	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	if _, err := tn.get(ctx, src, info); err == nil {
		t.Fatal("a request that timed out reported success")
	}
	if elapsed := time.Since(start); elapsed > thumbWait {
		t.Errorf("the request waited %s, past its own deadline", elapsed)
	}

	// The work should still be running rather than cancelled with the request.
	select {
	case <-fake.started:
	case <-time.After(time.Second):
		t.Fatal("generation never started")
	}
}

func TestSweepKeepsCurrentThumbnailsOnly(t *testing.T) {
	fake := &fakeExtractor{}
	tn := newTestThumbnailer(t, fake)
	dir := t.TempDir()

	keptSrc, keptInfo := mediaFile(t, dir, "keep.mp4", "video")
	goneSrc, goneInfo := mediaFile(t, dir, "gone.mp4", "video")

	if _, err := tn.get(t.Context(), keptSrc, keptInfo); err != nil {
		t.Fatal(err)
	}
	if _, err := tn.get(t.Context(), goneSrc, goneInfo); err != nil {
		t.Fatal(err)
	}

	keptToken := thumbToken("keep.mp4", keptInfo)
	goneToken := thumbToken("gone.mp4", goneInfo)

	tn.sweep(map[string]bool{keptToken: true})

	if _, err := os.Stat(tn.cachePath(keptToken)); err != nil {
		t.Error("the sweep deleted a thumbnail that is still in the playlist")
	}
	if _, err := os.Stat(tn.cachePath(goneToken)); err == nil {
		t.Error("the sweep left behind a thumbnail whose file is gone")
	}
}

func TestThumbnailsCanBeSwitchedOff(t *testing.T) {
	t.Setenv("PIPLAYER_NO_THUMBS", "1")
	if tn := newThumbnailer(t.TempDir()); tn != nil {
		t.Error("PIPLAYER_NO_THUMBS did not switch thumbnails off")
	}
}

// TestNilThumbnailerIsHarmless: a player that could not build a cache still
// serves its pages, it just has no thumbnails.
func TestNilThumbnailerIsHarmless(t *testing.T) {
	var tn *thumbnailer
	if _, err := tn.get(t.Context(), "anything.mp4", nil); err == nil {
		t.Error("a nil thumbnailer reported success")
	}
	tn.sweep(nil) // must not panic
}

// TestFFmpegFrame exercises the real binary when it is present. Skipped
// otherwise so CI never depends on it.
func TestFFmpegFrame(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "sample.mp4")
	// Generate the fixture rather than committing a binary blob.
	gen := exec.CommandContext(t.Context(), "ffmpeg", "-nostdin", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=duration=5:size=640x480:rate=10",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-y", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Skipf("could not generate a test video: %v: %s", err, out)
	}

	dst := filepath.Join(dir, "thumb.jpg")
	if err := ffmpegFrame(t.Context(), src, dst, thumbWidth); err != nil {
		t.Fatalf("extracting a frame failed: %v", err)
	}

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("no thumbnail was written: %v", err)
	}
	if info.Size() == 0 {
		t.Error("the thumbnail is empty")
	}
}

// TestFFmpegFrameRejectsRubbish: the zero-byte files the playlist tests use as
// fixtures must fail cleanly rather than producing something.
func TestFFmpegFrameRejectsRubbish(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}

	dir := t.TempDir()
	src, _ := mediaFile(t, dir, "empty.mp4", "")
	dst := filepath.Join(dir, "thumb.jpg")

	if err := ffmpegFrame(t.Context(), src, dst, thumbWidth); err == nil {
		t.Error("a zero-byte file produced a thumbnail")
	}
}

// TestThumbRouteServesAndGuards drives the handler through the real mux, so
// the route pattern, the auth guard and the path handling are all covered.
func TestThumbRouteServesAndGuards(t *testing.T) {
	mediaDir := t.TempDir()
	p := newTestPlayer(t, withMediaDir(mediaDir))
	p.thumbs = newTestThumbnailer(t, &fakeExtractor{})
	mux := setupRoutes(p)

	_, info := mediaFile(t, mediaDir, "clip.mp4", "video")
	token := thumbToken("clip.mp4", info)

	get := func(path, remoteAddr string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.RemoteAddr = remoteAddr
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)
		return recorder
	}

	const kiosk = "127.0.0.1:41000"

	// The kiosk fetches thumbnails with no session, like the rest of its
	// assets.
	rec := get("/thumb/clip.mp4?v="+token, kiosk)
	if rec.Code != http.StatusOK {
		t.Fatalf("the kiosk got %d for a thumbnail, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("Content-Type is %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Errorf("Cache-Control is %q, want it cacheable when the token matches", got)
	}

	// A stale token still serves, but must not be cached under that URL.
	rec = get("/thumb/clip.mp4?v=stale", kiosk)
	if rec.Code != http.StatusOK {
		t.Errorf("a stale token got %d, want the current thumbnail", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control for a stale token is %q, want no-store", got)
	}

	// Nothing that is not a plain name in the media directory. Some of these
	// never reach the handler at all - the mux path-cleans them into a
	// redirect - so the assertion is only that no file comes back.
	for _, path := range []string{
		"/thumb/missing.mp4",
		"/thumb/..",
		"/thumb/%2e%2e%2f%2e%2e%2fetc%2fpasswd",
		"/thumb/%2fetc%2fpasswd",
	} {
		if rec := get(path, kiosk); rec.Code == http.StatusOK {
			t.Errorf("%s was served, body %d bytes", path, rec.Body.Len())
		}
	}

	// And the same guard as the rest of the media routes off the network.
	rec = get("/thumb/clip.mp4?v="+token, "192.168.1.50:41000")
	if rec.Code != http.StatusFound {
		t.Errorf("a network request without a session got %d, want a redirect to the login page", rec.Code)
	}
}

// TestThumbRouteWithoutThumbnailer: a player that could not build a cache
// still answers, so the pages fall back to icons instead of hanging.
func TestThumbRouteWithoutThumbnailer(t *testing.T) {
	mediaDir := t.TempDir()
	p := newTestPlayer(t, withMediaDir(mediaDir))
	p.thumbs = nil
	mediaFile(t, mediaDir, "clip.mp4", "video")

	request := httptest.NewRequest(http.MethodGet, "/thumb/clip.mp4", nil)
	request.RemoteAddr = "127.0.0.1:41000"
	recorder := httptest.NewRecorder()
	setupRoutes(p).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Errorf("got %d with thumbnails disabled, want 404", recorder.Code)
	}
}
