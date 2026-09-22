import {callApi, getItems} from './api.js';
import {ReconnectingSocket} from './socket.js';

class Control {
  constructor() {
    // make these constants in a module
    this.btns = document.querySelectorAll('#divControlsPlayer button');
    // this.btnsPlaylist = document.querySelectorAll('#divControlPlaylist');
    this.btnStart = document.querySelector('#btnStart');
    this.spCurrent = document.querySelector('#spCurrent');
    this.tblPlaylist = document.querySelector('#tblPlaylist');
    this.divOverlay = document.querySelector('#divOverlay');
    this.divReconnect = document.querySelector('#divReconnect');
    this.divDisconnect = document.querySelector('#divDisconnect');
    this.wsPath = "/ws/control";

    this.conn = null;
    this.playlist = {
      current: null,
      selected: null,
      items: []
    };

    if (!window["WebSocket"]) {
      console.error("This page requires WebSocket support. Please use a WebSocket enabled service.");
      return;
    }

    this.getItems().then(res => {
      console.log("loaded playlist from server");
    })

    this.wsConnect();

    this.tblPlaylist.addEventListener('click', this.plSelect.bind(this));
    this.btns.forEach(btn => btn.addEventListener('click', this.callMethod.bind(this)));
    // this.btnsPlaylist.forEach(btn => btn.addEventListener('click', this.callMethod.bind(this)));
    this.btnStart.addEventListener('click', this.startItem.bind(this));
  }

  getItems() {
    return getItems().then(res => {
      if (!res.success) {
        console.error('could not load the playlist:', res);
        return res;
      }
      this.playlist.items = res.message;
      return res;
    });
  }

  wsConnect() {
    this.conn = new ReconnectingSocket({
      path: this.wsPath,
      onOpen: () => {
        this.disconnect = false;
        this.warningHide();
      },
      onClose: () => {
        if (this.disconnect) {
          // Another device took the connection; reconnecting would just take
          // it back off them.
          this.conn.stop();
          this.warningShow(this.divDisconnect);
          return;
        }
        this.warningShow(this.divReconnect);
      },
      onMessage: this.socketMessage.bind(this),
    });
    this.conn.connect();
  }

  warningShow(warning) {
    this.divOverlay.style.display = 'grid';
    warning.style.display = 'block';
  }

  warningHide() {
    this.divOverlay.style.display = '';
    let warnings = Array.from(this.divOverlay.querySelectorAll('.warning'));
    warnings.forEach(e => e.style.display = '');
  }

  socketMessage(msg) {
    switch (msg.event) {
      case "setCurrent":
        this.setCurrent(parseInt(msg.message))
        break;
      case "disconnect":
      this.disconnect = true;
      console.warn(`server requested websocket disconnection. Connection should be closed any second now.`)
        break;
      default:
      console.log('unsupported message received:', msg);
    }
  }

  plSelect(e) {
    if (this.playlist.selected != null) {
      this.playlist.selected.classList.remove('selected');
    }
    this.playlist.selected = e.target.closest('tr');
    this.playlist.selected.classList.add('selected');
  }

  setCurrent(index) {
    this.playlist.current = index;
    this.spCurrent.textContent = this.playlist.items[index].Visual;
    let el = this.tblPlaylist.querySelector(`tr[data-index="${index}"]`);
    this.plSelect({target: el});
  }
  
  callMethod(e) {
    let btn = e.target.closest('button');
    let args = null;

    if (btn.dataset.arguments) {
      args = JSON.parse(btn.dataset.arguments);
    }
  
    let reqBody = {
      component: btn.dataset.component,
      method: btn.dataset.method,
      arguments: args,
    };
  
    callApi(reqBody).then(this.videoCallback.bind(this));
  }
  
  startItem(e) {
    let s = this.playlist.selected;
    let itemName = s.querySelector('td.item-name').textContent;
    let reqBody = {
      component: "player",
      method: "start",
      arguments: {
        path: itemName,
        index: s.dataset.index
      }
    };
  
    callApi(reqBody).then(this.videoCallback.bind(this));
  }
  
  videoCallback(json) {
    if (json.success) {
      console.log('instruction sent, awaiting confirmation on the socket');
      return;
    }
    // Without this the operator presses a button, the player refuses, and the
    // page carries on looking like nothing happened.
    console.error('the player refused the instruction:', json);
    window.alert(`The player refused that: ${json.message ?? 'unknown error'}`);
  }
  
}

let control = new Control();