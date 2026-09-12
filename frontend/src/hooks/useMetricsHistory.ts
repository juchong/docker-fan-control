import { useEffect, useState } from 'react';
import { Monitoring } from '../types/monitoring';

export interface MetricSample {
  t: number; // epoch ms
  gpu: number | null;
  cpu: number | null;
  drive: number | null;
  board: number | null;
  duty: number | null; // max fan duty %
}

const MAX_POINTS = 240; // ~8 min at a 2s cadence
const MIN_INTERVAL_MS = 1500; // don't record faster than this

// Module-level ring buffer so history survives route changes within a session.
let buffer: MetricSample[] = [];
let lastRecorded = 0;
const listeners = new Set<() => void>();

function max(nums: Array<number | undefined | null>): number | null {
  const valid = nums.filter((n): n is number => typeof n === 'number' && !Number.isNaN(n));
  return valid.length ? Math.max(...valid) : null;
}

export function recordSample(m: Monitoring | null) {
  if (!m) return;
  const now = Date.now();
  if (now - lastRecorded < MIN_INTERVAL_MS) return;
  lastRecorded = now;

  const sample: MetricSample = {
    t: now,
    gpu: max((m.gpus ?? []).map((g) => g.temperature)),
    cpu: max((m.system?.cpu_packages ?? []).map((c) => c.temperature)),
    drive: max((m.system?.drives ?? []).map((d) => d.temperature)),
    board: max((m.system?.board_temps ?? []).map((b) => b.temperature)),
    duty: max((m.fans ?? []).map((f) => f.current_duty)),
  };

  buffer = [...buffer, sample].slice(-MAX_POINTS);
  listeners.forEach((l) => l());
}

// Records samples from the provided monitoring frame and returns the history.
export function useMetricsHistory(data: Monitoring | null): MetricSample[] {
  const [snapshot, setSnapshot] = useState<MetricSample[]>(buffer);

  useEffect(() => {
    const listener = () => setSnapshot(buffer);
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
    };
  }, []);

  useEffect(() => {
    recordSample(data);
  }, [data]);

  return snapshot;
}
