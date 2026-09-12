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

interface SeriesDef {
  key: keyof Omit<MetricSample, 't'>;
  name: string;
  color: string;
  axis: 'temp' | 'duty';
  dashed?: boolean;
}

const SERIES: SeriesDef[] = [
  { key: 'gpu', name: 'GPU', color: 'rgb(var(--c-danger))', axis: 'temp' },
  { key: 'cpu', name: 'CPU', color: 'rgb(var(--c-info))', axis: 'temp' },
  { key: 'drive', name: 'Drive', color: '#22d3ee', axis: 'temp' },
  { key: 'board', name: 'Board', color: 'rgb(var(--c-warn))', axis: 'temp' },
  { key: 'duty', name: 'Fan duty', color: 'rgb(var(--c-ok))', axis: 'duty', dashed: true },
];

function fmtTime(t: number): string {
  const d = new Date(t);
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

// Temperature + fan-duty time series, backed by the client metric history
// ring buffer. Essential for tuning PID/curve response.
export default function TimeSeriesPanel({ history }: { history: MetricSample[] }) {
  const data = useMemo(
    () => history.map((s) => ({ ...s, time: fmtTime(s.t) })),
    [history]
  );

  // Only plot series that actually have data.
  const activeSeries = useMemo(
    () => SERIES.filter((s) => history.some((h) => h[s.key] != null)),
    [history]
  );

  if (history.length < 2) {
    return (
      <div className="flex items-center justify-center h-64 text-sm text-muted">
        Collecting data… the chart appears after a few samples.
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
          <YAxis
            yAxisId="temp"
            tick={{ fill: axisColor, fontSize: 11 }}
            stroke={axisColor}
            width={40}
            domain={['auto', 'auto']}
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
          {activeSeries.map((s) => (
            <Line
              key={s.key}
              yAxisId={s.axis}
              type="monotone"
              dataKey={s.key}
              name={s.name}
              stroke={s.color}
              strokeWidth={2}
              strokeDasharray={s.dashed ? '5 4' : undefined}
              dot={false}
              isAnimationActive={false}
              connectNulls
            />
          ))}
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}
