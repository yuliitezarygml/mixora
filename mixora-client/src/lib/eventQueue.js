const MAX_QUEUED_EVENTS = 500;
const EVENT_BATCH_SIZE = 100;

const canonicalEvent = (event) => {
  if (event?.track_source?.toLowerCase() !== "soundcloud") return event;
  const raw = String(event.track_id || "").trim();
  const id = raw.match(/(?:^|:|\/)(\d+)$/)?.[1] || raw;
  return id === raw ? event : { ...event, track_id: id };
};

export function enqueueEvent(queue, event, limit = MAX_QUEUED_EVENTS) {
  const previous = Array.isArray(queue) ? queue : [];
  if (!event?.idempotency_key) return previous.slice(-limit);
  const withoutDuplicate = previous.filter(
    (item) => item?.idempotency_key !== event.idempotency_key,
  );
  return [...withoutDuplicate, event].slice(-limit);
}

export function eventBatch(queue, limit = EVENT_BATCH_SIZE) {
  return (Array.isArray(queue) ? queue : [])
    .filter((event) => event?.idempotency_key && event?.type)
    .slice(0, limit)
    .map(canonicalEvent);
}

export function acknowledgeEvents(queue, delivered) {
  const ids = new Set(
    (Array.isArray(delivered) ? delivered : []).map(
      (event) => event?.idempotency_key,
    ),
  );
  return (Array.isArray(queue) ? queue : []).filter(
    (event) => !ids.has(event?.idempotency_key),
  );
}

export const eventQueueKey = (userId) =>
  `mixora-ui:event-queue:${String(userId || "guest")}`;
