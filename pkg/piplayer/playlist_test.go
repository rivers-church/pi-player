package piplayer

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFiles creates each named file (empty unless content given) in dir.
func writeFiles(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte{}, 0o644); err != nil {
			t.Fatalf("failed to write fixture %q: %v", name, err)
		}
	}
}

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
