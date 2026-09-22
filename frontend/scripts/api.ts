// The JSON API the pages talk to. The websocket is outbound-only - the server
// never reads structured messages from the browser - so everything the pages
// ask for goes through here.

/** A request to the player's API. */
export interface ApiRequest {
  component: string;
  method: string;
  arguments?: Record<string, string>;
}

/** The player's reply. `success` is false for anything the server refused. */
export interface ApiResponse {
  success: boolean;
  event?: string;
  message?: unknown;
}

/** One playlist entry, as the server describes it. */
export interface Item {
  Audio: string;
  Visual: string;
  Type: string;
  Cues: Record<string, string>;
}

/**
 * callApi posts a message to the player's API.
 *
 * It always resolves to a response object, never to undefined, so callers can
 * check .success without guarding for a rejected promise first. A 401 means
 * the session has expired - there is nothing useful the page can do in that
 * state, so it goes back to the login page.
 */
export async function callApi(reqBody: ApiRequest): Promise<ApiResponse> {
  let res: Response;

  try {
    res = await fetch(`${globalThis.location.origin}/api`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(reqBody),
    });
  } catch (err) {
    console.error("could not reach the player:", err);
    return { success: false, event: "networkError", message: String(err) };
  }

  if (res.status === 401) {
    globalThis.location.href = "/login";
    return { success: false, event: "notLoggedIn" };
  }

  let json: ApiResponse;
  try {
    json = await res.json();
  } catch (err) {
    console.error(`the player answered ${res.status} with something that isn't JSON:`, err);
    return { success: false, event: "badResponse", message: String(err) };
  }

  if (!res.ok) {
    console.error(`the player answered ${res.status}:`, json);
  }
  return json;
}

/** getItems fetches the playlist. */
export function getItems(): Promise<ApiResponse> {
  return callApi({ component: "playlist", method: "getItems" });
}

/** setCurrent tells the player which item is the current one. */
export function setCurrent(index: number): Promise<ApiResponse> {
  return callApi({
    component: "playlist",
    method: "setCurrent",
    arguments: { index: index.toString() },
  });
}
