// The error page is what the kiosk display shows when the media directory
// isn't readable. Poll until it comes back, then return to the page that
// failed - there is nobody at the device to press reload.

const pollInterval = 3000;

export function watchForMediaDirectory(redirect: string): void {
  const check = async (): Promise<void> => {
    try {
      const res = await fetch("/api/dircheck");
      if (res.ok) {
        const data = await res.json() as { ok?: boolean };
        if (data.ok) {
          globalThis.location.href = redirect;
          return;
        }
      }
    } catch {
      // The server is down or restarting; keep waiting for it.
    }
    setTimeout(check, pollInterval);
  };

  setTimeout(check, pollInterval);
}
