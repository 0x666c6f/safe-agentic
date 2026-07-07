import { Check, CircleX, X } from "lucide-react";
import { useStore } from "../store";

const STYLE = {
  pending: "border-neutral-600 bg-neutral-800 text-neutral-100",
  ok: "border-green-800 bg-neutral-800 text-neutral-100",
  error: "border-red-800 bg-red-950/60 text-red-100",
} as const;

export function Toasts() {
  const { toasts, dismissToast } = useStore();
  return (
    <div className="fixed bottom-4 right-4 z-50 flex flex-col gap-2">
      {toasts.map((t) => (
        <div
          key={t.id}
          className={`flex max-w-xl items-start gap-2 whitespace-pre-wrap rounded border px-4 py-2 text-sm shadow-lg ${STYLE[t.kind]}`}
        >
          <span className="mt-0.5 shrink-0">
            {t.kind === "pending"
              ? <span className="inline-block h-3 w-3 animate-spin rounded-full border-2 border-neutral-500 border-t-neutral-200" />
              : t.kind === "ok" ? <Check className="h-3.5 w-3.5 text-green-400" />
              : <CircleX className="h-3.5 w-3.5 text-red-400" />}
          </span>
          <span className="min-w-0 flex-1">{t.text}</span>
          <button className="shrink-0 text-neutral-400 hover:text-neutral-200" onClick={() => dismissToast(t.id)}><X className="h-3.5 w-3.5" /></button>
        </div>
      ))}
    </div>
  );
}
