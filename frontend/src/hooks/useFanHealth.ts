import { useCallback, useEffect, useState } from 'react';
import { Monitoring } from '../types/monitoring';
import { FanStatus } from '../types/fan';

export type FanState = 'running' | 'idle' | 'failed';

const PERSIST_KEY = 'fan-control-fan-everspun';
const PERSIST_INTERVAL_MS = 5000;

function load(): Record<number, boolean> {
  try {
    const raw = localStorage.getItem(PERSIST_KEY);
    if (!raw) return {};
    const parsed = JSON.parse(raw);
    return parsed && typeof parsed === 'object' ? parsed : {};
  } catch {
    return {};
  }
}

// Which fans have ever reported RPM > 0. Persisted so "a fan that used to spin
// has stopped" survives page refreshes.
let everSpun: Record<number, boolean> = load();
let lastPersisted = 0;
const listeners = new Set<() => void>();

function persist() {
  const now = Date.now();
  if (now - lastPersisted < PERSIST_INTERVAL_MS) return;
  lastPersisted = now;
  try {
    localStorage.setItem(PERSIST_KEY, JSON.stringify(everSpun));
  } catch {
    // ignore
  }
}

export function recordFanHealth(m: Monitoring | null) {
  if (!m?.fans) return;
  let changed = false;
  const next = { ...everSpun };
  for (const fan of m.fans) {
    if ((fan.current_rpm ?? 0) > 0 && !next[fan.id]) {
      next[fan.id] = true;
      changed = true;
    }
  }
  if (changed) {
    everSpun = next;
    persist();
    listeners.forEach((l) => l());
  }
}

// Classify a fan:
//   running — reporting RPM.
//   failed  — has spun before and is being commanded to move (duty > 0) but
//             now reports 0 RPM → likely a stalled/failed fan.
//   idle    — 0 RPM and either never seen spinning or intentionally at 0 duty.
export function fanState(fan: FanStatus, spun: Record<number, boolean> = everSpun): FanState {
  if ((fan.current_rpm ?? 0) > 0) return 'running';
  const commanded = (fan.current_duty ?? 0) > 0;
  if (spun[fan.id] && commanded) return 'failed';
  return 'idle';
}

// Returns a classifier bound to the current everSpun snapshot.
export function useFanHealth(): (fan: FanStatus) => FanState {
  const [snap, setSnap] = useState<Record<number, boolean>>(everSpun);
  useEffect(() => {
    const listener = () => setSnap(everSpun);
    listeners.add(listener);
    listener();
    return () => {
      listeners.delete(listener);
    };
  }, []);
  return useCallback((fan: FanStatus) => fanState(fan, snap), [snap]);
}
