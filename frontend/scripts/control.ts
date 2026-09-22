// The operator's remote control.
//
// Two directions: commands go out over the JSON API, and the player pushes
// state back over the websocket - which is why this page needs a script at all
// and can use custom elements freely.

import type WaDialog from "@awesome.me/webawesome/dist/components/dialog/dialog.js";

import { callApi, getItems, type ApiRequest, type Item } from "./api.ts";
import { ReconnectingSocket, type WsMessage } from "./socket.ts";

export class Control {
  readonly #transport: NodeListOf<HTMLElement>;
  readonly #btnStart: HTMLElement;
  readonly #spCurrent: HTMLElement;
  readonly #tblPlaylist: HTMLTableElement;
  readonly #dlgReconnect: WaDialog;
  readonly #dlgDisconnect: WaDialog;

  #conn: ReconnectingSocket | undefined;
  #items: Item[] = [];
  #selected: HTMLTableRowElement | undefined;
  /** Set when the server says another device has taken the connection. */
  #displaced = false;

  constructor(root: Document) {
    this.#transport = root.querySelectorAll("#divControlsPlayer wa-button");
    this.#btnStart = root.querySelector("#btnStart")!;
    this.#spCurrent = root.querySelector("#spCurrent")!;
    this.#tblPlaylist = root.querySelector("#tblPlaylist")!;
    this.#dlgReconnect = root.querySelector("#dlgReconnect")!;
    this.#dlgDisconnect = root.querySelector("#dlgDisconnect")!;
  }

  start(): void {
    this.loadItems();
    this.connect();
    this.#watchThumbnails();

    this.#tblPlaylist.addEventListener("click", (event) => this.select(event));
    this.#btnStart.addEventListener("click", () => this.startSelected());
    for (const button of this.#transport) {
      button.addEventListener("click", () => this.sendCommand(button));
    }
  }

  /**
   * A thumbnail that fails to load is removed, leaving the type icon in the
   * next column. Listened for in capture phase because an <img> error does
   * not bubble, and attached here rather than inline because the content
   * security policy forbids inline handlers.
   */
  #watchThumbnails(): void {
    this.#tblPlaylist.addEventListener("error", (event) => {
      const target = event.target as HTMLElement;
      if (target instanceof HTMLImageElement) target.remove();
    }, true);
  }

  async loadItems(): Promise<void> {
    const res = await getItems();
    if (!res.success) {
      console.error("could not load the playlist:", res);
      return;
    }
    this.#items = res.message as Item[];
  }

  connect(): void {
    this.#conn = new ReconnectingSocket({
      path: "/ws/control",
      onOpen: () => this.#dlgReconnect.open = false,
      onClose: () => {
        if (this.#displaced) {
          // Another device has it; reconnecting would only take it back off
          // them and leave the two pages fighting.
          this.#conn?.stop();
          this.#dlgReconnect.open = false;
          this.#dlgDisconnect.open = true;
          return;
        }
        this.#dlgReconnect.open = true;
      },
      onMessage: (msg) => this.handleMessage(msg),
    });
    this.#conn.connect();
  }

  handleMessage(msg: WsMessage): void {
    switch (msg.event) {
      case "setCurrent":
        this.showCurrent(Number(msg.message));
        break;
      case "disconnect":
        this.#displaced = true;
        console.warn("the player closed this connection:", msg.message);
        break;
      default:
        console.log("unsupported message received:", msg);
    }
  }

  /** select moves the highlight to the clicked row. */
  select(event: Event): void {
    const row = (event.target as HTMLElement).closest("tr");
    if (!row) return;

    this.#selected?.classList.remove("selected");
    this.#selected = row;
    row.classList.add("selected");
  }

  /** showCurrent reflects what the display says it is playing. */
  showCurrent(index: number): void {
    const item = this.#items[index];
    if (!item) return;

    this.#spCurrent.textContent = trimExtension(item.Visual);

    const row = this.#tblPlaylist.querySelector<HTMLTableRowElement>(`tr[data-index="${index}"]`);
    if (row) {
      this.#selected?.classList.remove("selected");
      this.#selected = row;
      row.classList.add("selected");
    }
  }

  async startSelected(): Promise<void> {
    if (!this.#selected) {
      console.log("nothing is selected to start");
      return;
    }

    const index = this.#selected.dataset.index ?? "";
    const name = this.#selected.querySelector(".item-name")?.textContent ?? "";
    await this.send({
      component: "player",
      method: "start",
      arguments: { path: name, index },
    });
  }

  async sendCommand(button: HTMLElement): Promise<void> {
    const args = button.dataset.arguments ? JSON.parse(button.dataset.arguments) : undefined;
    await this.send({
      component: button.dataset.component ?? "",
      method: button.dataset.method ?? "",
      arguments: args,
    });
  }

  /** send posts a command and says so when the player refuses it. */
  async send(req: ApiRequest): Promise<void> {
    const res = await callApi(req);
    if (res.success) return;

    // Without this the operator presses a button, the player refuses, and the
    // page carries on looking like nothing happened.
    console.error("the player refused the instruction:", res);
    this.#spCurrent.textContent = `refused: ${String(res.message ?? "unknown error")}`;
  }
}

/** trimExtension drops the file extension, the way the server's Name does. */
export function trimExtension(filename: string): string {
  const dot = filename.lastIndexOf(".");
  return dot < 0 ? filename : filename.slice(0, dot);
}
