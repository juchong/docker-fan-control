import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useMonitoring } from '../hooks/useMonitoring';
import { Card } from '../components/common/Card';
import { logsApi } from '../services/api';
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
  Layers
} from 'lucide-react';

function getTempColor(temp: number): string {
  if (temp < 50) return 'text-blue-400';
  if (temp < 65) return 'text-green-400';
  if (temp < 80) return 'text-yellow-400';
  return 'text-red-400';
}

function formatBytes(bytes: number): string {
  const gb = bytes / (1024 * 1024 * 1024);
  return `${gb.toFixed(1)} GB`;
}

// Collapsible section component
function CollapsibleSection({ 
  title, 
  count, 
  maxTemp, 
  icon: Icon, 
  iconColor,
  children,
  defaultExpanded = true,
  maxVisible = 6
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
        className="w-full flex items-center justify-between p-2 hover:bg-slate-700/50 rounded-lg transition-colors"
      >
        <div className="flex items-center gap-2">
          <Icon className={`w-5 h-5 ${iconColor}`} />
          <span className="font-medium text-slate-200">{title}</span>
          <span className="text-sm text-slate-400">({count})</span>
          {maxTemp !== undefined && maxTemp > 0 && (
            <span className={`text-sm font-medium ${getTempColor(maxTemp)}`}>
              Max: {maxTemp}°C
            </span>
          )}
        </div>
        {expanded ? <ChevronUp className="w-4 h-4 text-slate-400" /> : <ChevronDown className="w-4 h-4 text-slate-400" />}
      </button>
      
      {expanded && (
        <div className="space-y-2">
          {visibleChildren}
          {hasMore && (
            <button
              onClick={() => setShowAll(!showAll)}
              className="w-full text-center text-sm text-slate-400 hover:text-slate-300 py-2"
            >
              {showAll ? 'Show less' : `Show all ${children.length} items`}
            </button>
          )}
        </div>
      )}
    </div>
  );
}

export function Dashboard() {
  const { data: monitoring } = useMonitoring();
  const isControllerRunning = monitoring?.controller?.running || false;

  const { data: recentLogs } = useQuery({
    queryKey: ['recent-logs'],
    queryFn: () => logsApi.list({ limit: 5 }),
    refetchInterval: 10000,
  });

  // Calculate max temperatures
  const maxGpuTemp = Math.max(...(monitoring?.gpus?.map(g => g.temperature) || [0]));
  const maxCpuTemp = Math.max(...(monitoring?.system?.cpu_packages?.map(c => c.temperature) || [0]));
  const maxDriveTemp = Math.max(...(monitoring?.system?.drives?.map(d => d.temperature) || [0]));

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold text-slate-100">Dashboard</h1>

      {/* Controller Status */}
      <Card title="Controller Status">
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div className="p-3 bg-slate-700/50 rounded-lg">
            <div className="flex items-center gap-2 mb-2">
              {isControllerRunning ? (
                <CheckCircle className="w-5 h-5 text-green-400" />
              ) : (
                <AlertTriangle className="w-5 h-5 text-yellow-400" />
              )}
              <span className="font-medium text-slate-200">Status</span>
            </div>
            <div className={`text-lg font-bold ${isControllerRunning ? 'text-green-400' : 'text-red-400'}`}>
              {isControllerRunning ? 'Running' : 'Stopped'}
            </div>
          </div>

        </div>
      </Card>

      {/* Quick Stats */}
      <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-4">
        <div className="bg-slate-800 border border-slate-700 rounded-lg p-4">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm text-slate-400">GPUs</p>
              <p className="text-2xl font-bold text-slate-100">{monitoring?.gpus?.length || 0}</p>
            </div>
            <div className="p-2 bg-purple-900/50 rounded-lg">
              <Activity className="w-5 h-5 text-purple-400" />
            </div>
          </div>
        </div>

        <div className="bg-slate-800 border border-slate-700 rounded-lg p-4">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm text-slate-400">Max GPU</p>
              <p className={`text-2xl font-bold ${getTempColor(maxGpuTemp)}`}>
                {maxGpuTemp > 0 ? `${maxGpuTemp}°C` : '--'}
              </p>
            </div>
            <div className="p-2 bg-orange-900/50 rounded-lg">
              <Thermometer className="w-5 h-5 text-orange-400" />
            </div>
          </div>
        </div>

        <div className="bg-slate-800 border border-slate-700 rounded-lg p-4">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm text-slate-400">Max CPU</p>
              <p className={`text-2xl font-bold ${getTempColor(maxCpuTemp)}`}>
                {maxCpuTemp > 0 ? `${maxCpuTemp}°C` : '--'}
              </p>
            </div>
            <div className="p-2 bg-blue-900/50 rounded-lg">
              <Cpu className="w-5 h-5 text-blue-400" />
            </div>
          </div>
        </div>

        <div className="bg-slate-800 border border-slate-700 rounded-lg p-4">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm text-slate-400">Max Drive</p>
              <p className={`text-2xl font-bold ${getTempColor(maxDriveTemp)}`}>
                {maxDriveTemp > 0 ? `${maxDriveTemp}°C` : '--'}
              </p>
            </div>
            <div className="p-2 bg-cyan-900/50 rounded-lg">
              <HardDrive className="w-5 h-5 text-cyan-400" />
            </div>
          </div>
        </div>

        <div className="bg-slate-800 border border-slate-700 rounded-lg p-4">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm text-slate-400">CPU Load</p>
              <p className="text-2xl font-bold text-slate-100">
                {monitoring?.system?.cpu_load?.toFixed(1) || 0}%
              </p>
            </div>
            <div className="p-2 bg-indigo-900/50 rounded-lg">
              <Activity className="w-5 h-5 text-indigo-400" />
            </div>
          </div>
        </div>

        <div className="bg-slate-800 border border-slate-700 rounded-lg p-4">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm text-slate-400">Fans</p>
              <p className="text-2xl font-bold text-slate-100">{monitoring?.fans?.length || 0}</p>
            </div>
            <div className="p-2 bg-green-900/50 rounded-lg">
              <Fan className="w-5 h-5 text-green-400" />
            </div>
          </div>
        </div>
      </div>

      {/* Temperature Sensors */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <Card title="GPUs">
          {monitoring?.gpus && monitoring.gpus.length > 0 ? (
            <CollapsibleSection title="Graphics Cards" count={monitoring.gpus.length} maxTemp={maxGpuTemp} icon={Activity} iconColor="text-purple-400">
              {monitoring.gpus.map((gpu) => (
                <div key={gpu.index} className="flex items-center justify-between p-3 bg-slate-700/50 rounded-lg">
                  <div>
                    <p className="font-medium text-slate-200">{gpu.name}</p>
                    <p className="text-sm text-slate-400">GPU {gpu.index}</p>
                  </div>
                  <div className="text-right">
                    <p className={`text-xl font-bold ${getTempColor(gpu.temperature)}`}>{gpu.temperature}°C</p>
                    <p className="text-sm text-slate-400">
                      {gpu.load}% • {formatBytes(gpu.memory_used)} / {formatBytes(gpu.memory_total)}
                    </p>
                  </div>
                </div>
              ))}
            </CollapsibleSection>
          ) : (
            <p className="text-slate-400 text-center py-4">No GPUs detected</p>
          )}
        </Card>

        <Card title="CPUs">
          {monitoring?.system?.cpu_packages && monitoring.system.cpu_packages.length > 0 ? (
            <CollapsibleSection title="CPU Packages" count={monitoring.system.cpu_packages.length} maxTemp={maxCpuTemp} icon={Cpu} iconColor="text-blue-400">
              {monitoring.system.cpu_packages.map((cpu) => (
                <div key={cpu.index} className="flex items-center justify-between p-3 bg-slate-700/50 rounded-lg">
                  <div>
                    <p className="font-medium text-slate-200">{cpu.name}</p>
                    <p className="text-sm text-slate-400">CPU {cpu.index}</p>
                  </div>
                  <p className={`text-xl font-bold ${getTempColor(cpu.temperature)}`}>{cpu.temperature.toFixed(0)}°C</p>
                </div>
              ))}
            </CollapsibleSection>
          ) : (
            <p className="text-slate-400 text-center py-4">No CPU sensors detected</p>
          )}
        </Card>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Drives */}
        <Card title="Drives">
          {monitoring?.system?.drives && monitoring.system.drives.length > 0 ? (
            <CollapsibleSection title="Storage Drives" count={monitoring.system.drives.length} maxTemp={maxDriveTemp} icon={HardDrive} iconColor="text-cyan-400">
              {monitoring.system.drives.map((drive) => (
                <div key={drive.index} className="flex items-center justify-between p-3 bg-slate-700/50 rounded-lg">
                  <div className="flex items-center gap-3">
                    <HardDrive className={`w-5 h-5 ${
                      drive.type === 'nvme' ? 'text-purple-400' : drive.type === 'ssd' ? 'text-blue-400' : 'text-slate-400'
                    }`} />
                    <div>
                      <p className="font-medium text-slate-200">{drive.model || drive.device}</p>
                      <p className="text-sm text-slate-400">{drive.device} • {drive.type.toUpperCase()}</p>
                    </div>
                  </div>
                  <p className={`text-xl font-bold ${getTempColor(drive.temperature)}`}>{drive.temperature}°C</p>
                </div>
              ))}
            </CollapsibleSection>
          ) : (
            <p className="text-slate-400 text-center py-4">No drives detected</p>
          )}
        </Card>

        {/* Fan Status - grouped by control zone (fan.ipmi_zone) */}
        <Card title="Fans">
          {monitoring?.fans && monitoring.fans.length > 0 ? (
            <div className="space-y-4">
              {(() => {
                const byZone = new Map<number | null, typeof monitoring.fans>();
                monitoring.fans.forEach((f) => {
                  const z = f.ipmi_zone ?? null;
                  if (!byZone.has(z)) byZone.set(z, []);
                  byZone.get(z)!.push(f);
                });
                const ids = Array.from(byZone.keys()).sort((a, b) =>
                  a === null ? 1 : b === null ? -1 : a - b
                );
                return ids.map((zid) => {
                  const zoneFans = byZone.get(zid)!;
                  return (
                    <div key={zid ?? 'unassigned'} className="space-y-2">
                      <div className="flex items-center gap-2 text-sm">
                        {zid === null ? (
                          <AlertTriangle className="w-4 h-4 text-yellow-400" />
                        ) : (
                          <Layers className="w-4 h-4 text-slate-400" />
                        )}
                        <span className="font-medium text-slate-300">
                          {zid === null ? 'Unassigned' : `Zone ${zid}`}
                        </span>
                        <span className="text-slate-500">({zoneFans.length})</span>
                      </div>
                      {zoneFans.map((fan) => (
                        <div
                          key={fan.id}
                          className="flex items-center justify-between p-2 bg-slate-700/50 rounded-lg ml-6"
                        >
                          <div className="flex items-center gap-2">
                            <Fan className={`w-4 h-4 ${fan.current_rpm > 0 ? 'text-green-400' : 'text-slate-500'}`} />
                            <span className="text-sm text-slate-200">{fan.label}</span>
                            {fan.manual_override && (
                              <span className="text-xs text-yellow-400">Manual</span>
                            )}
                          </div>
                          <div className="text-right">
                            <span className="text-sm font-bold text-slate-200">{fan.current_rpm} RPM</span>
                            <span className="text-xs text-slate-400 ml-2">{fan.current_duty}%</span>
                          </div>
                        </div>
                      ))}
                    </div>
                  );
                });
              })()}
            </div>
          ) : (
            <p className="text-slate-400 text-center py-4">No fans detected</p>
          )}
        </Card>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Active Profiles */}
        <Card title="Active Profiles">
          {monitoring?.controller?.active_profiles && monitoring.controller.active_profiles.length > 0 ? (
            <div className="space-y-2">
              {monitoring.controller.active_profiles.map((profileName, index) => (
                <div key={index} className="flex items-center gap-3 p-2 bg-slate-700/30 rounded">
                  <CheckCircle className="w-5 h-5 text-green-400" />
                  <p className="font-medium text-slate-200">{profileName}</p>
                </div>
              ))}
            </div>
          ) : (
            <div className="flex items-center gap-3 text-slate-400">
              <AlertTriangle className="w-5 h-5" />
              <span>No active profiles</span>
            </div>
          )}
        </Card>

        {/* Recent Events */}
        <Card title="Recent Events">
          <div className="space-y-2">
            {(recentLogs as { events?: Array<{ id: number; timestamp: string; level: string; message: string }> })?.events?.map((event) => (
              <div key={event.id} className="flex items-start gap-2 p-2 hover:bg-slate-700/50 rounded">
                {event.level === 'error' ? (
                  <AlertTriangle className="w-4 h-4 text-red-400 mt-0.5" />
                ) : event.level === 'warning' ? (
                  <AlertTriangle className="w-4 h-4 text-yellow-400 mt-0.5" />
                ) : (
                  <CheckCircle className="w-4 h-4 text-green-400 mt-0.5" />
                )}
                <div className="flex-1 min-w-0">
                  <p className="text-sm text-slate-300 truncate">{event.message}</p>
                  <p className="text-xs text-slate-500 flex items-center gap-1">
                    <Clock className="w-3 h-3" />
                    {new Date(event.timestamp).toLocaleString()}
                  </p>
                </div>
              </div>
            ))}
            {(!recentLogs || !(recentLogs as { events?: unknown[] }).events?.length) && (
              <p className="text-slate-400 text-center py-4">No recent events</p>
            )}
          </div>
        </Card>
      </div>
    </div>
  );
}
