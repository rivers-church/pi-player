"use strict";

let viewer = {
  menuItemSelector: '.item',
  arrItems: null,
  conn: null,
  divContainer: document.querySelector('#container'),
  ulPlaylist: document.querySelector('#ulPlaylist'),
  vidMedia: document.querySelector('#vidMedia'),
  audMusic: document.querySelector('#audMusic'),

  run: function() {
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
  
    this.wsConnect();
  },

  wsConnect: function() {
    let u = 'ws://' + document.location.host + '/ws';
    this.conn = new WebSocket(u);

    // If connection is not established, try again after 2 seconds.
    // let to = setTimeout(() => {
    //   if (conn.readyState != 1) {
    //     console.warn("Connection attempt unsuccessfull, trying again...");
    //     wsConnect();
    //   } else {
    //     console.log("Connection successful.");
    //   }
    // }, 2000);
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

    this.conn.addEventListener('message', this.socketMessage);
  },

  socketMessage: function(e) {
    let msg = JSON.parse(e.data)

    console.log(msg);

    switch (msg.message) {
      case 'KEY_UP':
      case 'KEY_DOWN':
        this.remoteArrowPress(e, msg);
        break;
      case 'KEY_ENTER':
        this.remoteEnterPress(e, msg);
        break;
      case 'KEY_HOME':
        this.remoteHomePress(e, msg);
        break;
      default:
        console.log("Unsupported message received: ", e.data);
        break;
    }
  },

  getItems: function() {
    let reqBody = {
      component: 'playlist',
      method: 'getItems'
    }

    this.conn.send(JSON.stringify(reqBody));
  },

  remoteArrowPress: function(e, msg) {
    let selectedItem = document.querySelector(menuItemSelector + ':focus');
    if (selectedItem == null) {
      // No item is selected, focus on first item.
      this.arrItems[0].focus();
      return;
    }

    let i = this.arrItems.indexOf(selectedItem);
    if (i < 0) {
      console.error("Element not in initial array of elements?\nFocusing on first item.")
      arrItems[0].focus();
      return;
    }

    let diff = msg.message == 'KEY_UP' ? -1 : 1;

    if (msg.message == 'KEY_UP' && i <= 0) {
      i = this.arrItems.length;
    } else if (msg.message == 'KEY_DOWN' && i >= this.arrItems.length - 1) {
      i = -1;
    }

    this.arrItems[i + diff].focus();
  },

  remoteEnterPress: function(e, msg) {
    let selectedItem = document.querySelector(menuItemSelector + ':focus');

    if (selectedItem == null) {
      // No item selected, focus on first item again.
      this.arrItems[0].focus();
      return;
    }

    // let reqBody = {
    //   component: 'player',
    //   method: 'start',
    //   arguments: {
    //     path: selectedItem.textContent
    //   }
    // };
    // conn.send(JSON.stringify(reqBody));
    this.startItem(selectedItem);
  },

  remoteHomePress: function(e, msg) {
    // If the menu is hidden, show it.
    if (this.ulPlaylist.style.visibility !== 'visible') {
      this.ulPlaylist.style.visibility = 'visible';
      // arrItems[0].focus();
      return;
    }

    // If the meny is showing, hide it.
    if (this.ulPlaylist.style.visibility === 'visible') {
      this.ulPlaylist.style.visibility = 'hidden';
    }
  },

  remotePlayPress: function(e, msg) {

  },

  remoteStopPress: function(e, msg) {

  },

  remoteSeek: function(e, msg) {
    
  },

  startItem: function(el) {
    let n = el.textContent;
    let ext = n.slice(n.lastIndexOf('.'));
    this.ulPlaylist.style.visibility = 'hidden';

    switch (ext) {
      case '.mp4':
        this.vidMedia.src = `/content/${n}`;
        this.vidMedia.style.visibility = 'visible';
        // Blackout the background.
        this.divContainer.style.backgroundImage = null;
        this.vidMedia.play();
        break;
      case '.jpg':
      case '.jpeg':
      case '.png':
        // Stop video if playing.
        if (!this.vidMedia.paused) {
          this.vidMedia.pause();
          this.vidMedia.style.visibility = 'hidden';
        }
        // Change background image.
        this.divContainer.style.backgroundImage = `url("/content/${n}")`;
      break;
      default:
        console.log("File type not supported: ", ext);
        break;
    }
  }
}

viewer.run();