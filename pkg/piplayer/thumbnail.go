package piplayer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// thumbWidth is the widest a thumbnail gets. Both the control table and the
	// viewer overlay draw them far smaller; this leaves room for a denser
	// layout later without regenerating the cache.
	thumbWidth = 320

	// thumbWorkers bounds how many ffmpeg processes run at once. The players
	// are low-end boxes reading multi-gigabyte files over a network share,
	// often while the display is decoding a video.
	thumbWorkers = 2

	// thumbWait is how long a request waits before giving up on a thumbnail
	// that is still being made. Generation carries on regardless, so a cold
	// page fills in over a refresh or two rather than stalling for minutes.
	thumbWait = 8 * time.Second

	// thumbGenerate caps one generation, so a share that stops answering ends
	// as a failed thumbnail instead of a stuck goroutine.
	thumbGenerate = 20 * time.Second

	// thumbSweepEvery bounds how often the cache is swept, since a scan runs
	// on every page load.
	thumbSweepEvery = 10 * time.Minute

	// thumbFailTTL is how long a file that failed is left alone. Long enough
	// not to retry a corrupt file on every page load, short enough that a
	// transient share problem heals itself.
	thumbFailTTL = time.Hour
)

// errNoThumbnail means this item has no thumbnail and the page should fall back
// to its type icon.
var errNoThumbnail = errors.New("no thumbnail available")

// frameExtractor writes a single JPEG frame of src to dst. Production uses
// ffmpeg; tests replace it so the suite never needs the binary installed.
type frameExtractor func(ctx context.Context, src, dst string, width int) error

// thumbnailer makes a small JPEG of a media file, once, and remembers it.
//
// Thumbnails are cached on local disk and never in the media directory: that is
// a share other people write to, a .jpg dropped there would be picked up as a
// playable item, and the write would trip the directory watcher into a rescan.
type thumbnailer struct {
	dir     string
	extract frameExtractor
	sem     chan struct{}

	mu        sync.Mutex
	pending   map[string]chan struct{}
	lastSweep time.Time
}

// newThumbnailer returns a thumbnailer writing into dir, or nil if the cache
// directory cannot be used - callers treat a nil thumbnailer as "this player
// does not do thumbnails", which is also what PIPLAYER_NO_THUMBS=1 gives.
func newThumbnailer(dir string) *thumbnailer {
	if os.Getenv("PIPLAYER_NO_THUMBS") == "1" {
		logger.Info("thumbnails are switched off by PIPLAYER_NO_THUMBS")
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logger.Error("could not create the thumbnail cache, falling back to icons", "dir", dir, "error", err)
		return nil
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		logger.Warn("ffmpeg is not installed, so items will show their type icon instead of a thumbnail", "error", err)
	}

	return &thumbnailer{
		dir:     dir,
		extract: ffmpegFrame,
		sem:     make(chan struct{}, thumbWorkers),
		pending: make(map[string]chan struct{}),
	}
}

// thumbToken identifies one version of one file. Size and modification time
// both change when a file is replaced under the same name, which happens here
// as often as daily, and the name keeps two different files from sharing an
// entry.
func thumbToken(name string, info fs.FileInfo) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%d\x00%d",
		name, info.Size(), info.ModTime().UnixNano(), thumbWidth)))
	return hex.EncodeToString(sum[:])[:16]
}

// thumbURL is what the pages put in an <img src>. The token rides along so the
// browser fetches a new one when the file behind it changes, which is what
// makes a long cache lifetime safe to send.
func thumbURL(name string, info fs.FileInfo) string {
	return "/thumb/" + url.PathEscape(name) + "?v=" + thumbToken(name, info)
}

func (t *thumbnailer) cachePath(token string) string {
	return filepath.Join(t.dir, token+".jpg")
}

func (t *thumbnailer) failPath(token string) string {
	return filepath.Join(t.dir, token+".fail")
}

// get returns the path of the cached thumbnail for src, generating it on first
// ask. Callers waiting on the same token share one generation.
func (t *thumbnailer) get(ctx context.Context, src string, info fs.FileInfo) (string, error) {
	if t == nil {
		return "", errNoThumbnail
	}

	token := thumbToken(filepath.Base(src), info)
	cached := t.cachePath(token)
	if _, err := os.Stat(cached); err == nil {
		return cached, nil
	}
	if t.failedRecently(token) {
		return "", errNoThumbnail
	}

	wait, leader := t.claim(token)
	if leader {
		go t.generate(token, src)
	}

	select {
	case <-wait:
		if _, err := os.Stat(cached); err != nil {
			return "", errNoThumbnail
		}
		return cached, nil
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(thumbWait):
		// Still being made. The next page load will find it cached.
		return "", context.DeadlineExceeded
	}
}

// claim joins the generation of token, or starts it. The returned channel
// closes when the generation finishes.
func (t *thumbnailer) claim(token string) (<-chan struct{}, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if wait, ok := t.pending[token]; ok {
		return wait, false
	}
	wait := make(chan struct{})
	t.pending[token] = wait
	return wait, true
}

// generate runs one extraction. It deliberately does not take the request's
// context: an operator navigating away should not kill work other tabs are
// waiting on.
func (t *thumbnailer) generate(token, src string) {
	defer func() {
		t.mu.Lock()
		wait := t.pending[token]
		delete(t.pending, token)
		t.mu.Unlock()
		close(wait)
	}()

	t.sem <- struct{}{}
	defer func() { <-t.sem }()

	ctx, cancel := context.WithTimeout(context.Background(), thumbGenerate)
	defer cancel()

	// Write to a temp file and rename into place, so a killed ffmpeg cannot
	// leave a truncated JPEG behind to be served as valid for ever.
	tmp, err := os.CreateTemp(t.dir, "tmp-*.jpg")
	if err != nil {
		logger.Error("could not create a temporary thumbnail file", "error", err)
		return
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	if err := t.extract(ctx, src, tmp.Name(), thumbWidth); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			// The machine's problem, not the file's: an ansible run fixes it,
			// and marking the file failed would outlast the fix.
			logger.Debug("no ffmpeg, so no thumbnail", "file", src)
			return
		}
		logger.Debug("could not make a thumbnail", "file", src, "error", err)
		t.markFailed(token)
		return
	}

	// An extractor that exits cleanly having written nothing is still a
	// failure; serving a zero-byte JPEG would just be a broken image.
	if info, err := os.Stat(tmp.Name()); err != nil || info.Size() == 0 {
		logger.Debug("the thumbnail came out empty", "file", src)
		t.markFailed(token)
		return
	}

	if err := os.Rename(tmp.Name(), t.cachePath(token)); err != nil {
		logger.Error("could not store the thumbnail", "file", src, "error", err)
	}
}

func (t *thumbnailer) markFailed(token string) {
	if err := os.WriteFile(t.failPath(token), nil, 0o644); err != nil {
		logger.Debug("could not record a failed thumbnail", "error", err)
	}
}

// failedRecently reports whether this exact version of the file has already
// failed, so a corrupt file is not retried on every page load.
func (t *thumbnailer) failedRecently(token string) bool {
	info, err := os.Stat(t.failPath(token))
	if err != nil {
		return false
	}
	if time.Since(info.ModTime()) > thumbFailTTL {
		os.Remove(t.failPath(token))
		return false
	}
	return true
}

// sweep deletes every cache entry that is not in keep. Media here changes
// often, so without this the cache grows by one entry per replacement for ever.
func (t *thumbnailer) sweep(keep map[string]bool) {
	if t == nil {
		return
	}

	entries, err := os.ReadDir(t.dir)
	if err != nil {
		logger.Debug("could not read the thumbnail cache to sweep it", "error", err)
		return
	}

	removed := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		token := name[:len(name)-len(filepath.Ext(name))]
		if keep[token] {
			continue
		}
		// Leave temporary files alone; a generation may be in flight.
		if len(name) > 4 && name[:4] == "tmp-" {
			continue
		}
		if err := os.Remove(filepath.Join(t.dir, name)); err == nil {
			removed++
		}
	}
	if removed > 0 {
		logger.Debug("swept the thumbnail cache", "removed", removed)
	}
}

// ffmpegFrame pulls one frame out of a media file. It works for images as well
// as video, which keeps this to a single code path.
func ffmpegFrame(ctx context.Context, src, dst string, width int) error {
	scale := "scale=" + strconv.Itoa(width) + ":-2"

	// Seeking before -i lands on the nearest keyframe rather than decoding
	// from the start, which is the difference between milliseconds and
	// minutes on a long video.
	err := runFFmpeg(ctx, "3", src, dst, scale)
	if err == nil {
		return nil
	}
	if errors.Is(err, exec.ErrNotFound) || ctx.Err() != nil {
		return err
	}

	// Anything shorter than the seek point yields no frame.
	return runFFmpeg(ctx, "0", src, dst, scale)
}

func runFFmpeg(ctx context.Context, seek, src, dst, scale string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-nostdin", "-loglevel", "error",
		"-ss", seek, "-i", src,
		"-frames:v", "1", "-vf", scale, "-q:v", "4",
		"-y", dst)
	// Give it a moment to die politely before the pipes are torn out from
	// under it.
	cmd.WaitDelay = 3 * time.Second

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg: %w: %s", err, out)
	}
	return nil
}

// handleThumb serves a media file's thumbnail, making it on first request.
//
// The name comes from the URL path and is never used to build a cache path -
// only the hex token derived from it is - so nothing an operator can name
// reaches the filesystem beyond the media directory itself.
func (p *Player) handleThumb(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" || name != filepath.Base(name) || name == "." || name == ".." {
		http.Error(w, "Not found.", http.StatusNotFound)
		return
	}

	src := filepath.Join(p.conf.mediaDir(), name)
	info, err := os.Stat(src)
	if err != nil || info.IsDir() {
		http.Error(w, "Not found.", http.StatusNotFound)
		return
	}

	cached, err := p.thumbs.get(r.Context(), src, info)
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		// Still being made. Say so rather than holding the request open; the
		// page will have it next time.
		w.Header().Set("Retry-After", "5")
		http.Error(w, "Thumbnail is being generated.", http.StatusServiceUnavailable)
		return
	case err != nil:
		http.Error(w, "No thumbnail.", http.StatusNotFound)
		return
	}

	// The URL carries a token that changes with the file, so this can be
	// cached hard. The kiosk browser has no cache at all; this is for the
	// control page on a laptop or a phone.
	if r.URL.Query().Get("v") == thumbToken(name, info) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		// The file moved on since the page was rendered. Serve what we have,
		// but do not let the stale URL stick.
		w.Header().Set("Cache-Control", "no-store")
	}
	w.Header().Set("Content-Type", "image/jpeg")
	http.ServeFile(w, r, cached)
}

// sweepAfterScan drops cache entries that the current playlist no longer
// refers to. Media here is replaced often, so every swap orphans an entry.
//
// Rate limited because a scan happens on every control and viewer page load,
// and guarded on a non-empty scan so a share that briefly went away does not
// take the whole cache with it.
func (t *thumbnailer) sweepAfterScan(items []Item) {
	if t == nil || len(items) == 0 {
		return
	}

	t.mu.Lock()
	if time.Since(t.lastSweep) < thumbSweepEvery {
		t.mu.Unlock()
		return
	}
	t.lastSweep = time.Now()
	t.mu.Unlock()

	keep := make(map[string]bool, len(items))
	for _, item := range items {
		if token := tokenFromURL(item.thumb); token != "" {
			keep[token] = true
		}
	}

	go t.sweep(keep)
}

// tokenFromURL pulls the token back out of a thumbnail URL, which is where it
// was already computed during the scan.
func tokenFromURL(thumb string) string {
	_, token, found := strings.Cut(thumb, "?v=")
	if !found {
		return ""
	}
	return token
}
