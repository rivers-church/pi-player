// Entry point for the pages with chrome: control, settings, login and error.
//
// One bundle rather than one per page, so the component library is not inlined
// four times. Each page's setup runs only if its markup is present.

import { watchForMediaDirectory } from "./errorPage.ts";

function start(): void {
  const redirect = document.body.dataset.redirect;
  if (redirect) {
    watchForMediaDirectory(redirect);
  }
}

start();
