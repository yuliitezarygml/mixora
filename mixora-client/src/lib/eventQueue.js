const MAX_QUEUED_EVENTS = 500;
const EVENT_BATCH_SIZE = 100;

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
    .slice(0, limit);
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
