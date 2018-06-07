"use strict";

run();

function run() {
  if (!window["WebSocket"]) {
    console.error("This page requires WebSocket support. Please use a WebSocket enabled service.");
    return;
  }

  // Ignore all keyboard input on the Pi browser.
  document.addEventListener("keydown", e => {
    e.preventDefault();
  });

  const menuItemSelector = '.item';
  const divContainer = document.querySelector('#container');
  const ulPlaylist = document.querySelector('#ulPlaylist');
  const vidMedia = document.querySelector('#vidMedia');
  const audMusic = document.querySelector('#audMusic');
  const arrItems = Array.from(document.querySelectorAll(menuItemSelector));

  if (arrItems.length <= 0) {
    // No items in the menu. Nothing to do here.
    console.error("No items in the playlist, so then not much to do here?")
    return;
  }

  let conn = new WebSocket('ws://' + document.location.host + '/ws');
  conn.addEventListener('close', e => {
    console.log("Connection closed.");
    // TODO retry connection
  });

  conn.addEventListener('message', e => {
    let msg = JSON.parse(e.data)

    console.log(msg);

    switch (msg.message) {
      case 'KEY_UP':
      case 'KEY_DOWN':
        remoteArrowPress(e, msg);
        break;
      case 'KEY_ENTER':
        remoteEnterPress(e, msg);
        break;
      case 'KEY_HOME':
        remoteHomePress(e, msg);
        break;
      default:
        console.log("Unsupported message received: ", e.data);
        break;
    }
  });

  function remoteArrowPress(e, msg) {
    let selectedItem = document.querySelector(menuItemSelector + ':focus');
    if (selectedItem == null) {
      // No item is selected, focus on first item.
      arrItems[0].focus();
      return;
    }

    let i = arrItems.indexOf(selectedItem);
    if (i < 0) {
      console.error("Element not in initial array of elements?\nFocusing on first item.")
      arrItems[0].focus();
      return;
    }

    let diff = msg.message == 'KEY_UP' ? -1 : 1;

    if (msg.message == 'KEY_UP' && i <= 0) {
      i = arrItems.length;
    } else if (msg.message == 'KEY_DOWN' && i >= arrItems.length - 1) {
      i = -1;
    }

    arrItems[i + diff].focus();
  }

  function remoteEnterPress(e, msg) {
    let selectedItem = document.querySelector(menuItemSelector + ':focus');

    if (selectedItem == null) {
      // No item selected, focus on first item again.
      arrItems[0].focus();
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
    startItem(selectedItem);
  }


  function remoteHomePress(e, msg) {
    // If the menu is hidden, show it.
    if (ulPlaylist.style.visibility !== 'visible') {
      ulPlaylist.style.visibility = 'visible';
      // arrItems[0].focus();
      return;
    }

    // If the meny is showing, hide it.
    if (ulPlaylist.style.visibility === 'visible') {
      ulPlaylist.style.visibility = 'hidden';
    }
  }

  function startItem(el) {
    let n = el.textContent;
    let ext = n.slice(n.lastIndexOf('.'));

    switch (ext) {
      case '.mp4':
        // play video
        ulPlaylist.style.visibility = 'hidden';
        break;
      case '.jpg':
      case '.jpeg':
      case '.png':
        // change background image
        divContainer.style.backgroundImage = `url("/content/${n}")`;
        ulPlaylist.style.visibility = 'hidden';
      break;
      default:
        console.log("File type not supported: ", ext);
        break;
    }
  }
}