import { useEffect, type ReactNode } from "react";
import { stateClass, stateName } from "./api";

export function Drawer({
  title,
  lede,
  onClose,
  children,
}: {
  title: string;
  lede?: string;
  onClose: () => void;
  children: ReactNode;
}) {
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <div className="scrim" onMouseDown={onClose}>
      <aside className="drawer" role="dialog" aria-modal="true" aria-label={title} onMouseDown={(e) => e.stopPropagation()}>
        <h2>{title}</h2>
        {lede ? <p className="hint">{lede}</p> : null}
        {children}
      </aside>
    </div>
  );
}

export function Field({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  return (
    <label className="field">
      <span>{label}</span>
      {children}
    </label>
  );
}

export function Pill({ value }: { value?: string }) {
  return (
    <span className={`pill ${stateClass(value)}`}>
      <i />
      {stateName(value)}
    </span>
  );
}

export function Banner({ message }: { message: string | null }) {
  if (!message) return null;
  return <div className="banner">{message}</div>;
}
