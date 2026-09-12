import { useEffect, useState } from 'react';
import { useMonitoring } from './useMonitoring';
import { Monitoring } from '../types/monitoring';

export interface MetricSample {
  t: number; // epoch ms
  gpu: number | null;
  cpu: number | null;
  drive: number | null;
  board: number | null;
  duty: number | null; // max fan duty %
}

const MAX_POINTS = 240; // ~6 min at a 1.5s cadence
const MIN_INTERVAL_MS = 1500; // don't record faster than this
const MAX_AGE_MS = 20 * 60 * 1000; // drop persisted samples older than this on load
const PERSIST_KEY = 'fan-control-metrics-history';
const PERSIST_INTERVAL_MS = 5000; // throttle localStorage writes

function loadBuffer(): MetricSample[] {
  try {
    const raw = localStorage.getItem(PERSIST_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    const cutoff = Date.now() - MAX_AGE_MS;
    return parsed
      .filter((s): s is MetricSample => s && typeof s.t === 'number' && s.t >= cutoff)
      .slice(-MAX_POINTS);
  } catch {
    return [];
  }
}

// Module-level ring buffer, seeded from localStorage so history survives a full
// page refresh instead of starting empty.
let buffer: MetricSample[] = loadBuffer();
let lastRecorded = 0;
let lastPersisted = 0;
const listeners = new Set<() => void>();

function persist() {
  const now = Date.now();
  if (now - lastPersisted < PERSIST_INTERVAL_MS) return;
  lastPersisted = now;
  try {
    localStorage.setItem(PERSIST_KEY, JSON.stringify(buffer));
  } catch {
    // localStorage unavailable / quota — history just won't survive refresh
  }
}

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
  persist();
  listeners.forEach((l) => l());
}

// Drives the ring buffer from the shared monitoring feed. Mount this ONCE high
// in the authenticated app shell so history keeps accruing on every page — not
// only while the dashboard chart is on screen.
export function useMetricsRecorder() {
  const { data } = useMonitoring();
  useEffect(() => {
    recordSample(data);
  }, [data]);
}

// Read-only subscription to the accumulated history.
export function useMetricsHistory(): MetricSample[] {
  const [snapshot, setSnapshot] = useState<MetricSample[]>(buffer);

  useEffect(() => {
    const listener = () => setSnapshot(buffer);
    listeners.add(listener);
    listener(); // sync in case the buffer advanced between render and subscribe
    return () => {
      listeners.delete(listener);
    };
  }, []);

  return snapshot;
}
