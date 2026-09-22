import { assertEquals } from "@std/assert";
import {
  audioFor,
  cueTimeoutMs,
  remoteAction,
  step,
  thumbFor,
  trimExtension,
  visualFor,
} from "./playback.ts";
import type { Item } from "./api.ts";

function item(overrides: Partial<Item> = {}): Item {
  return { Audio: "", Visual: "clip.mp4", Type: "video", Cues: {}, ...overrides };
}

Deno.test("visualFor dispatches on the file extension", () => {
  assertEquals(visualFor("clip.mp4"), { kind: "video", src: "/content/clip.mp4" });
  assertEquals(visualFor("clip.webm"), { kind: "video", src: "/content/clip.webm" });
  assertEquals(visualFor("photo.jpg"), { kind: "image", src: "/content/photo.jpg" });
  assertEquals(visualFor("photo.jpeg"), { kind: "image", src: "/content/photo.jpeg" });
  assertEquals(visualFor("photo.png"), { kind: "image", src: "/content/photo.png" });
});

Deno.test("visualFor is case insensitive", () => {
  // A file off a Windows share is as likely to be .JPG as .jpg, and the old
  // code compared the extension literally.
  assertEquals(visualFor("PHOTO.JPG").kind, "image");
  assertEquals(visualFor("CLIP.MP4").kind, "video");
});

Deno.test("visualFor refuses anything else", () => {
  assertEquals(visualFor("notes.pdf"), { kind: "unsupported", extension: ".pdf" });
});

Deno.test("audioFor silences a video, a clear cue, and an item with no track", () => {
  assertEquals(audioFor(item({ Type: "video", Audio: "song.mp3" })), { kind: "silence" });
  assertEquals(
    audioFor(item({ Type: "image", Audio: "song.mp3", Cues: { clear: "audio" } })),
    { kind: "silence" },
  );
  // An image with no music is the ordinary case. The old code logged it as an
  // unsupported file type on every single item change.
  assertEquals(audioFor(item({ Type: "image", Audio: "" })), { kind: "silence" });
});

Deno.test("audioFor plays an mp3 beside an image", () => {
  assertEquals(
    audioFor(item({ Type: "image", Visual: "photo.png", Audio: "song.mp3" })),
    { kind: "play", src: "/content/song.mp3" },
  );
});

Deno.test("step wraps at both ends", () => {
  assertEquals(step(0, 3, 1), 1);
  assertEquals(step(2, 3, 1), 0);
  assertEquals(step(0, 3, -1), 2);
  assertEquals(step(1, 3, -1), 0);
});

Deno.test("step survives an empty playlist", () => {
  // A media directory can be emptied while the display is running.
  assertEquals(step(0, 0, 1), 0);
  assertEquals(step(0, 0, -1), 0);
});

Deno.test("cueTimeoutMs reads the timeout cue in seconds", () => {
  assertEquals(cueTimeoutMs(item({ Cues: { timeout: "10" } })), 10000);
  assertEquals(cueTimeoutMs(item({ Cues: {} })), undefined);
  // A typo in a hand-written presentation.json should not schedule an
  // immediate or a NaN jump to the next item.
  assertEquals(cueTimeoutMs(item({ Cues: { timeout: "soon" } })), undefined);
  assertEquals(cueTimeoutMs(item({ Cues: { timeout: "0" } })), undefined);
  assertEquals(cueTimeoutMs(item({ Cues: { timeout: "-5" } })), undefined);
});

Deno.test("trimExtension matches the name the server renders", () => {
  assertEquals(trimExtension("PiPlayer Logo.png"), "PiPlayer Logo");
  assertEquals(trimExtension("no-extension"), "no-extension");
});

Deno.test("the remote keymap covers every key the player sends", () => {
  // These names come from remote.go; if one is dropped here the key silently
  // stops working, and there is nobody at the device to notice.
  assertEquals(remoteAction("KEY_UP"), "focusPrevious");
  assertEquals(remoteAction("KEY_DOWN"), "focusNext");
  assertEquals(remoteAction("KEY_LEFT"), "previous");
  assertEquals(remoteAction("KEY_PAGEUP"), "previous");
  assertEquals(remoteAction("KEY_RIGHT"), "next");
  assertEquals(remoteAction("KEY_PAGEDOWN"), "next");
  assertEquals(remoteAction("KEY_ENTER"), "startFocused");
  assertEquals(remoteAction("KEY_SELECT"), "startFocused");
  assertEquals(remoteAction("KEY_CONTEXT_MENU"), "togglePlaylist");
  assertEquals(remoteAction("KEY_DOT"), "togglePlaylist");
  assertEquals(remoteAction("KEY_COMPOSE"), "togglePlaylist");
  assertEquals(remoteAction("KEY_PLAYPAUSE"), "playPause");
  assertEquals(remoteAction("KEY_FASTFORWARD"), "seekForward");
  assertEquals(remoteAction("KEY_NEXTSONG"), "seekForward");
  assertEquals(remoteAction("KEY_REWIND"), "seekBack");
  assertEquals(remoteAction("KEY_PREVIOUSSONG"), "seekBack");
  assertEquals(remoteAction("KEY_BACK"), "reload");
  assertEquals(remoteAction("KEY_STOP"), "ignore");
  assertEquals(remoteAction("KEY_WHATEVER"), undefined);
});

Deno.test("thumbFor copes with items that have no thumbnail", () => {
  assertEquals(thumbFor(item({ Thumb: "/thumb/clip.mp4?v=abc" })), "/thumb/clip.mp4?v=abc");
  // A page is not something a frame can be pulled out of, so the server sends
  // nothing and the row keeps its type icon.
  assertEquals(thumbFor(item({ Thumb: "" })), undefined);
  // A player older than thumbnails does not send the field at all.
  assertEquals(thumbFor(item()), undefined);
});
