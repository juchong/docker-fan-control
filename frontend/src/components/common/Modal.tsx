import React, { useEffect, useId, useRef } from 'react';
import { X } from 'lucide-react';

interface ModalProps {
  isOpen: boolean;
  onClose: () => void;
  title: string;
  children: React.ReactNode;
  size?: 'sm' | 'md' | 'lg' | 'xl';
}

const FOCUSABLE =
  'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])';

export function Modal({ isOpen, onClose, title, children, size = 'md' }: ModalProps) {
  const panelRef = useRef<HTMLDivElement>(null);
  const titleId = useId();

  // Keep the latest onClose in a ref so the focus/key effect below can call it
  // without listing onClose as a dependency. The parent often passes a fresh
  // onClose on every render (and the editor re-renders on each live-metrics
  // frame); if the effect depended on onClose it would re-run constantly and
  // keep yanking focus back into the dialog while the user is typing.
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;

  useEffect(() => {
    if (!isOpen) return;

    const previouslyFocused = document.activeElement as HTMLElement | null;
    document.body.style.overflow = 'hidden';

    // Move focus into the dialog — once, when it opens.
    const panel = panelRef.current;
    if (panel) {
      const focusable = panel.querySelectorAll<HTMLElement>(FOCUSABLE);
      (focusable[0] ?? panel).focus();
    }

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onCloseRef.current();
        return;
      }
      if (e.key !== 'Tab') return;

      const p = panelRef.current;
      if (!p) return;
      const focusable = Array.from(p.querySelectorAll<HTMLElement>(FOCUSABLE));
      if (focusable.length === 0) {
        e.preventDefault();
        return;
      }
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      const active = document.activeElement;

      if (e.shiftKey && (active === first || active === p)) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && active === last) {
        e.preventDefault();
        first.focus();
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    return () => {
      document.removeEventListener('keydown', handleKeyDown);
      document.body.style.overflow = 'unset';
      previouslyFocused?.focus?.();
    };
  }, [isOpen]);

  if (!isOpen) return null;

  const sizeStyles = {
    sm: 'max-w-md',
    md: 'max-w-lg',
    lg: 'max-w-2xl',
    xl: 'max-w-4xl',
  };

  return (
    <div className="fixed inset-0 z-50 overflow-y-auto">
      <div className="flex min-h-full items-center justify-center p-4">
        {/* Backdrop */}
        <div className="fixed inset-0 bg-black/50 transition-opacity" onClick={onClose} aria-hidden="true" />

        {/* Dialog */}
        <div
          ref={panelRef}
          role="dialog"
          aria-modal="true"
          aria-labelledby={titleId}
          tabIndex={-1}
          className={`relative w-full ${sizeStyles[size]} bg-surface rounded-lg shadow-xl border border-line animate-fade-in focus:outline-none`}
        >
          {/* Header */}
          <div className="flex items-center justify-between px-4 py-3 border-b border-line">
            <h3 id={titleId} className="text-lg font-semibold text-fg-2">
              {title}
            </h3>
            <button
              onClick={onClose}
              className="p-1 text-muted hover:text-fg-2 hover:bg-surface-2 rounded transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500"
              aria-label="Close dialog"
            >
              <X className="w-5 h-5" />
            </button>
          </div>

          {/* Content */}
          <div className="p-4">{children}</div>
        </div>
      </div>
    </div>
  );
}
