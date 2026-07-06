// Wails alpha dispatches every event on its own goroutine, so term:data
// chunks can arrive out of order under streaming load — interleaved escape
// sequences garble the terminal. The Go side prefixes each chunk with a
// sequence number ("42|<base64>"); this reassembles strict order. The first
// seen seq becomes the baseline (panes can attach mid-stream), and a gap is
// skipped only after `stallMs` without the missing chunk showing up.
export function orderedStream(write: (b64: string) => void, stallMs = 250) {
  let next = 0; // 0 = adopt the first arrival as baseline
  const pending = new Map<number, string>();
  let timer: ReturnType<typeof setTimeout> | null = null;
  const clear = () => { if (timer !== null) { clearTimeout(timer); timer = null; } };
  const flush = () => {
    while (pending.has(next)) {
      write(pending.get(next)!);
      pending.delete(next);
      next++;
    }
    clear();
    if (pending.size) {
      timer = setTimeout(() => { next = Math.min(...pending.keys()); flush(); }, stallMs);
    }
  };
  return {
    push(raw: string) {
      const i = raw.indexOf("|");
      const seq = Number(raw.slice(0, i));
      if (i < 0 || !Number.isFinite(seq)) { write(raw); return; } // unnumbered fallback
      if (next === 0) next = seq;
      if (seq < next) return; // stale duplicate
      pending.set(seq, raw.slice(i + 1));
      flush();
    },
    dispose: clear,
  };
}
