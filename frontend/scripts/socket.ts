// The websocket half of the pages. The server pushes; the browser listens.

/** A message pushed down the websocket. */
export interface WsMessage {
  component?: string;
  method?: string;
  arguments?: Record<string, string>;
  success?: boolean;
  event?: string;
  message?: unknown;
}

export interface ReconnectingSocketOptions {
  path: string;
  onMessage?: (msg: WsMessage) => void;
  onOpen?: () => void;
  onClose?: () => void;
  maxAttempts?: number;
}

/**
 * ReconnectingSocket keeps a websocket to the player open.
 *
 * A websocket handshake gives the page no status code when it fails, so a
 * refused connection is indistinguishable from a server that is still starting
 * up. It backs off instead of retrying on a fixed timer, and after enough
 * failures assumes the session is the problem and goes to the login page -
 * otherwise an expired session leaves the page reconnecting forever behind a
 * "Reconnecting..." overlay.
 */
export class ReconnectingSocket {
  readonly #path: string;
  readonly #onMessage?: (msg: WsMessage) => void;
  readonly #onOpen?: () => void;
  readonly #onClose?: () => void;
  readonly #maxAttempts: number;

  #attempts = 0;
  #conn: WebSocket | undefined;
  #stopped = false;

  constructor(opts: ReconnectingSocketOptions) {
    this.#path = opts.path;
    this.#onMessage = opts.onMessage;
    this.#onOpen = opts.onOpen;
    this.#onClose = opts.onClose;
    this.#maxAttempts = opts.maxAttempts ?? 10;
  }

  connect(): void {
    const scheme = globalThis.location.protocol === "https:" ? "wss:" : "ws:";
    this.#conn = new WebSocket(`${scheme}//${document.location.host}${this.#path}`);

    this.#conn.addEventListener("open", () => {
      this.#attempts = 0;
      this.#onOpen?.();
    });

    this.#conn.addEventListener("error", (event) => {
      console.log("error in the websocket connection:", event);
    });

    this.#conn.addEventListener("close", () => {
      if (this.#stopped) return;
      this.#onClose?.();

      this.#attempts++;
      if (this.#attempts > this.#maxAttempts) {
        console.error("giving up on the websocket; the session has probably expired");
        globalThis.location.href = "/login";
        return;
      }

      const delay = backoffDelay(this.#attempts);
      console.log(`connection closed, reconnecting in ${delay}ms`);
      setTimeout(() => this.connect(), delay);
    });

    this.#conn.addEventListener("message", (event: MessageEvent<string>) => {
      this.#onMessage?.(JSON.parse(event.data) as WsMessage);
    });
  }

  /**
   * stop closes the socket and stops reconnecting, for a page that has been
   * told another device took the connection over.
   */
  stop(): void {
    this.#stopped = true;
    this.#conn?.close();
  }
}

/** backoffDelay is 1s, 2s, 4s... capped at 30s. */
export function backoffDelay(attempt: number): number {
  return Math.min(1000 * 2 ** (attempt - 1), 30000);
}
