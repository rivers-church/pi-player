// The error page is what the kiosk display shows when the media directory
// isn't readable. Poll until it comes back, then return to the page that
// failed - there is nobody at the device to press reload.
const redirect = document.body.dataset.redirect;
const pollInterval = 3000;

async function check() {
  try {
    const res = await fetch('/api/dircheck');
    if (res.ok) {
      const data = await res.json();
      if (data.ok) {
        window.location.href = redirect;
        return;
      }
    }
  } catch (err) {
    // The server is down or restarting; keep waiting for it.
  }
  setTimeout(check, pollInterval);
}

setTimeout(check, pollInterval);
