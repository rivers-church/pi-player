// The kiosk display.
//
// Deliberately free of the component library: this page is a fullscreen media
// surface with no chrome, and the display should not be able to break because
// a component library changed. Everything it decides lives in playback.ts,
// which has no DOM in it and is tested; this half is the element handling.

import { getItems, setCurrent, type Item } from "./api.ts";
import { ReconnectingSocket, type WsMessage } from "./socket.ts";
import {
  audioFor,
  cueTimeoutMs,
  remoteAction,
  seekSeconds,
  step,
  trimExtension,
  visualFor,
  type RemoteAction,
} from "./playback.ts";

/** How long the cursor stays visible after the mouse moves. */
const cursorHideDelay = 1000;

class Viewer {
  readonly #container: HTMLElement;
  readonly #playlistPanel: HTMLElement;
  readonly #table: HTMLTableElement;
  readonly #rowTemplate: HTMLTemplateElement;
  readonly #video: HTMLVideoElement;
  readonly #audio: HTMLAudioElement;

  #items: Item[] = [];
  #rows: HTMLElement[] = [];
  #current = 0;
  #cueTimer: ReturnType<typeof setTimeout> | undefined;
  #cursorTimer: ReturnType<typeof setTimeout> | undefined;

  constructor(root: Document) {
    this.#container = root.querySelector("#container")!;
    this.#playlistPanel = root.querySelector("#containerPlaylist")!;
    this.#table = root.querySelector("#tblPlaylist")!;
    this.#rowTemplate = root.querySelector("#tmpItemRow")!;
    this.#video = root.querySelector("#vidMedia")!;
    this.#audio = root.querySelector("#audMusic")!;
  }

  async start(): Promise<void> {
    this.#video.addEventListener("ended", () => this.next());

    // Nobody is sitting at this machine, so local input can only get the
    // display into a state no one is there to fix.
    document.addEventListener("keydown", (event) => event.preventDefault());
    this.#watchMouse();

    await this.loadItems();
    this.startItem(0);

    new ReconnectingSocket({
      path: "/ws/viewer",
      onMessage: (msg) => this.handleMessage(msg),
    }).connect();
  }

  async loadItems(): Promise<void> {
    const res = await getItems();
    if (!res.success) {
      console.error("could not load the playlist:", res);
      return;
    }
    this.#items = res.message as Item[];
    this.#renderItems();
  }

  handleMessage(msg: WsMessage): void {
    switch (msg.component) {
      case "remote":
        this.#handleRemote(msg.arguments?.keyString ?? "");
        break;
      case "player":
        this.#handlePlayer(msg);
        break;
      case "playlist":
        if (msg.event === "newItems") this.loadItems();
        break;
      case "connection":
        console.warn("the player closed this connection:", msg.message);
        break;
      default:
        console.error("unsupported message:", msg);
    }
  }

  #handleRemote(keyString: string): void {
    const action = remoteAction(keyString);
    if (action === undefined) {
      console.log("unsupported remote key:", keyString);
      return;
    }
    this.#run(action);
  }

  #run(action: RemoteAction): void {
    switch (action) {
      case "focusNext":
        this.#moveFocus(1);
        break;
      case "focusPrevious":
        this.#moveFocus(-1);
        break;
      case "previous":
        this.previous();
        break;
      case "next":
        this.next();
        break;
      case "startFocused":
        this.#startFocused();
        break;
      case "togglePlaylist":
        this.#togglePlaylist();
        break;
      case "playPause":
        this.playPause();
        break;
      case "seekForward":
        this.seek(seekSeconds);
        break;
      case "seekBack":
        this.seek(-seekSeconds);
        break;
      case "reload":
        this.loadItems();
        break;
      case "ignore":
        break;
    }
  }

  #handlePlayer(msg: WsMessage): void {
    switch (msg.method) {
      case "start":
        this.startItem(Number(msg.message));
        break;
      case "stop":
        this.stop();
        break;
      case "play":
      case "pause":
        this.playPause();
        break;
      case "seek":
        this.seek(Number(msg.arguments?.value ?? 0));
        break;
      case "previous":
        this.previous();
        break;
      case "next":
        this.next();
        break;
      default:
        console.error("unsupported player method:", msg.method);
    }
  }

  next(): void {
    this.startItem(step(this.#current, this.#items.length, 1));
  }

  previous(): void {
    this.startItem(step(this.#current, this.#items.length, -1));
  }

  /** startItem puts item at index on the screen. */
  startItem(index: number): void {
    const item = this.#items[index];
    if (!item) return;

    clearTimeout(this.#cueTimer);
    this.#hidePlaylist();

    this.#showAudio(item);
    if (!this.#showVisual(item)) return;

    this.#current = index;

    const timeout = cueTimeoutMs(item);
    if (timeout !== undefined) {
      this.#cueTimer = setTimeout(() => this.next(), timeout);
    }

    setCurrent(index).then((res) => {
      if (!res.success) console.error("could not set the current item:", res);
    });
  }

  /** showVisual returns false when the file is not something we can display. */
  #showVisual(item: Item): boolean {
    const visual = visualFor(item.Visual);

    switch (visual.kind) {
      case "video":
        this.#container.style.backgroundImage = "";
        this.#video.src = visual.src;
        this.#video.style.visibility = "visible";
        // An autoplay refusal is the browser's call and not worth a crash.
        this.#video.play().catch((err) => console.error("could not play the video:", err));
        return true;
      case "image":
        this.#video.pause();
        this.#video.style.visibility = "hidden";
        this.#container.style.backgroundImage = `url("${visual.src}")`;
        return true;
      case "unsupported":
        console.log("file type not supported:", item.Visual, visual.extension);
        return false;
    }
  }

  #showAudio(item: Item): void {
    const audio = audioFor(item);
    if (audio.kind === "silence") {
      this.#audio.pause();
      this.#audio.src = "";
      return;
    }
    this.#audio.src = audio.src;
    this.#audio.play().catch((err) => console.error("could not play the audio:", err));
  }

  /**
   * playPause moves the video and its music together. They used to toggle
   * independently, which let them drift apart.
   */
  playPause(): void {
    const item = this.#items[this.#current];
    if (!item) return;

    const playing = !this.#video.paused || !this.#audio.paused;
    for (const media of [this.#video, this.#audio]) {
      if (!media.src) continue;
      if (playing) {
        media.pause();
      } else {
        media.play().catch((err) => console.error("could not resume:", err));
      }
    }
  }

  stop(): void {
    clearTimeout(this.#cueTimer);
    for (const media of [this.#video, this.#audio]) {
      media.pause();
      media.currentTime = 0;
    }
    this.#video.style.visibility = "hidden";
    this.#container.style.backgroundImage = "";
  }

  seek(seconds: number): void {
    if (!this.#video.src) return;
    this.#video.currentTime += seconds;
  }

  // -- the playlist overlay -------------------------------------------------

  #renderItems(): void {
    this.#table.innerHTML = "";

    for (const [index, item] of this.#items.entries()) {
      const row = this.#rowTemplate.content.cloneNode(true) as DocumentFragment;
      const tr = row.querySelector("tr")!;
      tr.dataset.index = index.toString();

      // The icons are CSS masks rather than a library: the display carries no
      // component framework, and an <img> of a currentColor SVG would render
      // black on black.
      const icons = row.querySelectorAll<HTMLElement>("td.icon .icon-img");
      icons[0].dataset.icon = item.Type === "browser" ? "window-maximize" : item.Type;
      if (item.Cues?.clear === "audio") {
        icons[1].dataset.icon = "bell-slash";
      } else if (item.Audio !== "") {
        icons[1].dataset.icon = "music";
      } else {
        icons[1].remove();
      }

      row.querySelector(".itemName")!.textContent = trimExtension(item.Visual);
      this.#table.appendChild(row);
    }

    this.#rows = Array.from(this.#table.querySelectorAll<HTMLElement>(".item"));
  }

  /** Focus is the selection: the rows carry tabindex and :focus styles them. */
  #moveFocus(by: 1 | -1): void {
    if (this.#rows.length === 0) return;

    const focused = this.#rows.findIndex((row) => row === document.activeElement);
    if (focused < 0) {
      this.#rows[0].focus();
      return;
    }
    this.#rows[step(focused, this.#rows.length, by)].focus();
  }

  #startFocused(): void {
    const focused = document.activeElement as HTMLElement | null;
    const index = focused?.dataset?.index;
    if (index === undefined) return;
    this.startItem(Number(index));
  }

  #togglePlaylist(): void {
    const hidden = this.#playlistPanel.style.visibility === "hidden" ||
      this.#playlistPanel.style.visibility === "";
    this.#playlistPanel.style.visibility = hidden ? "visible" : "hidden";
    if (hidden) this.#rows[this.#current]?.focus();
  }

  #hidePlaylist(): void {
    this.#playlistPanel.style.visibility = "hidden";
  }

  // -- the mouse ------------------------------------------------------------

  /** The display has no mouse; if one moves, show it briefly and hide it again. */
  #watchMouse(): void {
    document.documentElement.style.cursor = "none";
    document.addEventListener("mousemove", () => {
      document.documentElement.style.cursor = "";
      clearTimeout(this.#cursorTimer);
      this.#cursorTimer = setTimeout(() => {
        document.documentElement.style.cursor = "none";
      }, cursorHideDelay);
    });
  }
}

if (!globalThis.WebSocket) {
  console.error("This page requires WebSocket support.");
} else {
  new Viewer(document).start();
}
