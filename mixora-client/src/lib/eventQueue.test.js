import test from "node:test";
import assert from "node:assert/strict";
import {
  acknowledgeEvents,
  enqueueEvent,
  eventBatch,
  eventQueueKey,
} from "./eventQueue.js";

const event = (id) => ({ idempotency_key: id, type: "play" });

test("offline event queue deduplicates and stays bounded", () => {
  const queued = [event("a"), event("b")];
  assert.deepEqual(enqueueEvent(queued, event("a")), [event("b"), event("a")]);
  assert.deepEqual(enqueueEvent(queued, event("c"), 2), [
    event("b"),
    event("c"),
  ]);
});

test("event queue sends bounded batches and only acknowledges delivered ids", () => {
  const queued = Array.from({ length: 120 }, (_, index) =>
    event(String(index)),
  );
  const batch = eventBatch(queued);
  assert.equal(batch.length, 100);
  const remaining = acknowledgeEvents(queued, batch);
  assert.equal(remaining.length, 20);
  assert.equal(remaining[0].idempotency_key, "100");
});

test("event queue is isolated per account", () => {
  assert.equal(eventQueueKey("user-a"), "mixora-ui:event-queue:user-a");
  assert.notEqual(eventQueueKey("user-a"), eventQueueKey("user-b"));
});
