import test from "node:test";
import assert from "node:assert/strict";
import {
  searchListeningEvents,
  trackListeningEvent,
} from "./listeningEvents.js";

const options = {
  id: (() => {
    let value = 0;
    return () => `event-${++value}`;
  })(),
  now: () => "2026-09-30T00:00:00.000Z",
};

test("track events use provider-neutral references", () => {
  assert.deepEqual(
    trackListeningEvent(
      "play",
      { id: 42, source: "soundcloud" },
      { position_ms: 10 },
      options,
    ),
    {
      idempotency_key: "event-1",
      type: "play",
      track_source: "soundcloud",
      track_id: "42",
      occurred_at: "2026-09-30T00:00:00.000Z",
      position_ms: 10,
    },
  );
});

test("search creates one query event and bounded result impressions", () => {
  let value = 0;
  const events = searchListeningEvents(
    " ambient ",
    Array.from({ length: 12 }, (_, index) => ({
      id: index + 1,
      source: "soundcloud",
    })),
    {
      id: () => `search-${++value}`,
      now: () => "2026-09-30T00:00:00.000Z",
    },
  );
  assert.equal(events.length, 11);
  assert.equal(events[0].type, "search");
  assert.equal(events[0].context.query, "ambient");
  assert.equal(events[1].type, "impression");
  assert.equal(events[10].context.rank, 10);
});
