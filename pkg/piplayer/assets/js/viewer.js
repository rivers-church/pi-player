class Viewer {
  
  constructor() {
    // make these constants in a module
    this.menuItemSelector = '.item';
    this.wsPath = '/ws/viewer';
    this.conn = null;
    this.arrItems = null;
    this.playlist = {
      current: null,
      items: []
    };
  
    this.divContainer = document.querySelector('#container');
    this.ulPlaylist = document.querySelector('#ulPlaylist');
    this.divContainerPlaylist = document.querySelector('#containerPlaylist');
    this.vidMedia = document.querySelector('#vidMedia');
    this.audMusic = document.querySelector('#audMusic');

    if (!window["WebSocket"]) {
      console.error("This page requires WebSocket support. Please use a WebSocket enabled service.");
      return;
    }
  
    // Ignore all keyboard input on the Pi browser.
    // document.addEventListener("keydown", e => {
    //   e.preventDefault();
    // });
  
    this.arrItems = Array.from(document.querySelectorAll(this.menuItemSelector));
  
    if (this.arrItems.length <= 0) {
      // No items in the menu. Nothing to do here.
      console.warn("No items in the playlist, so then not much to do here?")
      return;
    }

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
    let msg = JSON.parse(e.data)

    console.log(msg);

    switch (msg.message) {
      case 'KEY_UP':
      case 'KEY_DOWN':
        this.remoteArrowPress(e, msg);
        break;
      case 'KEY_LEFT':
        this.remoteArrowLeftPress(e);
        break;
      case 'KEY_RIGHT':
        this.remoteArrowRightPress(e);
        break;
      case 'KEY_ENTER':
        this.remoteEnterPress(e);
        break;
      case 'KEY_CONTEXT_MENU':
        this.remoteContextMenuPress(e);
        break;
      case 'KEY_PLAYPAUSE':
        this.remotePlayPress(e);
        break;
      case 'KEY_STOP':
        this.remoteStopPress(e);
      case 'KEY_FASTFORWARD':
        this.remoteSeek(e, 15);
        break;
      case 'KEY_REWIND':
        this.remoteSeek(e, -15);
        break;
      default:
        console.log("Unsupported message received: ", e.data);
        break;
    }
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

    let diff = msg.message == 'KEY_UP' ? -1 : 1;

    if (msg.message == 'KEY_UP' && i <= 0) {
      i = this.arrItems.length;
    } else if (msg.message == 'KEY_DOWN' && i >= this.arrItems.length - 1) {
      i = -1;
    }

    this.arrItems[i + diff].focus();
  }

  remoteArrowLeftPress(e) {
    if (this.playlist.current == 0) {
      this.startItem(this.playlist.items.length - 1);
      return
    }

      this.startItem(this.playlist.current - 1);
  }

  remoteArrowRightPress(e) {
    if (this.playlist.current >= this.playlist.items.length - 1) {
      this.startItem(0);
      return;
    }

    this.startItem(this.playlist.current + 1);
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

  remotePlayPress(e) {
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

  remoteSeek(e, msg) {
    this.vidMedia.currentTime += msg;
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
          this.vidMedia.style.visibility = 'hidden';
        }
      break;
      default:
        console.log("File type not supported: ", fileName);
        success = false;
        break;
    }
    return success;
  }

  startAudio(fileName) {
    if (fileName == "") {
      this.audMusic.src = "";
      return true;
    }
    
    let success = true;
    let ext = fileName.slice(fileName.lastIndexOf('.'));

    switch (ext) {
      case '.mp3':
        this.audMusic.src = `/content/${fileName}`;
        this.audMusic.play();
        break;
      case '':
        this.audMusic.pause();
        this.audMusic.src = null;
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