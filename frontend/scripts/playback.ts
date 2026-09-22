// The viewer's decisions, with no DOM in them.
//
// Kept apart from the element handling so they can be tested: the half below
// is arithmetic and lookup tables, and it is where the display's behaviour
// actually lives.

import type { Item } from "./api.ts";

/** What the display should do with an item's visual file. */
export type Visual =
  | { kind: "video"; src: string }
  | { kind: "image"; src: string }
  | { kind: "unsupported"; extension: string };

/** visualFor decides how to show an item, by file extension. */
export function visualFor(fileName: string): Visual {
  const extension = fileName.slice(fileName.lastIndexOf(".")).toLowerCase();
  const src = `/content/${fileName}`;

  switch (extension) {
    case ".mp4":
    case ".webm":
      return { kind: "video", src };
    case ".jpg":
    case ".jpeg":
    case ".png":
      return { kind: "image", src };
    default:
      return { kind: "unsupported", extension };
  }
}

/** What the display should do with an item's audio track. */
export type Audio =
  | { kind: "play"; src: string }
  | { kind: "silence" };

/**
 * audioFor decides what the music track should do. A video carries its own
 * sound, and an item can ask for silence with a clear cue; an item with no
 * track at all is the ordinary case, not an error.
 */
export function audioFor(item: Item): Audio {
  if (item.Type === "video" || item.Cues?.clear === "audio" || item.Audio === "") {
    return { kind: "silence" };
  }

  const extension = item.Audio.slice(item.Audio.lastIndexOf(".")).toLowerCase();
  if (extension !== ".mp3") {
    return { kind: "silence" };
  }
  return { kind: "play", src: `/content/${item.Audio}` };
}

/** step moves through the playlist, wrapping at both ends. */
export function step(current: number, count: number, by: 1 | -1): number {
  if (count <= 0) return 0;
  return (current + by + count) % count;
}

/** cueTimeoutMs is how long an item should hold before the next one, if it says. */
export function cueTimeoutMs(item: Item): number | undefined {
  const timeout = item.Cues?.timeout;
  if (!timeout) return undefined;

  const seconds = Number.parseInt(timeout, 10);
  if (!Number.isFinite(seconds) || seconds <= 0) return undefined;
  return seconds * 1000;
}

/** trimExtension drops the file extension, the way the server's Name does. */
export function trimExtension(fileName: string): string {
  const dot = fileName.lastIndexOf(".");
  return dot < 0 ? fileName : fileName.slice(0, dot);
}

/** What a key on the remote asks the display to do. */
export type RemoteAction =
  | "focusNext"
  | "focusPrevious"
  | "previous"
  | "next"
  | "startFocused"
  | "togglePlaylist"
  | "playPause"
  | "seekForward"
  | "seekBack"
  | "reload"
  | "ignore";

// The key names are evdev's, pushed by the Go side from the USB remote. This
// is the whole of the remote's behaviour, which is why it is a table.
const REMOTE_KEYS: Record<string, RemoteAction> = {
  KEY_UP: "focusPrevious",
  KEY_DOWN: "focusNext",
  KEY_LEFT: "previous",
  KEY_PAGEUP: "previous",
  KEY_RIGHT: "next",
  KEY_PAGEDOWN: "next",
  KEY_ENTER: "startFocused",
  KEY_SELECT: "startFocused",
  KEY_CONTEXT_MENU: "togglePlaylist",
  KEY_DOT: "togglePlaylist",
  KEY_COMPOSE: "togglePlaylist",
  KEY_PLAYPAUSE: "playPause",
  KEY_FASTFORWARD: "seekForward",
  KEY_NEXTSONG: "seekForward",
  KEY_REWIND: "seekBack",
  KEY_PREVIOUSSONG: "seekBack",
  KEY_BACK: "reload",
  // Deliberately nothing: stopping the display from the remote leaves a black
  // screen with no obvious way back for whoever is holding it.
  KEY_STOP: "ignore",
};

/** remoteAction maps a key from the remote to what it does. */
export function remoteAction(keyString: string): RemoteAction | undefined {
  return REMOTE_KEYS[keyString];
}

/** seekSeconds is how far the remote's skip keys move. */
export const seekSeconds = 15;
