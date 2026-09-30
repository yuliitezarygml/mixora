import test from "node:test";
import assert from "node:assert/strict";
import { waveFeedbackPath, withWaveSession } from "./waveFeedback.js";

test("wave events carry the opaque recommendation session", () => {
  assert.deepEqual(withWaveSession({ type: "like" }, " wave-1 "), {
    type: "like",
    session_id: "wave-1",
  });
  assert.deepEqual(withWaveSession({ type: "like" }, ""), { type: "like" });
});

test("only actionable events use the immediate feedback endpoint", () => {
  assert.equal(
    waveFeedbackPath("session/1", "skip"),
    "/wave/session%2F1/feedback",
  );
  assert.equal(waveFeedbackPath("session-1", "search"), "");
  assert.equal(waveFeedbackPath("", "like"), "");
});
