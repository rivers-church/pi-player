"use strict";

let controls = {
  btns: document.querySelectorAll('#divControlsPlayer button'),
  btnsPlaylist: document.querySelectorAll('#divControlPlaylist'),
  btnStart: document.querySelector('#btnStart'),
  spCurrent: document.querySelector('#spCurrent'),
  tblPlaylist: document.querySelector('#tblPlaylist'),
  socket: null,
  flags: {
    websockets: false
  }
};

if (!window["WebSocket"]) {
  console.warn("Websockets not supported in your browser. Fallback functionality will be used.");
} else if (controls.flags.websockets) {
  controls.socket = new WebSocket(`ws://${document.location.host}/control/ws`);
  controls.socket.addEventListener('close', () => {
    console.log("Websocket connection closed, falling back...");
  });

  controls.socket.addEventListener('message', e => {
    let msg = JSON.parse(e.data);
    console.log('WS Message received!!: ', msg);
  });
}

let playlist = {
  items: Array.from(controls.tblPlaylist.querySelectorAll('td')).map(el => el.textContent),
  selected: null,
  playPause: e => console.log(e)
}

controls.tblPlaylist.addEventListener('click', plSelect);
controls.btns.forEach(btn => btn.addEventListener('click', sendCommand));
controls.btnsPlaylist.forEach(btn => btn.addEventListener('click', callMethod));
controls.btnStart.addEventListener('click', startItem);

function plSelect(e) {
  if (playlist.selected != null) {
    playlist.selected.classList.remove('selected');
  }
  playlist.selected = e.target;
  playlist.selected.classList.add('selected');
}

function callMethod(e) {
  let btn = e.target.closest('button');

  let reqBody = {
    component: "player",
    method: btn.dataset["method"]
  };

  callApi(reqBody)
    .then(videoCallback);
}

function startItem(e) {
  let reqBody = {
    component: "player",
    method: "start",
    arguments: {
      path: playlist.selected.textContent
    }
  };

  callApi(reqBody).then(videoCallback);
}

function videoCallback(json) {
  if (json.success) {
    spCurrent.textContent = json.message;
    let event = {};
    let items = Array.from(tblPlaylist.querySelectorAll('td'));
    event.target = items.find(val => {
      return val.textContent == json.message;
    });
    plSelect(event);
  }
}

function sendCommand(e) {
  let btn = e.target.closest('button');
  
  let reqBody = {
    component: "player",
    method: "sendCommand",
    arguments: {
      command: btn.dataset["command"]
    }
  };

  callApi(reqBody);
}

function callApi(reqBody) {
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