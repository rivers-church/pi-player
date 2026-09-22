// Entry point for the pages with chrome: control, settings, login and error.
//
// One bundle rather than one per page, so the component library is not inlined
// four times. Each page's setup runs only if its markup is present.

import { registerIconLibrary } from "@awesome.me/webawesome/dist/components/icon/library.js";

import WaButton from "@awesome.me/webawesome/dist/components/button/button.js";
import WaButtonGroup from "@awesome.me/webawesome/dist/components/button-group/button-group.js";
import WaCallout from "@awesome.me/webawesome/dist/components/callout/callout.js";
import WaDialog from "@awesome.me/webawesome/dist/components/dialog/dialog.js";
import WaIcon from "@awesome.me/webawesome/dist/components/icon/icon.js";
import WaInput from "@awesome.me/webawesome/dist/components/input/input.js";
import WaSwitch from "@awesome.me/webawesome/dist/components/switch/switch.js";

import "@awesome.me/webawesome/dist/styles/themes/default.css";
import "@awesome.me/webawesome/dist/styles/webawesome.css";

import { watchForMediaDirectory } from "./errorPage.ts";

// Importing a component is what registers its custom element, so the bundler
// must not treeshake these away as unused.
void (WaButton && WaButtonGroup && WaCallout && WaDialog && WaIcon && WaInput && WaSwitch);

// Web Awesome fetches icon SVGs from the Font Awesome CDN unless told
// otherwise. The players sit on a LAN with no route to it, and the content
// security policy is 'self' only, so every icon comes from the binary.
registerIconLibrary("default", {
  resolver: (name: string) => `/assets/dist/icons/${name}.svg`,
});

function start(): void {
  const redirect = document.body.dataset.redirect;
  if (redirect) {
    watchForMediaDirectory(redirect);
  }
}

start();
