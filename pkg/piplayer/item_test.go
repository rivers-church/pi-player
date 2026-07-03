package piplayer

import (
	"io/fs"
	"os"
	"reflect"
	"testing"
	"time"
)

// Create something that satisfies the os.FileInfo interface
// just for this test.
type fi struct {
	FileName string
}

// This is the only method that we actually need,
// the other methods can return nothing, we just need to
// satisfy the interface.
func (f fi) Name() string {
	return f.FileName
}

func (fi) Size() (r int64) {
	return
}

func (fi) Mode() (r os.FileMode) {
	return
}

func (fi) ModTime() (r time.Time) {
	return
}

func (fi) IsDir() (r bool) {
	return
}

func (fi) Sys() interface{} {
	return nil
}

func (fi) Info() (fs.FileInfo, error) {
	return nil, nil
}

func (fi) Type() fs.FileMode {
	return 0
}

func TestName(t *testing.T) {
	i := Item{
		Audio:  fi{"testAudio.mp3"},
		Visual: fi{"testVideo.mp4"},
		Type:   "video",
	}

	want := ItemString{
		Audio:  "testAudio.mp3",
		Visual: "testVideo.mp4",
		Type:   "video",
	}

	got := i.String()

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestRemoveExtension(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     string
	}{
		{"normal", "video.mp4", "video"},
		{"no extension", "video", "video"},
		{"multiple dots", "my.video.file.mp4", "my.video.file"},
		// filepath.Ext treats a leading-dot name as all-extension, so the
		// whole name is stripped. Documents current behavior (dotfiles are
		// never media visuals, so fromFolder never hits this).
		{"hidden dotfile", ".gitignore", ""},
		{"empty string", "", ""},
		{"trailing dot", "video.", "video"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := removeExtension(tt.filename); got != tt.want {
				t.Errorf("removeExtension(%q) = %q, want %q", tt.filename, got, tt.want)
			}
		})
	}
}

func TestItemName(t *testing.T) {
	tests := []struct {
		name   string
		visual string
		want   string
	}{
		{"video", "clip.mp4", "clip"},
		{"image with dots", "logo.final.png", "logo.final"},
		{"no extension", "readme", "readme"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := Item{Visual: fi{tt.visual}}
			if got := i.Name(); got != tt.want {
				t.Errorf("Item.Name() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestItemString(t *testing.T) {
	cues := map[string]string{"clear": "audio"}

	tests := []struct {
		name string
		item Item
		want ItemString
	}{
		{
			name: "audio and visual",
			item: Item{Audio: fi{"song.mp3"}, Visual: fi{"clip.mp4"}, Type: "video"},
			want: ItemString{Audio: "song.mp3", Visual: "clip.mp4", Type: "video"},
		},
		{
			name: "nil audio",
			item: Item{Visual: fi{"photo.jpg"}, Type: "image"},
			want: ItemString{Audio: "", Visual: "photo.jpg", Type: "image"},
		},
		{
			name: "nil visual",
			item: Item{Audio: fi{"song.mp3"}, Type: "video"},
			want: ItemString{Audio: "song.mp3", Visual: "", Type: "video"},
		},
		{
			name: "cues propagated",
			item: Item{Visual: fi{"photo.jpg"}, Type: "image", Cues: cues},
			want: ItemString{Visual: "photo.jpg", Type: "image", Cues: cues},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.item.String(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Item.String() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
