import { useEffect, useRef, useState } from "react";
import { RotateCw, X } from "lucide-react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { SearchAddon } from "@xterm/addon-search";
import { Events } from "@wailsio/runtime";
import { TerminalService } from "../../bindings/github.com/0x666c6f/safe-agentic/app/internal/svc";
import { errText } from "../types";
import "@xterm/xterm/css/xterm.css";

const b64ToBytes = (b64: string) => Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
// Pinned alpha delivers event.data raw, no array wrapping (see App.tsx).
const unwrap = (e: any) => e?.data;

export function TerminalPane({ container }: { container: string }) {
  const ref = useRef<HTMLDivElement>(null);
  const searchRef = useRef<SearchAddon | null>(null);
  const xtermRef = useRef<Terminal | null>(null);
  const [error, setError] = useState("");
  const [attempt, setAttempt] = useState(0);
  const [searchOpen, setSearchOpen] = useState(false);
  const [query, setQuery] = useState("");

  useEffect(() => {
    if (!ref.current) return;
    setError("");
    const xterm = new Terminal({ fontSize: 13, fontFamily: "Menlo, monospace", scrollback: 10000 });
    xtermRef.current = xterm;
    const fit = new FitAddon();
    const search = new SearchAddon();
    searchRef.current = search;
    xterm.loadAddon(fit);
    xterm.loadAddon(search);
    xterm.open(ref.current);
    // No WebglAddon: its glyph atlas corrupts in WKWebView (red blocks,
    // smeared glyphs). The DOM renderer is correct and fast enough here.
    fit.fit();
    xterm.writeln(`\x1b[90mattaching to ${container}…\x1b[0m`);

    // ⌘F opens search while the terminal has focus.
    xterm.attachCustomKeyEventHandler((e) => {
      if (e.type === "keydown" && e.metaKey && e.key === "f") {
        setSearchOpen(true);
        return false;
      }
      return true;
    });

    let id: string | null = null;
    let offData = () => {};
    let offExit = () => {};
    let disposed = false;

    // Open the PTY AT the fitted xterm size so tmux attaches at the right
    // dimensions immediately (SIGWINCH from a later resize is unreliable
    // through the relay — see the Go side).
    TerminalService.Open(container, xterm.cols, xterm.rows)
      .then((tid: string) => {
        if (disposed) { TerminalService.Close(tid); return; }
        id = tid;
        // Reconcile: the ResizeObserver may have re-fitted the grid to a
        // narrower size while Open was in flight (IPC round-trip) but couldn't
        // resize the PTY yet (id was still null). Push the settled grid size
        // now so the agent's terminal width matches what's actually rendered —
        // otherwise a stale wider size leaks through and status lines overflow.
        fit.fit();
        TerminalService.Resize(tid, xterm.cols, xterm.rows);
        offData = Events.On(`term:data:${tid}`, (e: any) => xterm.write(b64ToBytes(unwrap(e))));
        offExit = Events.On(`term:exit:${tid}`, () =>
          xterm.writeln("\r\n\x1b[33m[disconnected — press ⟳ Reattach]\x1b[0m"));
        xterm.focus();
      })
      .catch((e: unknown) => setError(errText(`attach ${container}`, e)));

    const onData = xterm.onData((d) => { if (id) TerminalService.Write(id, d); });
    const ro = new ResizeObserver(() => {
      fit.fit();
      if (id) TerminalService.Resize(id, xterm.cols, xterm.rows);
    });
    ro.observe(ref.current);

    return () => {
      disposed = true;
      ro.disconnect();
      onData.dispose();
      offData(); offExit();
      if (id) TerminalService.Close(id);
      xterm.dispose();
      xtermRef.current = null;
    };
  }, [container, attempt]);

  return (
    <div className="relative h-full w-full">
      <div ref={ref} className="h-full w-full" />
      {searchOpen && (
        <div className="absolute right-2 top-2 z-10 flex items-center gap-1 rounded bg-neutral-800 p-1 shadow-lg">
          <input
            autoFocus
            className="input w-48 text-xs"
            placeholder="search (Enter/⇧Enter)"
            value={query}
            onChange={(e) => { setQuery(e.target.value); searchRef.current?.findNext(e.target.value); }}
            onKeyDown={(e) => {
              if (e.key === "Enter" && e.shiftKey) searchRef.current?.findPrevious(query);
              else if (e.key === "Enter") searchRef.current?.findNext(query);
              else if (e.key === "Escape") { setSearchOpen(false); xtermRef.current?.focus(); }
            }}
          />
          <button className="btn" onClick={() => { setSearchOpen(false); xtermRef.current?.focus(); }}><X className="h-3.5 w-3.5" /></button>
        </div>
      )}
      {error && (
        <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 bg-neutral-950/90 p-6">
          <pre className="max-w-xl whitespace-pre-wrap rounded border border-red-900 bg-red-950/40 p-3 text-sm text-red-200">{error}</pre>
          <button className="btn" onClick={() => setAttempt((n) => n + 1)}><RotateCw className="h-3.5 w-3.5" /> Reattach</button>
        </div>
      )}
      {!searchOpen && (
        <button
          className="btn absolute right-2 top-2 opacity-60 hover:opacity-100"
          title="Reattach"
          onClick={() => setAttempt((n) => n + 1)}
        ><RotateCw className="h-3.5 w-3.5" /></button>
      )}
    </div>
  );
}
