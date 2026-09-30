const immediateTypes = new Set([
  "play",
  "listen_30s",
  "complete",
  "skip",
  "repeat",
  "like",
  "dislike",
  "add_to_playlist",
]);

export function withWaveSession(event, sessionId) {
  const value = typeof sessionId === "string" ? sessionId.trim() : "";
  return value ? { ...event, session_id: value } : event;
}

export function waveFeedbackPath(sessionId, eventType) {
  const value = typeof sessionId === "string" ? sessionId.trim() : "";
  if (!value || !immediateTypes.has(eventType)) return "";
  return `/wave/${encodeURIComponent(value)}/feedback`;
}
