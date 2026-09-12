import React, { createContext, useCallback, useContext, useRef, useState } from 'react';
import { CheckCircle, AlertTriangle, XCircle, Info, X } from 'lucide-react';

export type ToastKind = 'success' | 'error' | 'warning' | 'info';

interface Toast {
  id: number;
  kind: ToastKind;
  message: string;
}

interface ToastContextValue {
  show: (kind: ToastKind, message: string) => void;
  success: (message: string) => void;
  error: (message: string) => void;
  warning: (message: string) => void;
  info: (message: string) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

const KIND_META: Record<ToastKind, { icon: React.ComponentType<{ className?: string }>; cls: string; label: string }> = {
  success: { icon: CheckCircle, cls: 'text-ok border-ok/40', label: 'Success' },
  error: { icon: XCircle, cls: 'text-danger border-danger/40', label: 'Error' },
  warning: { icon: AlertTriangle, cls: 'text-warn border-warn/40', label: 'Warning' },
  info: { icon: Info, cls: 'text-info border-info/40', label: 'Info' },
};

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const nextId = useRef(1);

  const dismiss = useCallback((id: number) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  const show = useCallback((kind: ToastKind, message: string) => {
    const id = nextId.current++;
    setToasts((prev) => [...prev, { id, kind, message }]);
    window.setTimeout(() => dismiss(id), 5000);
  }, [dismiss]);

  const value: ToastContextValue = {
    show,
    success: (m) => show('success', m),
    error: (m) => show('error', m),
    warning: (m) => show('warning', m),
    info: (m) => show('info', m),
  };

  return (
    <ToastContext.Provider value={value}>
      {children}
      {/* Live region: assertive for errors, polite otherwise (handled per-toast) */}
      <div
        className="fixed bottom-4 right-4 z-[60] flex flex-col gap-2 w-80 max-w-[calc(100vw-2rem)]"
        aria-live="polite"
        aria-atomic="false"
      >
        {toasts.map((t) => {
          const meta = KIND_META[t.kind];
          const Icon = meta.icon;
          return (
            <div
              key={t.id}
              role={t.kind === 'error' ? 'alert' : 'status'}
              className={`flex items-start gap-2 p-3 rounded-lg border bg-surface shadow-lg animate-fade-in ${meta.cls}`}
            >
              <Icon className="w-5 h-5 shrink-0 mt-0.5" aria-hidden="true" />
              <span className="sr-only">{meta.label}: </span>
              <p className="flex-1 text-sm text-fg-2 break-words">{t.message}</p>
              <button
                onClick={() => dismiss(t.id)}
                className="shrink-0 text-muted hover:text-fg-2 rounded focus:outline-none focus:ring-2 focus:ring-primary-500"
                aria-label="Dismiss notification"
              >
                <X className="w-4 h-4" />
              </button>
            </div>
          );
        })}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast(): ToastContextValue {
  const ctx = useContext(ToastContext);
  if (!ctx) {
    // Fail soft: a no-op toaster if used outside the provider (e.g. in tests).
    const noop = () => {};
    return { show: noop, success: noop, error: noop, warning: noop, info: noop };
  }
  return ctx;
}
