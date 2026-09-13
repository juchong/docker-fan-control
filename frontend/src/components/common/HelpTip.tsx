import { useRef, useState, type CSSProperties } from 'react';
import { HelpCircle } from 'lucide-react';

// A small "(?)" help affordance with an accessible tooltip. The tooltip is
// rendered with position: fixed (computed from the trigger's rect) so it is
// never clipped by a scrolling modal body, and flips above the trigger when
// near the bottom of the viewport.
export function HelpTip({ text, label }: { text: string; label?: string }) {
  const btnRef = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(false);
  const [style, setStyle] = useState<CSSProperties>({});

  const show = () => {
    const r = btnRef.current?.getBoundingClientRect();
    if (!r) return;
    const below = r.top < window.innerHeight / 2;
    const left = Math.min(Math.max(r.left, 8), window.innerWidth - 288);
    setStyle(
      below
        ? { position: 'fixed', top: r.bottom + 6, left }
        : { position: 'fixed', top: r.top - 6, left, transform: 'translateY(-100%)' }
    );
    setOpen(true);
  };
  const hide = () => setOpen(false);

  return (
    <span className="inline-flex">
      <button
        ref={btnRef}
        type="button"
        aria-label={label ? `Help: ${label}` : 'Help'}
        className="text-muted-2 hover:text-primary-400 focus:outline-none focus:ring-2 focus:ring-primary-500 rounded-full leading-none"
        onMouseEnter={show}
        onMouseLeave={hide}
        onFocus={show}
        onBlur={hide}
        onClick={(e) => {
          e.preventDefault();
          open ? hide() : show();
        }}
        onKeyDown={(e) => {
          if (e.key === 'Escape') hide();
        }}
      >
        <HelpCircle className="w-3.5 h-3.5" />
      </button>
      {open && (
        <span
          role="tooltip"
          style={{ ...style, maxWidth: 280 }}
          className="z-[70] block p-2 text-xs leading-snug rounded-lg bg-surface border border-line shadow-lg text-fg-2 pointer-events-none"
        >
          {text}
        </span>
      )}
    </span>
  );
}
