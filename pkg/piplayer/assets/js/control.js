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
    let reqBody = {
      component: 'playlist',
      method: 'getItems'
    }

    return this.callApi(reqBody)
    .then(res => {
      if (!res || !res.success) {
        console.error(res);
        return;
      }
      this.playlist.items = res.message;
      return res;
    });
  }

  wsConnect() {
    let u = 'ws://' + document.location.host + this.wsPath;
    this.conn = new WebSocket(u);

    this.conn.addEventListener('open', e => {
      console.log("Connection Opened.");
      this.disconnect = false;

      this.warningHide();
    });
    
    this.conn.addEventListener('error', e => {
      console.log("Error in the websocket connection:\n", e);
    });
  
    this.conn.addEventListener('close', e => {
      if (this.disconnect) {
        console.log("Disconnected from server. Login again to take back control.");
        this.warningShow(this.divDisconnect)
        return;
      }

      console.log("Connection closed.\nTrying to reconnect...");

      this.warningShow(this.divReconnect);
  
      let to = setTimeout(() => this.wsConnect(), 2000);
    });

    this.conn.addEventListener('message', this.socketMessage.bind(this));
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

  socketMessage(e) {
    let msg = JSON.parse(e.data);
    console.log(msg);

    switch (msg.event) {
      case "setCurrent":
        this.setCurrent(parseInt(msg.message))
        break;
      case "disconnect":
      this.disconnect = true;
      console.warn(`server requested websocket disconnection. Connection should be closed any second now.`)
        break;
      default:
      console.log(`Unsupported message received: ${e.data}`);
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
  
    this.callApi(reqBody)
      .then(this.videoCallback.bind(this));
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
  
    this.callApi(reqBody).then(this.videoCallback.bind(this));
  }
  
  videoCallback(json) {
    if (json.success) {
      console.log("instruction sent successfully. awaiting confirmation in socket.");
    }
  }
  
  callApi(reqBody) {
    let myHeaders = new Headers();
    myHeaders.append('Content-Type', 'application/json');
  
    let myInit = {
      method: "POST",
      headers: myHeaders,
      body: JSON.stringify(reqBody)
    }
  
    return fetch(`${window.location.origin}/api`, myInit)
      .then(res => res.json())
      .then(json => {
        console.log(json);
        return json;
      })
      .catch(err => console.error(err));
  }
}

let control = new Control();