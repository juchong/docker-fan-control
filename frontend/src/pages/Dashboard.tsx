import { useState, lazy, Suspense } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useMonitoring } from '../hooks/useMonitoring';
import type { DriveMetrics, ThermalInfo, ThermalStatus } from '../types/monitoring';
import { useMetricsHistory } from '../hooks/useMetricsHistory';
import { useFanHealth } from '../hooks/useFanHealth';
import { Card } from '../components/common/Card';
import { Button } from '../components/common/Button';
import { RadialGauge } from '../components/viz/RadialGauge';
import { useToast } from '../components/common/Toast';
import { logsApi, settingsApi } from '../services/api';
import {
  Thermometer,
  Cpu,
  Fan,
  Activity,
  AlertTriangle,
  CheckCircle,
  Clock,
  HardDrive,
  ChevronDown,
  ChevronUp,
  Layers,
  Play,
  Square,
  Cpu as Chip,
} from 'lucide-react';

type Tone = 'info' | 'ok' | 'warn' | 'danger';

function tempTone(temp: number): Tone {
  if (temp < 50) return 'info';
  if (temp < 65) return 'ok';
  if (temp < 80) return 'warn';
  return 'danger';
}

function dutyTone(duty: number): Tone {
  if (duty < 40) return 'info';
  if (duty < 70) return 'ok';
  if (duty < 90) return 'warn';
  return 'danger';
}

const TONE_TEXT: Record<Tone, string> = {
  info: 'text-info',
  ok: 'text-ok',
  warn: 'text-warn',
  danger: 'text-danger',
};

function tempTextClass(temp: number): string {
  return TONE_TEXT[tempTone(temp)];
}

// Device colouring, in order of preference: the controller's per-device thermal
// status (headroom to the device's own limit) when the backend provides it;
// else the device's own thresholds (drives report max/crit); else the generic
// bands. The fallbacks keep an older backend rendering exactly as before.
type ThermalLike = ThermalInfo & { temperature: number; max?: number; crit?: number };

const STATUS_TONE: Record<ThermalStatus, Tone> = { ok: 'ok', warning: 'warn', critical: 'danger' };

function statusTone(status: ThermalStatus, temp: number): Tone {
  // "ok" keeps the calm blue below 50 °C so idle devices don't all turn green.
  if (status === 'ok') return tempTone(temp) === 'info' ? 'info' : 'ok';
  return STATUS_TONE[status];
}

function deviceTone(d: ThermalLike): Tone {
  if (d.status) return statusTone(d.status, d.temperature);
  const t = d.temperature;
  if (d.crit != null && t >= d.crit) return 'danger';
  if (d.max != null && t >= d.max) return 'warn';
  if (d.max != null || d.crit != null) return tempTone(t) === 'info' ? 'info' : 'ok';
  return tempTone(t);
}

// Worst status in a list, for summary cards and section headers; undefined
// when the backend annotated nothing (old backend) so callers fall back.
function worstStatus(items: ThermalInfo[]): ThermalStatus | undefined {
  let worst: ThermalStatus | undefined;
  for (const i of items) {
    if (!i.status) continue;
    if (i.status === 'critical') return 'critical';
    if (i.status === 'warning') worst = 'warning';
    else if (!worst) worst = 'ok';
  }
  return worst;
}

function statusClass(status: ThermalStatus | undefined, temp: number): string {
  return status ? TONE_TEXT[statusTone(status, temp)] : tempTextClass(temp);
}

// "· 12 °C to limit" suffix for a device card; empty when the backend gave none.
function headroomText(d: ThermalInfo): string {
  return d.headroom != null ? ` · ${d.headroom} °C to limit` : '';
}

function limitTooltip(d: ThermalInfo): string[] {
  return d.limit != null ? [`limit ${d.limit} °C (${d.limit_source ?? 'unknown'})`] : [];
}

function driveTooltip(d: DriveMetrics): string {
  const parts = [...limitTooltip(d), ...(d.sensors ?? []).map((s) => `${s.label}: ${s.temperature.toFixed(1)} °C`)];
  if (d.max != null) parts.push(`warning at ${d.max} °C`);
  if (d.crit != null) parts.push(`critical at ${d.crit} °C`);
  if (d.serial) parts.push(`S/N ${d.serial}`);
  if (d.firmware) parts.push(`fw ${d.firmware}`);
  return parts.join(' · ');
}

function formatBytes(bytes: number): string {
  const gb = bytes / (1024 * 1024 * 1024);
  return `${gb.toFixed(1)} GB`;
}

const TimeSeriesPanel = lazy(() => import('../components/viz/TimeSeriesPanel'));

// --- Small presentational helpers -----------------------------------------

function StatCard({
  label,
  value,
  icon: Icon,
  valueClass = 'text-fg',
}: {
  label: string;
  value: React.ReactNode;
  icon: React.ComponentType<{ className?: string }>;
  valueClass?: string;
}) {
  // Two fixed rows — label + icon on top, value below — so the icon always sits
  // top-right aligned with the label, and every card's value starts at the same
  // height. A long value (e.g. the multi-chip driver name) then wraps downward
  // without shifting the icon or breaking alignment with its neighbours.
  return (
    <div className="bg-surface border border-line rounded-lg p-4 flex flex-col gap-2">
      <div className="flex items-start justify-between gap-2">
        <p className="text-sm text-muted truncate min-w-0">{label}</p>
        <div className="p-2 bg-surface-2 rounded-lg shrink-0">
          <Icon className="w-5 h-5 text-muted" />
        </div>
      </div>
      <p className={`text-2xl font-bold leading-tight break-words ${valueClass}`}>{value}</p>
    </div>
  );
}

function CollapsibleSection({
  title,
  count,
  maxTemp,
  status,
  icon: Icon,
  iconColor,
  children,
  defaultExpanded = true,
  maxVisible = 6,
}: {
  title: string;
  count: number;
  maxTemp?: number;
  /** Worst thermal status in the section; colours the max when present. */
  status?: ThermalStatus;
  icon: React.ComponentType<{ className?: string }>;
  iconColor: string;
  children: React.ReactNode[];
  defaultExpanded?: boolean;
  maxVisible?: number;
}) {
  const [expanded, setExpanded] = useState(defaultExpanded);
  const [showAll, setShowAll] = useState(false);

  const visibleChildren = showAll ? children : children.slice(0, maxVisible);
  const hasMore = children.length > maxVisible;

  return (
    <div className="space-y-2">
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center justify-between p-2 hover:bg-surface-2/50 rounded-lg transition-colors"
        aria-expanded={expanded}
      >
        <div className="flex items-center gap-2">
          <Icon className={`w-5 h-5 ${iconColor}`} />
          <span className="font-medium text-fg-2">{title}</span>
          <span className="text-sm text-muted">({count})</span>
          {maxTemp !== undefined && maxTemp > 0 && (
            <span className={`text-sm font-medium ${statusClass(status, maxTemp)}`}>Max: {maxTemp.toFixed(1)}°C</span>
          )}
        </div>
        {expanded ? (
          <ChevronUp className="w-4 h-4 text-muted" />
        ) : (
          <ChevronDown className="w-4 h-4 text-muted" />
        )}
      </button>

      {expanded && (
        <div className="space-y-2">
          {visibleChildren}
          {hasMore && (
            <button
              onClick={() => setShowAll(!showAll)}
              className="w-full text-center text-sm text-muted hover:text-fg-3 py-2"
            >
              {showAll ? 'Show less' : `Show all ${children.length} items`}
            </button>
          )}
        </div>
      )}
    </div>
  );
}

// --- Page ------------------------------------------------------------------

export function Dashboard() {
  const { data: monitoring } = useMonitoring();
  const history = useMetricsHistory();
  const fanHealth = useFanHealth();
  const queryClient = useQueryClient();
  const toast = useToast();

  const isControllerRunning = monitoring?.controller?.running ?? false;
  const driverLabel = monitoring?.controller?.driver_vendor
    ? `${monitoring.controller.driver_vendor}${
        monitoring.controller.driver_model ? ` · ${monitoring.controller.driver_model}` : ''
      }`
    : 'Unknown';

  const { data: recentLogs } = useQuery({
    queryKey: ['recent-logs'],
    queryFn: () => logsApi.list({ limit: 5 }),
    refetchInterval: 10000,
  });

  const startMutation = useMutation({
    mutationFn: settingsApi.startController,
    onSuccess: () => {
      toast.success('Controller started');
      queryClient.invalidateQueries({ queryKey: ['monitoring'] });
    },
    onError: (e: Error) => toast.error(`Failed to start controller: ${e.message}`),
  });

  const stopMutation = useMutation({
    mutationFn: settingsApi.stopController,
    onSuccess: () => {
      toast.warning('Controller stopped');
      queryClient.invalidateQueries({ queryKey: ['monitoring'] });
    },
    onError: (e: Error) => toast.error(`Failed to stop controller: ${e.message}`),
  });

  const maxGpuTemp = Math.max(...(monitoring?.gpus?.map((g) => g.temperature) || [0]));
  const maxCpuTemp = Math.max(...(monitoring?.system?.cpu_packages?.map((c) => c.temperature) || [0]));
  const maxDriveTemp = Math.max(...(monitoring?.system?.drives?.map((d) => d.temperature) || [0]));
  const activeProfiles = monitoring?.controller?.active_profiles ?? [];

  // Count fans that actually have something connected: the driver exposes one
  // zone per PWM header (10 here), but an empty header reads 0 RPM and never
  // spins → "idle". Count the running/failed ones so the stat reflects real
  // fans, not every header. Failed still counts — it's a connected fan that
  // stopped, flagged separately on its gauge.
  const fans = monitoring?.fans ?? [];
  const activeFanCount = fans.filter((f) => fanHealth(f) !== 'idle').length;

  return (
    <div className="space-y-6">
      {/* Title + promoted controller control */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-bold text-fg">Dashboard</h1>
        <div className="flex items-center gap-3">
          <span className="flex items-center gap-2 text-sm">
            <span
              className={`w-2.5 h-2.5 rounded-full ${isControllerRunning ? 'bg-ok' : 'bg-danger'}`}
              aria-hidden="true"
            />
            <span className={isControllerRunning ? 'text-ok' : 'text-danger'}>
              Controller {isControllerRunning ? 'running' : 'stopped'}
            </span>
          </span>
          {isControllerRunning ? (
            <Button
              variant="secondary"
              size="sm"
              onClick={() => stopMutation.mutate()}
              isLoading={stopMutation.isPending}
            >
              <Square className="w-4 h-4 mr-2" />
              Stop
            </Button>
          ) : (
            <Button size="sm" onClick={() => startMutation.mutate()} isLoading={startMutation.isPending}>
              <Play className="w-4 h-4 mr-2" />
              Start
            </Button>
          )}
        </div>
      </div>

      {/* Quick stats */}
      <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-4">
        <StatCard label="Driver" value={<span className="text-base">{driverLabel}</span>} icon={Chip} />
        <StatCard
          label="Active Profiles"
          value={activeProfiles.length}
          icon={CheckCircle}
          valueClass={activeProfiles.length > 0 ? 'text-ok' : 'text-muted'}
        />
        <StatCard
          label="Max GPU"
          value={maxGpuTemp > 0 ? `${maxGpuTemp.toFixed(1)}°C` : '--'}
          icon={Thermometer}
          valueClass={maxGpuTemp > 0 ? statusClass(worstStatus(monitoring?.gpus ?? []), maxGpuTemp) : 'text-muted'}
        />
        <StatCard
          label="Max CPU"
          value={maxCpuTemp > 0 ? `${maxCpuTemp.toFixed(1)}°C` : '--'}
          icon={Cpu}
          valueClass={
            maxCpuTemp > 0 ? statusClass(worstStatus(monitoring?.system?.cpu_packages ?? []), maxCpuTemp) : 'text-muted'
          }
        />
        <StatCard
          label="Max Drive"
          value={maxDriveTemp > 0 ? `${maxDriveTemp.toFixed(1)}°C` : '--'}
          icon={HardDrive}
          valueClass={
            maxDriveTemp > 0 ? statusClass(worstStatus(monitoring?.system?.drives ?? []), maxDriveTemp) : 'text-muted'
          }
        />
        <StatCard label="Active Fans" value={activeFanCount} icon={Fan} />
      </div>

      {/* Time series */}
      <Card title="Temperature vs. Fan Duty">
        <Suspense
          fallback={
            <div className="h-64 flex items-center justify-center text-sm text-muted">Loading chart…</div>
          }
        >
          <TimeSeriesPanel history={history} variant="temp" />
        </Suspense>
      </Card>

      <Card title="CPU/GPU Load vs Fan Duty">
        <Suspense
          fallback={
            <div className="h-64 flex items-center justify-center text-sm text-muted">Loading chart…</div>
          }
        >
          <TimeSeriesPanel history={history} variant="load" />
        </Suspense>
      </Card>

      {/* Fans as radial gauges — one wide grid that fills the row. Each card
          carries its control-zone chip (grouping is 1 fan per PWM channel on
          hwmon, so per-zone sections would just stack single gauges). */}
      <Card title="Fans">
        {monitoring?.fans && monitoring.fans.length > 0 ? (
          <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 xl:grid-cols-8 gap-4">
            {[...monitoring.fans]
              .sort((a, b) => (a.ipmi_zone ?? Infinity) - (b.ipmi_zone ?? Infinity))
              .map((fan) => {
                const state = fanHealth(fan);
                // An idle output (nothing spinning, not a failure) reads 0%:
                // the chip still reports a duty for the empty header, which
                // means nothing. A FAILED fan keeps its commanded duty so the
                // "commanded X% but no RPM" mismatch stays visible.
                const shownDuty = state === 'idle' ? 0 : Math.max(0, fan.current_duty ?? 0);
                const cardCls =
                  state === 'failed'
                    ? 'border border-danger bg-danger/10'
                    : state === 'idle'
                      ? 'bg-surface-2/50 opacity-60'
                      : 'bg-surface-2/50';
                return (
                  <div key={fan.id} className={`flex flex-col items-center p-3 rounded-lg ${cardCls}`}>
                    <RadialGauge
                      value={shownDuty}
                      tone={state === 'failed' ? 'danger' : dutyTone(shownDuty)}
                      size={104}
                    />
                    <span
                      className="mt-2 text-sm font-medium text-fg-2 text-center truncate w-full"
                      title={fan.label}
                    >
                      {fan.label}
                    </span>
                    {state === 'failed' ? (
                      <span className="text-xs font-semibold text-danger tabular-nums inline-flex items-center gap-1">
                        <AlertTriangle className="w-3 h-3" /> No RPM
                      </span>
                    ) : (
                      <span
                        className={`text-xs tabular-nums ${state === 'idle' ? 'text-muted-2' : 'text-muted'}`}
                      >
                        {fan.current_rpm} RPM{state === 'idle' ? ' · idle' : ''}
                        {fan.control_mode === 'firmware' ? ' · firmware' : ''}
                      </span>
                    )}
                    <span className="mt-1 inline-flex items-center gap-1 text-[10px] uppercase tracking-wide text-muted-2">
                      {fan.ipmi_zone == null ? (
                        <>
                          <AlertTriangle className="w-3 h-3 text-warn" />
                          Unassigned
                        </>
                      ) : (
                        <>
                          <Layers className="w-3 h-3" />
                          {fan.channel != null ? `pwm${fan.channel}` : `Zone ${fan.ipmi_zone}`}
                        </>
                      )}
                    </span>
                    {fan.manual_override && (
                      <span className="mt-0.5 text-[10px] uppercase tracking-wide text-warn">Manual</span>
                    )}
                  </div>
                );
              })}
          </div>
        ) : (
          <p className="text-muted text-center py-4">No fans detected</p>
        )}
      </Card>

      {/* Temperature sensors */}
      <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-6">
        <Card title="GPUs">
          {monitoring?.gpus && monitoring.gpus.length > 0 ? (
            <CollapsibleSection
              title="Graphics Cards"
              count={monitoring.gpus.length}
              maxTemp={maxGpuTemp}
              status={worstStatus(monitoring.gpus)}
              icon={Activity}
              iconColor="text-info"
            >
              {monitoring.gpus.map((gpu) => (
                <div
                  key={gpu.index}
                  className="flex items-center justify-between p-3 bg-surface-2/50 rounded-lg"
                  title={limitTooltip(gpu).join(' · ') || undefined}
                >
                  <div>
                    <p className="font-medium text-fg-2">{gpu.name}</p>
                    <p className="text-sm text-muted">GPU {gpu.index}</p>
                  </div>
                  <div className="text-right">
                    <p className={`text-xl font-bold ${TONE_TEXT[deviceTone(gpu)]}`}>{gpu.temperature.toFixed(1)}°C</p>
                    <p className="text-sm text-muted">
                      {gpu.load}% • {formatBytes(gpu.memory_used)} / {formatBytes(gpu.memory_total)}
                      {headroomText(gpu)}
                    </p>
                  </div>
                </div>
              ))}
            </CollapsibleSection>
          ) : (
            <p className="text-muted text-center py-4">No GPUs detected</p>
          )}
        </Card>

        <Card title="CPUs">
          {monitoring?.system?.cpu_packages && monitoring.system.cpu_packages.length > 0 ? (
            <CollapsibleSection
              title="CPU Packages"
              count={monitoring.system.cpu_packages.length}
              maxTemp={maxCpuTemp}
              status={worstStatus(monitoring.system.cpu_packages)}
              icon={Cpu}
              iconColor="text-info"
            >
              {monitoring.system.cpu_packages.map((cpu) => (
                <div
                  key={cpu.index}
                  className="flex items-start justify-between gap-3 p-3 bg-surface-2/50 rounded-lg"
                  title={limitTooltip(cpu).join(' · ') || undefined}
                >
                  <div className="min-w-0">
                    {/* Model names run long ("AMD Ryzen Threadripper 9960X 24-Cores");
                        wrap rather than truncate so the whole name is readable. */}
                    <p className="font-medium text-fg-2 break-words">{cpu.model || cpu.name}</p>
                    <p className="text-sm text-muted">
                      {cpu.model ? cpu.name : `CPU ${cpu.index}`}
                      {headroomText(cpu)}
                    </p>
                  </div>
                  <p className={`text-xl font-bold shrink-0 ${TONE_TEXT[deviceTone(cpu)]}`}>{cpu.temperature.toFixed(1)}°C</p>
                </div>
              ))}
            </CollapsibleSection>
          ) : (
            <p className="text-muted text-center py-4">No CPU sensors detected</p>
          )}
        </Card>

        {/* Drives */}
        <Card title="Drives">
          {monitoring?.system?.drives && monitoring.system.drives.length > 0 ? (
            <CollapsibleSection
              title="Storage Drives"
              count={monitoring.system.drives.length}
              maxTemp={maxDriveTemp}
              status={worstStatus(monitoring.system.drives)}
              icon={HardDrive}
              iconColor="text-info"
            >
              {monitoring.system.drives.map((drive) => (
                <div
                  key={drive.index}
                  className="flex items-start justify-between gap-3 p-3 bg-surface-2/50 rounded-lg"
                  title={driveTooltip(drive) || undefined}
                >
                  <div className="flex items-start gap-3 min-w-0">
                    <HardDrive className="w-5 h-5 text-muted shrink-0 mt-0.5" />
                    <div className="min-w-0">
                      <p className="font-medium text-fg-2 break-words">{drive.model || drive.device}</p>
                      <p className="text-sm text-muted">
                        {drive.device.replace(/^\/dev\//, '')} • {drive.type.toUpperCase()}
                        {headroomText(drive)}
                      </p>
                    </div>
                  </div>
                  <p className={`text-xl font-bold shrink-0 ${TONE_TEXT[deviceTone(drive)]}`}>{drive.temperature.toFixed(1)}°C</p>
                </div>
              ))}
            </CollapsibleSection>
          ) : (
            <p className="text-muted text-center py-4">No drives detected</p>
          )}
        </Card>
      </div>

      {/* Recent events */}
      <Card title="Recent Events">
          <div className="space-y-2">
            {(recentLogs as { events?: Array<{ id: number; timestamp: string; level: string; message: string }> })?.events?.map(
              (event) => (
                <div key={event.id} className="flex items-start gap-2 p-2 hover:bg-surface-2/50 rounded">
                  {event.level === 'error' ? (
                    <AlertTriangle className="w-4 h-4 text-danger mt-0.5" />
                  ) : event.level === 'warning' ? (
                    <AlertTriangle className="w-4 h-4 text-warn mt-0.5" />
                  ) : (
                    <CheckCircle className="w-4 h-4 text-ok mt-0.5" />
                  )}
                  <div className="flex-1 min-w-0">
                    <p className="text-sm text-fg-3 truncate">{event.message}</p>
                    <p className="text-xs text-muted-2 flex items-center gap-1">
                      <Clock className="w-3 h-3" />
                      {new Date(event.timestamp).toLocaleString()}
                    </p>
                  </div>
                </div>
              )
            )}
            {(!recentLogs || !(recentLogs as { events?: unknown[] }).events?.length) && (
              <p className="text-muted text-center py-4">No recent events</p>
            )}
          </div>
        </Card>
    </div>
  );
}
