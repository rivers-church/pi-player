// Copy the static frontend assets into the bundle output.
//
// `deno bundle` emits JS and CSS it can reach through the module graph and
// nothing else, so stylesheets linked straight from a Go template, and icon
// files fetched by name at runtime, have to be copied in separately.
//
// Deliberately a delete-then-copy rather than a merge: a file removed from
// source but left behind in dist/ would go on being served, which is harder to
// notice than an absence.

import { fromFileUrl, join } from "@std/path";

const DIST = join("pkg", "piplayer", "assets", "dist");

// Web Awesome fetches its icon SVGs from the Font Awesome CDN at runtime,
// which a player on an isolated LAN cannot reach and the content security
// policy would refuse anyway. ui.ts points the icon library at these instead,
// so every icon the pages name has to be here.
const ICONS = [
  "play",
  "pause",
  "stop",
  "step-backward",
  "step-forward",
  "fast-backward",
  "fast-forward",
  "play-circle",
  "music",
  "bell-slash",
  // Playlist rows take their icon from the item type: video, image, or an
  // html file. Font Awesome has no "browser", so that one borrows a window.
  "video",
  "image",
  "window-maximize",
];

async function copyDir(from: string, to: string): Promise<number> {
  await Deno.remove(to, { recursive: true }).catch(() => {});
  await Deno.mkdir(to, { recursive: true });

  let count = 0;
  for await (const entry of Deno.readDir(from)) {
    if (!entry.isFile) continue;
    await Deno.copyFile(join(from, entry.name), join(to, entry.name));
    count++;
  }
  return count;
}

// Resolved through the module graph rather than by guessing where Deno caches
// npm packages.
const faEntry = fromFileUrl(
  import.meta.resolve("@fortawesome/fontawesome-free/js/all.js"),
);
const faSolid = join(faEntry, "..", "..", "svgs", "solid");

async function copyIcons(to: string): Promise<number> {
  await Deno.remove(to, { recursive: true }).catch(() => {});
  await Deno.mkdir(to, { recursive: true });

  for (const name of ICONS) {
    // Let a missing icon fail the build: a silently absent SVG is a blank
    // button nobody notices until they are standing at the device.
    await Deno.copyFile(join(faSolid, `${name}.svg`), join(to, `${name}.svg`));
  }
  return ICONS.length;
}

const styles = await copyDir(join("frontend", "styles"), join(DIST, "css"));
const icons = await copyIcons(join(DIST, "icons"));

console.log(`copied ${styles} stylesheet(s) and ${icons} icon(s) to ${DIST}`);
