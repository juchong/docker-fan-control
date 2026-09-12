interface RadialGaugeProps {
  /** 0–100 */
  value: number;
  label?: string;
  sublabel?: string;
  /** Semantic tone for the arc fill. */
  tone?: 'accent' | 'ok' | 'warn' | 'danger' | 'info';
  size?: number;
  /** Show the numeric value in the center (default true). */
  showValue?: boolean;
  unit?: string;
}

const TONE_VAR: Record<NonNullable<RadialGaugeProps['tone']>, string> = {
  accent: '14 165 233', // sky-500
  ok: 'var(--c-ok)',
  warn: 'var(--c-warn)',
  danger: 'var(--c-danger)',
  info: 'var(--c-info)',
};

// A 270° radial gauge rendered as SVG (no chart library needed). The gap sits
// at the bottom; the arc fills clockwise with the given value.
export function RadialGauge({
  value,
  label,
  sublabel,
  tone = 'accent',
  size = 120,
  showValue = true,
  unit = '%',
}: RadialGaugeProps) {
  const clamped = Math.max(0, Math.min(100, value));
  const r = 40;
  const c = 2 * Math.PI * r;
  const sweep = 0.75; // 270°
  const track = sweep * c;
  const fill = (clamped / 100) * sweep * c;
  const toneColor = `rgb(${TONE_VAR[tone]})`;

  return (
    <div className="inline-flex flex-col items-center" style={{ width: size }}>
      <div className="relative" style={{ width: size, height: size }}>
        <svg viewBox="0 0 100 100" className="w-full h-full" role="img" aria-label={`${label ?? 'value'}: ${Math.round(clamped)}${unit}`}>
          <g transform="rotate(135 50 50)">
            <circle
              cx="50"
              cy="50"
              r={r}
              fill="none"
              stroke="rgb(var(--c-surface-3))"
              strokeWidth="8"
              strokeLinecap="round"
              strokeDasharray={`${track} ${c}`}
            />
            <circle
              cx="50"
              cy="50"
              r={r}
              fill="none"
              stroke={toneColor}
              strokeWidth="8"
              strokeLinecap="round"
              strokeDasharray={`${fill} ${c}`}
              style={{ transition: 'stroke-dasharray 0.5s ease-out' }}
            />
          </g>
        </svg>
        {showValue && (
          <div className="absolute inset-0 flex flex-col items-center justify-center">
            <span className="text-xl font-bold text-fg tabular-nums">{Math.round(clamped)}</span>
            <span className="text-xs text-muted">{unit}</span>
          </div>
        )}
      </div>
      {label && <span className="mt-1 text-sm font-medium text-fg-2 text-center">{label}</span>}
      {sublabel && <span className="text-xs text-muted text-center">{sublabel}</span>}
    </div>
  );
}
