// ReconnectingSocket keeps a websocket to the player open.
//
// A websocket handshake gives the page no status code when it fails, so a
// refused connection is indistinguishable from a server that is still starting
// up. It backs off instead of retrying on a fixed timer, and after enough
// failures assumes the session is the problem and goes to the login page -
// otherwise an expired session leaves the page reconnecting forever behind a
// "Reconnecting..." overlay.
export class ReconnectingSocket {
  constructor({path, onMessage, onOpen, onClose, maxAttempts = 10}) {
    this.path = path;
    this.onMessage = onMessage;
    this.onOpen = onOpen;
    this.onClose = onClose;
    this.maxAttempts = maxAttempts;

    this.attempts = 0;
    this.conn = null;
    this.stopped = false;
  }

  connect() {
    const scheme = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    this.conn = new WebSocket(`${scheme}//${document.location.host}${this.path}`);

    this.conn.addEventListener('open', () => {
      this.attempts = 0;
      if (this.onOpen) this.onOpen();
    });

    this.conn.addEventListener('error', e => {
      console.log('error in the websocket connection:', e);
    });

    this.conn.addEventListener('close', () => {
      if (this.stopped) return;
      if (this.onClose) this.onClose();

      this.attempts++;
      if (this.attempts > this.maxAttempts) {
        console.error('giving up on the websocket; the session has probably expired');
        window.location.href = '/login';
        return;
      }

      // 1s, 2s, 4s... capped at 30s.
      const delay = Math.min(1000 * 2 ** (this.attempts - 1), 30000);
      console.log(`connection closed, reconnecting in ${delay}ms`);
      setTimeout(() => this.connect(), delay);
    });

    if (this.onMessage) {
      this.conn.addEventListener('message', e => this.onMessage(JSON.parse(e.data)));
    }
  }

  // stop closes the socket and stops reconnecting, for a page that has been
  // told another device took the connection over.
  stop() {
    this.stopped = true;
    if (this.conn) this.conn.close();
  }
}
