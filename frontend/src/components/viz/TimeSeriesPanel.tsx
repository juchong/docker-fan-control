import { useMemo } from 'react';
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from 'recharts';
import { MetricSample } from '../../hooks/useMetricsHistory';

type Variant = 'temp' | 'load';
type DutyAxis = 'duty' | 'pct';

interface SeriesDef {
  key: keyof Omit<MetricSample, 't' | 'duties'>;
  name: string;
  color: string;
  axis: 'temp' | 'duty' | 'pct';
  dashed?: boolean;
}

// Temperature / load lines (solid). Fan duty is handled separately so it can be
// split into one line per active profile.
const TEMP_SERIES: SeriesDef[] = [
  { key: 'gpu', name: 'GPU', color: 'rgb(var(--c-danger))', axis: 'temp' },
  { key: 'cpu', name: 'CPU', color: 'rgb(var(--c-info))', axis: 'temp' },
  { key: 'drive', name: 'Drive', color: '#22d3ee', axis: 'temp' },
  { key: 'board', name: 'Board', color: 'rgb(var(--c-warn))', axis: 'temp' },
];

const LOAD_SERIES: SeriesDef[] = [
  { key: 'gpuLoad', name: 'GPU load', color: 'rgb(var(--c-info))', axis: 'pct' },
  { key: 'cpuLoad', name: 'CPU load', color: '#a78bfa', axis: 'pct' },
];

// Distinct dashed colours for the per-profile duty lines, chosen to stay clear
// of the solid temp/load colours above.
const PROFILE_COLORS = [
  'rgb(var(--c-ok))', // green
  '#f472b6', // pink
  '#facc15', // yellow
  '#fb923c', // orange
  '#c084fc', // purple
  '#a3e635', // lime
];

const AGG_DUTY_COLOR = 'rgb(var(--c-ok))';

type Row = MetricSample & { time: string };

function fmtTime(t: number): string {
  const d = new Date(t);
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

// Time-series of either temperatures (dual axis: °C + fan-duty %) or compute
// load (single % axis shared with fan duty), backed by the client metric
// history ring buffer. Fan duty is drawn as one dashed line per active profile
// (the duty each profile's curve computes); with no active profiles — or an
// older backend that doesn't report per-profile duty — a single aggregate
// "Fan duty" line (max across fans) is shown instead. Useful for seeing how
// each profile responds to heat / load.
export default function TimeSeriesPanel({
  history,
  variant = 'temp',
}: {
  history: MetricSample[];
  variant?: Variant;
}) {
  const data = useMemo<Row[]>(() => history.map((s) => ({ ...s, time: fmtTime(s.t) })), [history]);

  const baseAll = variant === 'load' ? LOAD_SERIES : TEMP_SERIES;
  // Only plot base series that actually have data.
  const activeBase = useMemo(
    () => baseAll.filter((s) => history.some((h) => h[s.key] != null)),
    [baseAll, history]
  );

  // Distinct profile names seen anywhere in the buffer → one duty line each.
  const profileNames = useMemo(() => {
    const set = new Set<string>();
    for (const s of history) {
      if (s.duties) for (const k of Object.keys(s.duties)) set.add(k);
    }
    return [...set];
  }, [history]);

  const dutyAxis: DutyAxis = variant === 'load' ? 'pct' : 'duty';
  const hasProfiles = profileNames.length > 0;
  const showAggDuty = !hasProfiles && history.some((h) => h.duty != null);

  if (history.length < 3) {
    return (
      <div className="flex flex-col items-center justify-center h-64 gap-2 text-sm text-muted">
        <span
          className="inline-block w-5 h-5 border-2 border-line border-t-primary-500 rounded-full animate-spin"
          aria-hidden="true"
        />
        Collecting live data… the chart fills in over the next few seconds.
      </div>
    );
  }

  const axisColor = 'rgb(var(--c-muted))';
  const gridColor = 'rgb(var(--c-line) / 0.6)';

  return (
    <div className="h-64 w-full">
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={data} margin={{ top: 8, right: 8, left: -8, bottom: 0 }}>
          <CartesianGrid strokeDasharray="3 3" stroke={gridColor} />
          <XAxis
            dataKey="time"
            tick={{ fill: axisColor, fontSize: 11 }}
            stroke={axisColor}
            minTickGap={48}
          />

          {variant === 'temp' ? (
            <>
              <YAxis
                yAxisId="temp"
                tick={{ fill: axisColor, fontSize: 11 }}
                stroke={axisColor}
                width={40}
                allowDecimals={false}
                // Round to 10°C steps with padding so the axis holds steady.
                domain={[
                  (min: number) => Math.max(0, Math.floor((min - 5) / 10) * 10),
                  (max: number) => Math.ceil((max + 5) / 10) * 10,
                ]}
                unit="°"
              />
              <YAxis
                yAxisId="duty"
                orientation="right"
                tick={{ fill: axisColor, fontSize: 11 }}
                stroke={axisColor}
                width={40}
                domain={[0, 100]}
                unit="%"
              />
            </>
          ) : (
            <YAxis
              yAxisId="pct"
              tick={{ fill: axisColor, fontSize: 11 }}
              stroke={axisColor}
              width={40}
              domain={[0, 100]}
              unit="%"
            />
          )}

          <Tooltip
            contentStyle={{
              background: 'rgb(var(--c-surface))',
              border: '1px solid rgb(var(--c-line))',
              borderRadius: 8,
              color: 'rgb(var(--c-fg))',
              fontSize: 12,
            }}
            labelStyle={{ color: 'rgb(var(--c-muted))' }}
          />
          <Legend wrapperStyle={{ fontSize: 12 }} />

          {/* Temperature / load lines */}
          {activeBase.map((s) => (
            <Line
              key={s.key}
              yAxisId={s.axis}
              type="monotone"
              dataKey={s.key}
              name={s.name}
              stroke={s.color}
              strokeWidth={2}
              dot={false}
              isAnimationActive={false}
              connectNulls
            />
          ))}

          {/* Fan duty: one dashed line per active profile, else the aggregate. */}
          {hasProfiles
            ? profileNames.map((name, i) => (
                <Line
                  key={`p:${name}`}
                  yAxisId={dutyAxis}
                  type="monotone"
                  dataKey={(row: Row) => row.duties?.[name] ?? null}
                  name={name}
                  stroke={PROFILE_COLORS[i % PROFILE_COLORS.length]}
                  strokeWidth={2}
                  strokeDasharray="5 4"
                  dot={false}
                  isAnimationActive={false}
                  connectNulls
                />
              ))
            : showAggDuty && (
                <Line
                  yAxisId={dutyAxis}
                  type="monotone"
                  dataKey="duty"
                  name="Fan duty"
                  stroke={AGG_DUTY_COLOR}
                  strokeWidth={2}
                  strokeDasharray="5 4"
                  dot={false}
                  isAnimationActive={false}
                  connectNulls
                />
              )}
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}
