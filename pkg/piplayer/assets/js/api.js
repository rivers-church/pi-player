// callApi posts a message to the player's JSON API.
//
// It always resolves to a response object, never to undefined, so callers can
// check .success without guarding for a rejected promise first. A 401 means
// the session has expired - there is nothing useful the page can do in that
// state, so it goes back to the login page.
export async function callApi(reqBody) {
  let res;

  try {
    res = await fetch(`${window.location.origin}/api`, {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(reqBody),
    });
  } catch (err) {
    console.error('could not reach the player:', err);
    return {success: false, event: 'networkError', message: String(err)};
  }

  if (res.status === 401) {
    window.location.href = '/login';
    return {success: false, event: 'notLoggedIn'};
  }

  let json;
  try {
    json = await res.json();
  } catch (err) {
    console.error(`the player answered ${res.status} with something that isn't JSON:`, err);
    return {success: false, event: 'badResponse', message: String(err)};
  }

  if (!res.ok) {
    console.error(`the player answered ${res.status}:`, json);
  }
  return json;
}

// getItems fetches the playlist.
export function getItems() {
  return callApi({component: 'playlist', method: 'getItems'});
}

// setCurrent tells the player which item is the current one.
export function setCurrent(index) {
  return callApi({
    component: 'playlist',
    method: 'setCurrent',
    arguments: {index: index.toString()},
  });
}
