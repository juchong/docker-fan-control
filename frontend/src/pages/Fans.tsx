import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { fansApi, settingsApi } from '../services/api';
import { useMonitoring } from '../hooks/useMonitoring';
import { Card } from '../components/common/Card';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
import { ZoneLayoutEditor } from '../components/zones/ZoneLayoutEditor';
import { ZoneLayout } from '../types/zone';
import { FanStatus } from '../types/fan';
import { AppSettings, DriverInfo } from '../types/settings';
import {
  Fan,
  Search,
  Edit2,
  Volume2,
  Sliders,
  RefreshCw,
  Layers,
  RotateCcw,
} from 'lucide-react';

interface ZoneOption {
  id: number;
  name: string;
}

export function Fans() {
  const queryClient = useQueryClient();
  const { data: monitoring } = useMonitoring();
  const [speedFan, setSpeedFan] = useState<FanStatus | null>(null);
  const [speedValue, setSpeedValue] = useState(50);
  const [editingFanLabel, setEditingFanLabel] = useState<FanStatus | null>(null);
  const [newLabel, setNewLabel] = useState('');
  const [activeTab, setActiveTab] = useState<'fans' | 'zones'>('fans');
  const [actionError, setActionError] = useState<string | null>(null);

  const { data: fans, isLoading, refetch } = useQuery({
    queryKey: ['fans'],
    queryFn: fansApi.list,
    refetchInterval: 5000,
  });

  const { data: settings } = useQuery<AppSettings>({
    queryKey: ['settings'],
    queryFn: settingsApi.get,
  });

  const { data: drivers } = useQuery<DriverInfo[]>({
    queryKey: ['drivers'],
    queryFn: settingsApi.getAvailableDrivers,
  });

  const zoneLayout = settings?.zone_layout ?? null;

  const detectMutation = useMutation({
    mutationFn: fansApi.detect,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['fans'] }),
    onError: (e: Error) => setActionError(e.message),
  });

  const updateFanMutation = useMutation({
    mutationFn: ({ id, data }: { id: number; data: { label?: string; ipmi_zone?: number } }) =>
      fansApi.update(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['fans'] });
      setEditingFanLabel(null);
    },
    onError: (e: Error) => setActionError(e.message),
  });

  const updateSettingsMutation = useMutation({
    mutationFn: settingsApi.update,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['settings'] }),
    onError: (e: Error) => setActionError(e.message),
  });

  const identifyMutation = useMutation({
    mutationFn: (id: number) => fansApi.identify(id, 5),
    onError: (e: Error) => setActionError(e.message),
  });

  const setSpeedMutation = useMutation({
    mutationFn: ({ id, percent }: { id: number; percent: number }) =>
      fansApi.setSpeed(id, percent),
    onSuccess: () => {
      setSpeedFan(null);
      queryClient.invalidateQueries({ queryKey: ['fans'] });
    },
    onError: (e: Error) => setActionError(e.message),
  });

  const clearSpeedMutation = useMutation({
    mutationFn: (id: number) => fansApi.clearSpeed(id),
    onSuccess: () => {
      setSpeedFan(null);
      queryClient.invalidateQueries({ queryKey: ['fans'] });
    },
    onError: (e: Error) => setActionError(e.message),
  });

  // Merge static fan data with real-time monitoring data.
  const mergedFans: FanStatus[] = (fans as FanStatus[] || []).map((fan) => {
    const liveData = monitoring?.fans?.find((f) => f.id === fan.id);
    return liveData ? { ...fan, ...liveData } : fan;
  });

  // Available control zones come from the ACTIVE driver's zone layout (the
  // single source of truth), falling back to the distinct zones the fans
  // already occupy. Zone IDs match fan.ipmi_zone (for hwmon this is the PWM
  // channel).
  const activeVendor =
    monitoring?.controller?.driver_vendor || monitoring?.controller?.motherboard_vendor;
  const activeDriver =
    (drivers || []).find((d) => d.vendor === activeVendor) || (drivers || [])[0];
  const driverZones: ZoneOption[] =
    activeDriver?.zone_layout?.zones?.map((z) => ({ id: z.id, name: z.name })) ?? [];
  const derivedZones: ZoneOption[] = Array.from(
    new Set(mergedFans.map((f) => f.ipmi_zone).filter((z): z is number => z != null))
  )
    .sort((a, b) => a - b)
    .map((id) => ({ id, name: `Zone ${id}` }));
  const zones: ZoneOption[] = driverZones.length ? driverZones : derivedZones;
  const zoneName = (id: number | null | undefined) =>
    id == null ? 'Unassigned' : zones.find((z) => z.id === id)?.name ?? `Zone ${id}`;

  // Group fans by their assigned control zone (fan.ipmi_zone) — no more parsing
  // the sensor name with a FAN(\d+) regex.
  const fansByZone = new Map<number | null, FanStatus[]>();
  mergedFans.forEach((fan) => {
    const z = fan.ipmi_zone ?? null;
    if (!fansByZone.has(z)) fansByZone.set(z, []);
    fansByZone.get(z)!.push(fan);
  });
  const sortedZoneIds = Array.from(fansByZone.keys()).sort((a, b) => {
    if (a === null) return 1;
    if (b === null) return -1;
    return a - b;
  });

  const handleSetSpeed = (fan: FanStatus) => {
    setSpeedFan(fan);
    // Seed from the fan's current duty so "nudge" is intuitive (was hardcoded 50).
    setSpeedValue(fan.current_duty ?? 50);
  };

  const handleSaveLabel = () => {
    if (editingFanLabel) {
      updateFanMutation.mutate({
        id: editingFanLabel.id,
        data: { label: newLabel || undefined },
      });
    }
  };

  const handleSaveZoneLayout = (layout: ZoneLayout) => {
    updateSettingsMutation.mutate({ zone_layout: layout });
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-slate-100">Fans &amp; Zones</h1>
        <div className="flex items-center gap-2">
          <Button variant="secondary" onClick={() => refetch()} isLoading={isLoading}>
            <RefreshCw className="w-4 h-4 mr-2" />
            Refresh
          </Button>
          <Button onClick={() => detectMutation.mutate()} isLoading={detectMutation.isPending}>
            <Search className="w-4 h-4 mr-2" />
            Detect Fans
          </Button>
        </div>
      </div>

      {actionError && (
        <div className="p-3 bg-red-900/40 border border-red-800 rounded-lg text-red-300 flex items-center justify-between">
          <span>{actionError}</span>
          <button className="text-red-400 hover:text-red-200" onClick={() => setActionError(null)}>
            ✕
          </button>
        </div>
      )}

      {detectMutation.isSuccess && (
        <div className="p-3 bg-green-900/50 border border-green-800 rounded-lg text-green-400">
          Fan detection completed. Found {(detectMutation.data as unknown[])?.length || 0} fans.
        </div>
      )}

      {/* Tab Navigation */}
      <div className="flex border-b border-slate-700">
        <button
          onClick={() => setActiveTab('fans')}
          className={`px-4 py-2 font-medium transition-colors ${
            activeTab === 'fans'
              ? 'text-blue-400 border-b-2 border-blue-400'
              : 'text-slate-400 hover:text-slate-200'
          }`}
        >
          <Fan className="w-4 h-4 inline mr-2" />
          Fans
        </button>
        <button
          onClick={() => setActiveTab('zones')}
          className={`px-4 py-2 font-medium transition-colors ${
            activeTab === 'zones'
              ? 'text-blue-400 border-b-2 border-blue-400'
              : 'text-slate-400 hover:text-slate-200'
          }`}
        >
          <Layers className="w-4 h-4 inline mr-2" />
          Zone Naming
        </button>
      </div>

      {activeTab === 'fans' ? (
        <div className="space-y-6">
          {sortedZoneIds.map((zoneId) => {
            const zoneFans = fansByZone.get(zoneId) || [];
            return (
              <div key={zoneId ?? 'unassigned'} className="space-y-3">
                <div className="flex items-center gap-2">
                  <h2 className="text-lg font-semibold text-slate-200">{zoneName(zoneId)}</h2>
                  <span className="text-xs bg-slate-700 text-slate-300 px-2 py-0.5 rounded">
                    {zoneFans.length} fan{zoneFans.length !== 1 ? 's' : ''}
                  </span>
                </div>
                <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                  {zoneFans.map((fan) => (
                    <FanCard
                      key={fan.id}
                      fan={fan}
                      zones={zones}
                      onEditLabel={() => {
                        setEditingFanLabel(fan);
                        setNewLabel(fan.label || '');
                      }}
                      onIdentify={() => identifyMutation.mutate(fan.id)}
                      onSetSpeed={() => handleSetSpeed(fan)}
                      onAssignZone={(zid) => updateFanMutation.mutate({ id: fan.id, data: { ipmi_zone: zid } })}
                      isIdentifying={identifyMutation.isPending && identifyMutation.variables === fan.id}
                    />
                  ))}
                </div>
              </div>
            );
          })}

          {mergedFans.length === 0 && !isLoading && (
            <div className="text-center py-12">
              <Fan className="w-12 h-12 text-slate-600 mx-auto mb-4" />
              <p className="text-slate-400">No fans detected</p>
              <p className="text-sm text-slate-500 mt-1">Click "Detect Fans" to scan for fan sensors</p>
            </div>
          )}
        </div>
      ) : (
        <Card title="Zone Naming (advanced)">
          <div className="space-y-4">
            <p className="text-sm text-slate-400">
              Fans are assigned to control zones directly on the Fans tab. This optional editor
              lets you define named zone groupings used by older profiles.
            </p>
            <ZoneLayoutEditor
              zoneLayout={zoneLayout}
              onSave={handleSaveZoneLayout}
              fans={mergedFans}
              isSaving={updateSettingsMutation.isPending}
            />
          </div>
        </Card>
      )}

      {/* Edit Label Modal */}
      <Modal isOpen={!!editingFanLabel} onClose={() => setEditingFanLabel(null)} title="Edit Fan Label">
        <div className="space-y-4">
          <div>
            <label className="input-label">Label</label>
            <input
              type="text"
              className="input"
              value={newLabel}
              onChange={(e) => setNewLabel(e.target.value)}
              placeholder="e.g., CPU Cooler, Case Front"
            />
            <p className="text-xs text-slate-400 mt-1">
              A friendly name for this fan. Leave empty to use the sensor ID.
            </p>
          </div>
          <div className="flex justify-end gap-2">
            <Button variant="secondary" onClick={() => setEditingFanLabel(null)}>
              Cancel
            </Button>
            <Button onClick={handleSaveLabel} isLoading={updateFanMutation.isPending}>
              Save
            </Button>
          </div>
        </div>
      </Modal>

      {/* Set Speed Modal */}
      <Modal
        isOpen={!!speedFan}
        onClose={() => setSpeedFan(null)}
        title={`Set Speed: ${speedFan?.label || speedFan?.ipmi_sensor_id}`}
      >
        <div className="space-y-4">
          <div>
            <label className="input-label">Fan Speed: {speedValue}%</label>
            <input
              type="range"
              className="w-full"
              value={speedValue}
              onChange={(e) => setSpeedValue(parseInt(e.target.value))}
              min={0}
              max={100}
              step={5}
              aria-label="Fan speed percent"
              aria-valuetext={`${speedValue} percent`}
            />
            <div className="flex justify-between text-xs text-slate-400 mt-1">
              <span>0%</span>
              <span>50%</span>
              <span>100%</span>
            </div>
          </div>
          <p className="text-sm text-slate-400">
            Sets a manual override for this fan's zone, bypassing profile control until you reset it.
          </p>
          <div className="flex justify-between gap-2">
            <Button
              variant="secondary"
              onClick={() => speedFan && clearSpeedMutation.mutate(speedFan.id)}
              isLoading={clearSpeedMutation.isPending}
              disabled={!speedFan?.manual_override}
            >
              <RotateCcw className="w-4 h-4 mr-1" />
              Reset to auto
            </Button>
            <div className="flex gap-2">
              <Button variant="secondary" onClick={() => setSpeedFan(null)}>
                Cancel
              </Button>
              <Button
                onClick={() => speedFan && setSpeedMutation.mutate({ id: speedFan.id, percent: speedValue })}
                isLoading={setSpeedMutation.isPending}
              >
                Apply
              </Button>
            </div>
          </div>
        </div>
      </Modal>
    </div>
  );
}

// Fan Card Component
interface FanCardProps {
  fan: FanStatus;
  zones: ZoneOption[];
  onEditLabel: () => void;
  onIdentify: () => void;
  onSetSpeed: () => void;
  onAssignZone: (zoneId: number) => void;
  isIdentifying: boolean;
}

function FanCard({ fan, zones, onEditLabel, onIdentify, onSetSpeed, onAssignZone, isIdentifying }: FanCardProps) {
  const isSpinning = fan.current_rpm > 0;
  const connected = fan.current_rpm > 0 || (fan.current_duty ?? 0) > 0;

  return (
    <Card>
      <div className="space-y-4">
        <div className="flex items-start justify-between">
          <div className="flex items-center gap-3">
            <div className={`p-2 rounded-lg ${isSpinning ? 'bg-green-900/50' : 'bg-slate-700'}`}>
              <Fan
                className={`w-6 h-6 ${isSpinning ? 'text-green-400 animate-spin' : 'text-slate-500'}`}
              />
            </div>
            <div>
              <h3 className="font-semibold text-slate-200">{fan.label || fan.ipmi_sensor_id}</h3>
              <p className="text-sm text-slate-400">
                {fan.ipmi_sensor_id}
                {fan.channel != null && <span className="ml-1 text-slate-500">· pwm{fan.channel}</span>}
              </p>
            </div>
          </div>
          {fan.manual_override && (
            <span className="text-xs bg-yellow-900/50 text-yellow-400 px-2 py-1 rounded">Manual</span>
          )}
        </div>

        <div className="grid grid-cols-2 gap-4 text-center">
          <div className="p-3 bg-slate-700/50 rounded-lg">
            <p className="text-2xl font-bold text-slate-100">{fan.current_rpm || 0}</p>
            <p className="text-xs text-slate-400">RPM{!connected && ' (idle)'}</p>
          </div>
          <div className="p-3 bg-slate-700/50 rounded-lg">
            <p className="text-2xl font-bold text-slate-100">{fan.current_duty ?? '-'}</p>
            <p className="text-xs text-slate-400">Duty %</p>
          </div>
        </div>

        {/* Zone assignment */}
        <div className="flex items-center gap-2">
          <label className="text-xs text-slate-400 whitespace-nowrap" htmlFor={`zone-${fan.id}`}>
            Control zone
          </label>
          <select
            id={`zone-${fan.id}`}
            className="input py-1 text-sm flex-1"
            value={fan.ipmi_zone ?? ''}
            onChange={(e) => onAssignZone(parseInt(e.target.value))}
          >
            {fan.ipmi_zone == null && <option value="">Unassigned</option>}
            {zones.map((z) => (
              <option key={z.id} value={z.id}>
                {z.name}
              </option>
            ))}
          </select>
        </div>

        <div className="flex gap-2">
          <Button variant="secondary" size="sm" className="flex-1" onClick={onEditLabel}>
            <Edit2 className="w-4 h-4 mr-1" />
            Label
          </Button>
          <Button variant="secondary" size="sm" className="flex-1" onClick={onIdentify} isLoading={isIdentifying}>
            <Volume2 className="w-4 h-4 mr-1" />
            Identify
          </Button>
          <Button variant="secondary" size="sm" className="flex-1" onClick={onSetSpeed}>
            <Sliders className="w-4 h-4 mr-1" />
            Speed
          </Button>
        </div>
      </div>
    </Card>
  );
}
