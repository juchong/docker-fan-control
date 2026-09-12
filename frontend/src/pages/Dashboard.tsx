import { useState, lazy, Suspense } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useMonitoring } from '../hooks/useMonitoring';
import { useMetricsHistory } from '../hooks/useMetricsHistory';
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
  return (
    <div className="bg-surface border border-line rounded-lg p-4">
      <div className="flex items-center justify-between gap-2">
        <div className="min-w-0">
          <p className="text-sm text-muted truncate">{label}</p>
          <p className={`text-2xl font-bold ${valueClass}`}>{value}</p>
        </div>
        <div className="p-2 bg-surface-2 rounded-lg shrink-0">
          <Icon className="w-5 h-5 text-muted" />
        </div>
      </div>
    </div>
  );
}

function CollapsibleSection({
  title,
  count,
  maxTemp,
  icon: Icon,
  iconColor,
  children,
  defaultExpanded = true,
  maxVisible = 6,
}: {
  title: string;
  count: number;
  maxTemp?: number;
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
            <span className={`text-sm font-medium ${tempTextClass(maxTemp)}`}>Max: {maxTemp}°C</span>
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
  const history = useMetricsHistory(monitoring);
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
          value={maxGpuTemp > 0 ? `${maxGpuTemp}°C` : '--'}
          icon={Thermometer}
          valueClass={maxGpuTemp > 0 ? tempTextClass(maxGpuTemp) : 'text-muted'}
        />
        <StatCard
          label="Max CPU"
          value={maxCpuTemp > 0 ? `${maxCpuTemp}°C` : '--'}
          icon={Cpu}
          valueClass={maxCpuTemp > 0 ? tempTextClass(maxCpuTemp) : 'text-muted'}
        />
        <StatCard
          label="Max Drive"
          value={maxDriveTemp > 0 ? `${maxDriveTemp}°C` : '--'}
          icon={HardDrive}
          valueClass={maxDriveTemp > 0 ? tempTextClass(maxDriveTemp) : 'text-muted'}
        />
        <StatCard label="Fans" value={monitoring?.fans?.length || 0} icon={Fan} />
      </div>

      {/* Time series */}
      <Card title="Temperature & Fan Duty">
        <Suspense
          fallback={
            <div className="h-64 flex items-center justify-center text-sm text-muted">Loading chart…</div>
          }
        >
          <TimeSeriesPanel history={history} />
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
              .map((fan) => (
                <div
                  key={fan.id}
                  className="flex flex-col items-center p-3 bg-surface-2/50 rounded-lg"
                >
                  <RadialGauge
                    value={fan.current_duty ?? 0}
                    tone={dutyTone(fan.current_duty ?? 0)}
                    size={104}
                  />
                  <span
                    className="mt-2 text-sm font-medium text-fg-2 text-center truncate w-full"
                    title={fan.label}
                  >
                    {fan.label}
                  </span>
                  <span className="text-xs text-muted tabular-nums">{fan.current_rpm} RPM</span>
                  <span className="mt-1 inline-flex items-center gap-1 text-[10px] uppercase tracking-wide text-muted-2">
                    {fan.ipmi_zone == null ? (
                      <>
                        <AlertTriangle className="w-3 h-3 text-warn" />
                        Unassigned
                      </>
                    ) : (
                      <>
                        <Layers className="w-3 h-3" />
                        Zone {fan.ipmi_zone}
                      </>
                    )}
                  </span>
                  {fan.manual_override && (
                    <span className="mt-0.5 text-[10px] uppercase tracking-wide text-warn">Manual</span>
                  )}
                </div>
              ))}
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
              icon={Activity}
              iconColor="text-info"
            >
              {monitoring.gpus.map((gpu) => (
                <div key={gpu.index} className="flex items-center justify-between p-3 bg-surface-2/50 rounded-lg">
                  <div>
                    <p className="font-medium text-fg-2">{gpu.name}</p>
                    <p className="text-sm text-muted">GPU {gpu.index}</p>
                  </div>
                  <div className="text-right">
                    <p className={`text-xl font-bold ${tempTextClass(gpu.temperature)}`}>{gpu.temperature}°C</p>
                    <p className="text-sm text-muted">
                      {gpu.load}% • {formatBytes(gpu.memory_used)} / {formatBytes(gpu.memory_total)}
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
              icon={Cpu}
              iconColor="text-info"
            >
              {monitoring.system.cpu_packages.map((cpu) => (
                <div key={cpu.index} className="flex items-center justify-between p-3 bg-surface-2/50 rounded-lg">
                  <div>
                    <p className="font-medium text-fg-2">{cpu.name}</p>
                    <p className="text-sm text-muted">CPU {cpu.index}</p>
                  </div>
                  <p className={`text-xl font-bold ${tempTextClass(cpu.temperature)}`}>{cpu.temperature.toFixed(0)}°C</p>
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
              icon={HardDrive}
              iconColor="text-info"
            >
              {monitoring.system.drives.map((drive) => (
                <div key={drive.index} className="flex items-center justify-between p-3 bg-surface-2/50 rounded-lg">
                  <div className="flex items-center gap-3">
                    <HardDrive className="w-5 h-5 text-muted" />
                    <div>
                      <p className="font-medium text-fg-2">{drive.model || drive.device}</p>
                      <p className="text-sm text-muted">
                        {drive.device} • {drive.type.toUpperCase()}
                      </p>
                    </div>
                  </div>
                  <p className={`text-xl font-bold ${tempTextClass(drive.temperature)}`}>{drive.temperature}°C</p>
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
