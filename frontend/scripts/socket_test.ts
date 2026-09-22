import { assertEquals } from "@std/assert";
import { backoffDelay } from "./socket.ts";

Deno.test("backoff doubles and then holds at thirty seconds", () => {
  assertEquals(backoffDelay(1), 1000);
  assertEquals(backoffDelay(2), 2000);
  assertEquals(backoffDelay(3), 4000);
  assertEquals(backoffDelay(6), 30000);
  // Without the cap this would be minutes, and a kiosk left overnight would
  // stop trying to come back in any useful time.
  assertEquals(backoffDelay(20), 30000);
});
