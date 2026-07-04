import { useEffect, useRef } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebglAddon } from "@xterm/addon-webgl";
import { Events } from "@wailsio/runtime";
import { TerminalService } from "../../bindings/github.com/0x666c6f/safe-agentic/app/internal/svc";
import { useStore } from "../store";
import "@xterm/xterm/css/xterm.css";

const b64ToBytes = (b64: string) => Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
// See App.tsx: pinned alpha delivers event.data raw, no array wrapping.
const unwrap = (e: any) => e?.data;

export function TerminalPane({ container }: { container: string }) {
  const ref = useRef<HTMLDivElement>(null);
  const toast = useStore((s) => s.toast);

  useEffect(() => {
    if (!ref.current) return;
    const xterm = new Terminal({ fontSize: 13, fontFamily: "Menlo, monospace", scrollback: 10000 });
    const fit = new FitAddon();
    xterm.loadAddon(fit);
    xterm.open(ref.current);
    try { xterm.loadAddon(new WebglAddon()); } catch { /* canvas fallback */ }
    fit.fit();

    let id: string | null = null;
    let offData = () => {};
    let offExit = () => {};
    let disposed = false;

    TerminalService.Open(container)
      .then((tid: string) => {
        if (disposed) { TerminalService.Close(tid); return; }
        id = tid;
        offData = Events.On(`term:data:${tid}`, (e: any) => xterm.write(b64ToBytes(unwrap(e))));
        offExit = Events.On(`term:exit:${tid}`, () => xterm.writeln("\r\n[disconnected — reopen tab to reattach]"));
        TerminalService.Resize(tid, xterm.cols, xterm.rows);
      })
      .catch((e: unknown) => toast(String(e)));

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
    };
  }, [container]);

  return <div ref={ref} className="h-full w-full" />;
}
