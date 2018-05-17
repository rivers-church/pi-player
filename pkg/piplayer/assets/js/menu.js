"use strict";

run();

function run() {
  if (!window["WebSocket"]) {
    console.error("This page requires WebSocket support. Please use a WebSocket enabled service.");
    return;
  }

  // Ignore all keyboard input on the Pi
  // document.addEventListener("keydown", e => {
  //   e.preventDefault();
  // });

  const menuItemSelector = '.item';
  const arrItems = Array.from(document.querySelectorAll(menuItemSelector));

  if (arrItems.length <= 0) {
    // No items in the menu. Nothing to do here.
    return;
  }

  let conn = new WebSocket('ws://' + document.location.host + '/ws');
  conn.addEventListener('close', e => {
    console.log("Connection closed.");
    // TODO retry connection
  });

  conn.addEventListener('message', e => {
    switch (e.data) {
      case 'KEY_UP':
      case 'KEY_DOWN':
        remoteArrowPress(e);
        break;
      case 'KEY_ENTER':
        remoteEnterPress(e);
        break;
      default:
        console.log("Unsupported message received: ", e.data);
        break;
    }
  });

  function remoteArrowPress(e) {
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

    let diff = e.data == 'KEY_UP' ? -1 : 1;

    if (e.data == 'KEY_UP' && i <= 0) {
      i = arrItems.length;
    } else if (e.data == 'KEY_DOWN' && i >= arrItems.length - 1) {
      i = -1;
    }

    arrItems[i + diff].focus();
  }

  function remoteEnterPress(e) {
    let selectedItem = document.querySelector(menuItemSelector + ':focus');
    let reqBody = {
      component: 'player',
      method: 'start',
      arguments: {
        path: selectedItem.textContent
      }
    };
    conn.send(JSON.stringify(reqBody));
  }
}