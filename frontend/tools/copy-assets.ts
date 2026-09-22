// Copy the static frontend assets into the bundle output.
//
// `deno bundle` emits JS and CSS it can reach through the module graph and
// nothing else, so stylesheets linked straight from a Go template, and icon
// files fetched by name at runtime, have to be copied in separately.
//
// Deliberately a delete-then-copy rather than a merge: a file removed from
// source but left behind in dist/ would go on being served, which is harder to
// notice than an absence.

import { join } from "@std/path";

const DIST = join("pkg", "piplayer", "assets", "dist");

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

const styles = await copyDir(join("frontend", "styles"), join(DIST, "css"));
const icons = await copyDir(join("frontend", "icons"), join(DIST, "icons"));

console.log(`copied ${styles} stylesheet(s) and ${icons} icon(s) to ${DIST}`);
