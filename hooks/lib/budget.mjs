/** One monotonic deadline, shared by all work for a single hook event. */
export function createEventBudget(timeoutMs = 5000) {
  const controller = new AbortController();
  const deadline = performance.now() + Math.max(0, timeoutMs);
  const expired = () => Object.assign(new Error("Agent-Team event deadline exceeded."), { code: "EVENT_DEADLINE" });
  const remaining = () => Math.max(0, Math.ceil(deadline - performance.now()));
  const timer = setTimeout(() => controller.abort(expired()), Math.max(0, timeoutMs));
  function check() {
    if (controller.signal.aborted || remaining() === 0) throw expired();
  }
  return {
    signal: controller.signal,
    remaining,
    timeout(cap) { check(); return Math.min(cap, remaining()); },
    async run(action) {
      check();
      let abort;
      try {
        return await Promise.race([
          Promise.resolve().then(action),
          new Promise((_, reject) => {
            abort = () => reject(expired());
            controller.signal.addEventListener("abort", abort, { once: true });
          }),
        ]);
      } finally {
        controller.signal.removeEventListener("abort", abort);
      }
    },
    close() { clearTimeout(timer); },
  };
}
