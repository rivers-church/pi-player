package piplayer

import (
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
