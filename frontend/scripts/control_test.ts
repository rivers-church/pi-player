import { assertEquals } from "@std/assert";
import { trimExtension } from "./control.ts";

Deno.test("trimExtension matches what the server renders into the playlist", () => {
  // The server writes Item.Name into the table, which has no extension; the
  // API returns Visual, which does. Showing one where the other is expected is
  // how "currently playing" used to disagree with the row it highlighted.
  assertEquals(trimExtension("PiPlayer Logo.png"), "PiPlayer Logo");
  assertEquals(trimExtension("clip.mp4"), "clip");
  assertEquals(trimExtension("two.dots.here.webm"), "two.dots.here");
  assertEquals(trimExtension("no-extension"), "no-extension");
});
