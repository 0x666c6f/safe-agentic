import { useStore } from "../store";

export function Toasts() {
  const { toasts, dismissToast } = useStore();
  return (
    <div className="fixed bottom-4 right-4 z-50 flex flex-col gap-2">
      {toasts.map((t) => (
        <div key={t.id} className="max-w-xl whitespace-pre-wrap rounded bg-neutral-800 px-4 py-2 text-sm text-neutral-100 shadow-lg">
          {t.text}
          <button className="ml-3 text-neutral-400" onClick={() => dismissToast(t.id)}>✕</button>
        </div>
      ))}
    </div>
  );
}
