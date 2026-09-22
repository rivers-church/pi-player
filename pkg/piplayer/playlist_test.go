package piplayer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
)

// byVisual indexes a playlist's items by their visual filename (with extension).
func byVisual(p *Playlist) map[string]Item {
	m := make(map[string]Item, len(p.Items))
	for _, it := range p.Items {
		if it.Visual != nil {
			m[it.Visual.Name()] = it
		}
	}
	return m
}

func TestFromFolderEmptyDir(t *testing.T) {
	p := &Playlist{}
	if err := p.fromFolder(t.TempDir()); err != nil {
		t.Fatalf("fromFolder() on empty dir returned error: %v", err)
	}
	if len(p.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(p.Items))
	}
}

func TestFromFolderNonExistentDir(t *testing.T) {
	p := &Playlist{}
	err := p.fromFolder(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("fromFolder() on missing dir returned nil error, want error")
	}
}

func TestFromFolderClassification(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir,
		"movie.mp4",
		"clip.webm",
		"photo.jpg",
		"pic.jpeg",
		"logo.png",
		"page.html",
		"UPPER.MP4", // case-insensitive extension match
		"notes.txt", // unsupported, ignored
		"archive.zip",
	)

	p := &Playlist{}
	if err := p.fromFolder(dir); err != nil {
		t.Fatalf("fromFolder() returned error: %v", err)
	}

	want := map[string]string{
		"movie.mp4": "video",
		"clip.webm": "video",
		"photo.jpg": "image",
		"pic.jpeg":  "image",
		"logo.png":  "image",
		"page.html": "browser",
		"UPPER.MP4": "video",
	}

	if len(p.Items) != len(want) {
		t.Errorf("expected %d items, got %d", len(want), len(p.Items))
	}

	items := byVisual(p)
	for name, wantType := range want {
		it, ok := items[name]
		if !ok {
			t.Errorf("expected item for %q, not found", name)
			continue
		}
		if it.Type != wantType {
			t.Errorf("item %q: got type %q, want %q", name, it.Type, wantType)
		}
	}

	for _, ignored := range []string{"notes.txt", "archive.zip"} {
		if _, ok := items[ignored]; ok {
			t.Errorf("unsupported file %q should have been ignored", ignored)
		}
	}
}

func TestFromFolderAudioPairing(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir,
		"song.jpg", // visual that gets an .mp3 audio attached
		"song.mp3",
		"muted.png", // visual with an .mp0 marker
		"muted.mp0",
		"orphan.mp3", // no matching visual
	)

	p := &Playlist{}
	if err := p.fromFolder(dir); err != nil {
		t.Fatalf("fromFolder() returned error: %v", err)
	}

	items := byVisual(p)

	song, ok := items["song.jpg"]
	if !ok {
		t.Fatal("expected item for song.jpg")
	}
	if song.Audio == nil {
		t.Error("song.jpg: expected .mp3 audio attached, got nil")
	} else if song.Audio.Name() != "song.mp3" {
		t.Errorf("song.jpg: audio = %q, want song.mp3", song.Audio.Name())
	}

	muted, ok := items["muted.png"]
	if !ok {
		t.Fatal("expected item for muted.png")
	}
	if got := muted.Cues["clear"]; got != "audio" {
		t.Errorf("muted.png: Cues[clear] = %q, want audio", got)
	}

	// Orphan audio should not create a visual item.
	if len(p.Items) != 2 {
		t.Errorf("expected 2 visual items, got %d", len(p.Items))
	}
}

func TestFromFolderPresentationCues(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "photo.jpg", "other.png")

	presentation := `{
		"Items": [
			{"Visual": "photo", "Cues": {"duration": "5", "loop": "true"}}
		]
	}`
	if err := os.WriteFile(filepath.Join(dir, "presentation.json"), []byte(presentation), 0o644); err != nil {
		t.Fatalf("failed to write presentation.json: %v", err)
	}

	p := &Playlist{}
	if err := p.fromFolder(dir); err != nil {
		t.Fatalf("fromFolder() returned error: %v", err)
	}

	items := byVisual(p)
	photo, ok := items["photo.jpg"]
	if !ok {
		t.Fatal("expected item for photo.jpg")
	}
	if photo.Cues["duration"] != "5" || photo.Cues["loop"] != "true" {
		t.Errorf("photo.jpg: cues not attached, got %v", photo.Cues)
	}

	// The non-matching item should have no presentation cues.
	other, ok := items["other.png"]
	if !ok {
		t.Fatal("expected item for other.png")
	}
	if len(other.Cues) != 0 {
		t.Errorf("other.png: expected no cues, got %v", other.Cues)
	}
}

func TestItemsString(t *testing.T) {
	p := &Playlist{
		Items: []Item{
			{Visual: fi{"clip.mp4"}, Audio: fi{"clip.mp3"}, Type: "video", Cues: map[string]string{}},
			{Visual: fi{"photo.jpg"}, Type: "image", Cues: map[string]string{}},
		},
	}

	got := p.itemsString()
	if len(got) != 2 {
		t.Fatalf("expected 2 ItemStrings, got %d", len(got))
	}
	if got[0].Visual != "clip.mp4" || got[0].Audio != "clip.mp3" || got[0].Type != "video" {
		t.Errorf("unexpected first ItemString: %+v", got[0])
	}
	if got[1].Visual != "photo.jpg" || got[1].Audio != "" || got[1].Type != "image" {
		t.Errorf("unexpected second ItemString: %+v", got[1])
	}
}

// TestFromFolderConcurrentWithReaders rescans the playlist while other
// goroutines read it, the way /control, /viewer and the getItems API call do.
// Without the mutex this reports a data race, and a template ranging over a
// half-rebuilt slice can panic.
func TestFromFolderConcurrentWithReaders(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"one.mp4", "two.jpg", "three.png"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("writing fixture %s failed: %v", name, err)
		}
	}

	p := &Playlist{}
	if err := p.fromFolder(dir); err != nil {
		t.Fatalf("initial scan failed: %v", err)
	}
	p.setCurrent(0)

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				if err := p.fromFolder(dir); err != nil {
					t.Errorf("rescan failed: %v", err)
					return
				}
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				view := p.snapshot()
				for i := range view.Items {
					_ = view.Items[i].Name()
				}
				if view.Current != nil {
					_ = view.Current.Name()
				}
				_ = p.itemsString()
			}
		}()
	}
	wg.Wait()
}

// TestFromFolderKeepsCurrentAcrossRescan guards against Current being left
// pointing into the slice that the previous scan abandoned.
func TestFromFolderKeepsCurrentAcrossRescan(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"one.mp4", "two.jpg"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("writing fixture %s failed: %v", name, err)
		}
	}

	p := &Playlist{}
	if err := p.fromFolder(dir); err != nil {
		t.Fatalf("initial scan failed: %v", err)
	}
	if !p.setCurrent(1) {
		t.Fatal("setCurrent(1) reported the index was out of range")
	}
	want, _ := p.currentName()

	if err := p.fromFolder(dir); err != nil {
		t.Fatalf("rescan failed: %v", err)
	}

	got, ok := p.currentName()
	if !ok {
		t.Fatal("the current item was lost by the rescan")
	}
	if got != want {
		t.Errorf("current item is %q after the rescan, want %q", got, want)
	}
	if p.Current != &p.Items[1] {
		t.Error("current item does not point into the rebuilt item slice")
	}
}

// TestSetCurrentRejectsOutOfRange documents that an index the browser made up
// is refused rather than panicking.
func TestSetCurrentRejectsOutOfRange(t *testing.T) {
	p := &Playlist{}
	for _, index := range []int{-1, 0, 5} {
		if p.setCurrent(index) {
			t.Errorf("setCurrent(%d) was accepted on an empty playlist", index)
		}
	}
}

// TestGetItemsFollowsConfiguredDir checks the API rescans the directory the
// config points at. It used to rescan whichever directory the playlist was
// last read from, so after a media-directory change the viewer could be handed
// items from the old one until some page load happened to resync them.
func TestGetItemsFollowsConfiguredDir(t *testing.T) {
	oldDir := t.TempDir()
	newDir := t.TempDir()
	writeFiles(t, oldDir, "old.mp4")
	writeFiles(t, newDir, "new.mp4")

	p := newTestPlayer(t, withMediaDir(oldDir))
	if err := p.playlist.fromFolder(oldDir); err != nil {
		t.Fatalf("initial scan failed: %v", err)
	}

	// Change the media directory the way the settings page does, with no page
	// load in between.
	p.conf.SetMount(mount{Dir: newDir})

	recorder := httptest.NewRecorder()
	p.playlist.handleAPI(reqMessage{Component: "playlist", Method: "getItems"}, recorder, p.conf.MediaDir(), p.ConnControl)

	var res resMessage
	if err := json.NewDecoder(recorder.Body).Decode(&res); err != nil {
		t.Fatalf("decoding the response failed: %v", err)
	}
	items, ok := res.Message.([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("got %v, want one item", res.Message)
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected item shape: %v", items[0])
	}
	if item["Visual"] != "new.mp4" {
		t.Errorf("getItems returned %v, want the item from the newly configured directory", item["Visual"])
	}
}

// fakeNotifier records what the playlist pushes to the control page.
type fakeNotifier struct {
	mu   sync.Mutex
	sent []wsMessage
}

func (f *fakeNotifier) trySend(msg wsMessage) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, msg)
	return true
}

func (f *fakeNotifier) messages() []wsMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.sent)
}

// TestSetCurrentNotifiesControlPage exercises the playlist API with a stand-in
// for the websocket, which is the point of the handler taking a notifier
// rather than the whole player.
func TestSetCurrentNotifiesControlPage(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "one.mp4", "two.mp4")

	pl := &Playlist{}
	if err := pl.fromFolder(dir); err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	control := &fakeNotifier{}

	recorder := httptest.NewRecorder()
	msg := reqMessage{Component: "playlist", Method: "setCurrent", Arguments: map[string]string{"index": "1"}}
	pl.handleAPI(msg, recorder, dir, control)

	if recorder.Code != http.StatusOK {
		t.Fatalf("setCurrent returned status %d", recorder.Code)
	}
	sent := control.messages()
	if len(sent) != 1 {
		t.Fatalf("the control page got %d messages, want 1", len(sent))
	}
	if sent[0].Event != "setCurrent" {
		t.Errorf("control page got event %q, want %q", sent[0].Event, "setCurrent")
	}

	name, ok := pl.currentName()
	if !ok || name != "two" {
		t.Errorf("current item is %q (set: %v), want %q", name, ok, "two")
	}
}

// TestSetCurrentOutOfRangeTellsNobody checks a bad index doesn't reach the page.
func TestSetCurrentOutOfRangeTellsNobody(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "one.mp4")

	pl := &Playlist{}
	if err := pl.fromFolder(dir); err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	control := &fakeNotifier{}

	recorder := httptest.NewRecorder()
	msg := reqMessage{Component: "playlist", Method: "setCurrent", Arguments: map[string]string{"index": "7"}}
	pl.handleAPI(msg, recorder, dir, control)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("an out-of-range index returned status %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if got := control.messages(); len(got) != 0 {
		t.Errorf("the control page was told about a rejected index: %v", got)
	}
}
