class Viewer {
  
  constructor() {
    // TODO: make these constants in a module.
    this.menuItemSelector = '.item';
    this.wsPath = '/ws/viewer';
    this.conn = null;
    this.arrItems = null;
    this.playlist = {
      current: null,
      items: []
    };
  
    this.tmpItem = document.querySelector('#tmpItem');
    this.divContainer = document.querySelector('#container');
    this.ulPlaylist = document.querySelector('#ulPlaylist');
    this.divContainerPlaylist = document.querySelector('#containerPlaylist');
    this.vidMedia = document.querySelector('#vidMedia');
    this.audMusic = document.querySelector('#audMusic');

    // Listen for when the video ends and start the next item.
    this.vidMedia.addEventListener('ended', e => {
      this.next();
    });

    if (!window["WebSocket"]) {
      console.error("This page requires WebSocket support. Please use a WebSocket enabled service.");
      return;
    }
  
    // Ignore all keyboard input on the Pi browser.
    // document.addEventListener("keydown", e => {
    //   e.preventDefault();
    // });
  
    this.getItems().then(res => {
      this.startItem(0);
    });
    this.wsConnect();
  }

  wsConnect() {
    let u = 'ws://' + document.location.host + this.wsPath;
    this.conn = new WebSocket(u);

    this.conn.addEventListener('open', e => {
      console.log("Connection Opened.");
    });
    
    this.conn.addEventListener('error', e => {
      console.log("Error in the websocket connection:\n", e);
    });
  
    this.conn.addEventListener('close', e => {
      console.log("Connection closed.\nTrying to reconnect...");
  
      let to = setTimeout(() => this.wsConnect(), 2000);
    });

    this.conn.addEventListener('message', this.socketMessage.bind(this));
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

  socketMessage(e) {
    let msg = JSON.parse(e.data);
    console.log(msg);

    switch(msg.component) {
      case 'remote':
        this.remoteMessage(e, msg);
        break;
      case 'player':
        this.playerMessage(e, msg);
        break;
      case 'playlist':
        this.playlistMessage(e, msg);
        break;
      default:
        console.error(`unsupported component: ${msg.component};\nmessage: ${msg}`);
    }
  }

  playlistMessage(e, msg) {
    switch (msg.event) {
      case "newItems":
        this.getItems();
        break;
      default:
        console.error(`unsupported playlist method: ${msg.component};\nmessage: ${msg}`);
    }
  }

  remoteMessage(e, msg) {
    switch (msg.arguments.keyString) {
      case 'KEY_UP':
      case 'KEY_DOWN':
        this.remoteArrowPress(e, msg);
        break;
      case 'KEY_LEFT':
        this.previous(e);
        break;
      case 'KEY_RIGHT':
        this.next(e);
        break;
      case 'KEY_ENTER':
        this.remoteEnterPress(e);
        break;
      case 'KEY_CONTEXT_MENU':
        this.remoteContextMenuPress(e);
        break;
      case 'KEY_PLAYPAUSE':
        this.playPause(e);
        break;
      case 'KEY_STOP':
        this.remoteStopPress(e);
      case 'KEY_FASTFORWARD':
        this.seek(e, 15);
        break;
      case 'KEY_REWIND':
        this.seek(e, -15);
        break;
      default:
        console.log("Unsupported message received: ", e.data);
        break;
    }
  }

  playerMessage(e, msg) {
    switch (msg.method) {
      case 'start':
        this.startItem(msg.message);
        break;
      case 'stop':
        break;
      case 'play':
      case 'pause':
        this.playPause(e);
        break;
      case 'seek':
        this.seek(e, msg.arguments.value);
        break;
      case 'previous':
        this.previous(e);
        break;
      case 'next':
        this.next(e);
        break;
      default:
        console.error(`unsupported method: ${msg.method}\nmessage: ${msg}`);
    }
  }

  // getItems retrieves an array of items from the API.
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
        this.genItems();
        return res;
      });
  }

  // genItems re-generates the html for the playlist items.
  genItems() {
    // First clear out the current items.
    this.ulPlaylist.innerHTML = '';

    this.playlist.items.forEach((item, i, arr) => {
      let cloneItem = document.importNode(this.tmpItem.content, true);
      let icons = cloneItem.querySelectorAll('i');
      let spName = cloneItem.querySelector('span.itemName');
      icons[0].classList.add("fa-" + item.Type);
      if (item.Audio == "") {
        icons[1].remove();
      }
      spName.textContent = item.Visual;
      this.ulPlaylist.appendChild(cloneItem);
    });

    this.arrItems = Array.from(this.ulPlaylist.querySelectorAll(this.menuItemSelector));
  }

  remoteArrowPress(e, msg) {
    let selectedItem = document.querySelector(this.menuItemSelector + ':focus');
    if (selectedItem == null) {
      // No item is selected, focus on first item.
      this.arrItems[0].focus();
      return;
    }

    let i = this.arrItems.indexOf(selectedItem);
    if (i < 0) {
      console.error("Element not in initial array of elements?\nFocusing on first item.")
      this.arrItems[0].focus();
      return;
    }

    let up = msg.arguments.keyString == 'KEY_UP';
    let diff = up ? -1 : 1;

    if (up && i <= 0) {
      i = this.arrItems.length;
    } else if (!up && i >= this.arrItems.length - 1) {
      i = -1;
    }

    this.arrItems[i + diff].focus();
  }

  previous(e) {
    if (this.playlist.current == 0) {
      this.startItem(this.playlist.items.length - 1);
      return
    }

      this.startItem(parseInt(this.playlist.current) - 1);
  }

  next(e) {
    if (this.playlist.current >= this.playlist.items.length - 1) {
      this.startItem(0);
      return;
    }

    this.startItem(parseInt(this.playlist.current, 10) + 1);
  }

  remoteEnterPress(e) {
    let selectedItem = document.querySelector(this.menuItemSelector + ':focus');

    if (selectedItem == null) {
      // No item selected, focus on first item again.
      this.arrItems[0].focus();
      return;
    }

    let i = parseInt(selectedItem.dataset.index, 10);
    this.startItem(i);
  }

  remoteContextMenuPress(e) {
    // If the menu is hidden, show it.
    if (this.divContainerPlaylist.style.visibility !== 'visible') {
      this.divContainerPlaylist.style.visibility = 'visible';
      this.arrItems[this.playlist.current].focus();
      return;
    }

    // If the meny is showing, hide it.
    if (this.divContainerPlaylist.style.visibility === 'visible') {
      this.divContainerPlaylist.style.visibility = 'hidden';
    }
  }

  playPause(e) {
    let item = this.playlist.items[this.playlist.current];

    if (item.Audio != "") {
      if (this.audMusic.paused) {
        this.audMusic.play();
      } else {
        this.audMusic.pause();
      }
    }

    if (item.Type == "video") {
      if (this.vidMedia.paused) {
        this.vidMedia.play();
      } else {
        this.vidMedia.pause();
      }
    }
  }

  remoteStopPress(e) {
    let item = this.playlist.items[this.playlist.current];

    if (item.Type == "video") {
      this.vidMedia.pause();
      this.vidMedia.currentTime = 0;
    }

    if (item.Audio != "") {
      this.audMusic.pause();
      this.audMusic.currentTime = 0;
    }
  }

  seek(e, value) {
    value = parseInt(value, 10);
    this.vidMedia.currentTime += value;
  }

  startItem(index) {
    if (index <= -1) {
      console.error("Cannot play item at negative index.");
      return;
    }

    this.divContainerPlaylist.style.visibility = 'hidden';
    let item = this.playlist.items[index];

    this.startAudio(item.Audio);
    let started = this.startVisual(item.Visual);
    if (started) {
      this.playlist.current = index;
    }

    // Notify the server that a new item has started.
    let reqBody = {
      component: "playlist",
      method: "setCurrent",
      arguments: {index: index.toString()}
    };

    this.callApi(reqBody).then(res => {
      if (!res || !res.success) {
        console.error("Cound't set the current item through the API.");
      }
    });
  }

  startVisual(fileName) {
    let success = true;
    let ext = fileName.slice(fileName.lastIndexOf('.'));

    switch (ext) {
      case '.mp4':
        this.vidMedia.src = `/content/${fileName}`;
        this.vidMedia.style.visibility = 'visible';
        this.vidMedia.play();
        // Blackout the background.
        this.divContainer.style.backgroundImage = null;
        break;
      case '.jpg':
      case '.jpeg':
      case '.png':
        // Change background image.
        this.divContainer.style.backgroundImage = `url("/content/${fileName}")`;
        // Stop video if playing.
        if (!this.vidMedia.paused) {
          this.vidMedia.pause();
        }
        this.vidMedia.style.visibility = 'hidden';
      break;
      default:
        console.log("File type not supported: ", fileName);
        success = false;
        break;
    }
    return success;
  }

  startAudio(fileName) {
    let success = true;

    if (this.audMusic.src != "" && !this.audMusic.paused) {
      this.audMusic.pause();
    }

    if (fileName == "") {
      this.audMusic.src = "";
      return success;
    }
    
    let ext = fileName.slice(fileName.lastIndexOf('.'));

    switch (ext) {
      case '.mp3':
        this.audMusic.src = `/content/${fileName}`;
        this.audMusic.play();
        break;
      case '':
        this.audMusic.pause();
        this.audMusic.src = "";
        break;
      default:
        console.log("File type not supported: ", fileName)
        success = false;
        break;
    }
    return success;
  }
}

let viewer = new Viewer();