import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { fansApi, settingsApi } from '../services/api';
import { useMonitoring } from '../hooks/useMonitoring';
import { Card } from '../components/common/Card';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
import { useToast } from '../components/common/Toast';
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
  const toast = useToast();
  const { data: monitoring } = useMonitoring();
  const [speedFan, setSpeedFan] = useState<FanStatus | null>(null);
  const [speedValue, setSpeedValue] = useState(50);
  const [editingFanLabel, setEditingFanLabel] = useState<FanStatus | null>(null);
  const [newLabel, setNewLabel] = useState('');
  const [activeTab, setActiveTab] = useState<'fans' | 'zones'>('fans');

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
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ['fans'] });
      toast.success(`Fan detection complete — found ${(data as unknown[])?.length ?? 0} fans.`);
    },
    onError: (e: Error) => toast.error(`Fan detection failed: ${e.message}`),
  });

  const updateFanMutation = useMutation({
    mutationFn: ({ id, data }: { id: number; data: { label?: string; ipmi_zone?: number } }) =>
      fansApi.update(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['fans'] });
      setEditingFanLabel(null);
      toast.success('Fan updated');
    },
    onError: (e: Error) => toast.error(`Could not update fan: ${e.message}`),
  });

  const updateSettingsMutation = useMutation({
    mutationFn: settingsApi.update,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['settings'] });
      toast.success('Zone layout saved');
    },
    onError: (e: Error) => toast.error(`Could not save zone layout: ${e.message}`),
  });

  const identifyMutation = useMutation({
    mutationFn: (id: number) => fansApi.identify(id, 5),
    onSuccess: () => toast.info('Identifying fan — it will spin up for a few seconds.'),
    onError: (e: Error) => toast.error(`Identify failed: ${e.message}`),
  });

  const setSpeedMutation = useMutation({
    mutationFn: ({ id, percent }: { id: number; percent: number }) =>
      fansApi.setSpeed(id, percent),
    onSuccess: (_data, vars) => {
      setSpeedFan(null);
      queryClient.invalidateQueries({ queryKey: ['fans'] });
      toast.success(`Manual override set to ${vars.percent}%`);
    },
    onError: (e: Error) => toast.error(`Could not set speed: ${e.message}`),
  });

  const clearSpeedMutation = useMutation({
    mutationFn: (id: number) => fansApi.clearSpeed(id),
    onSuccess: () => {
      setSpeedFan(null);
      queryClient.invalidateQueries({ queryKey: ['fans'] });
      toast.success('Returned to automatic control');
    },
    onError: (e: Error) => toast.error(`Could not reset to auto: ${e.message}`),
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
        <h1 className="text-2xl font-bold text-fg">Fans &amp; Zones</h1>
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

      {/* Tab Navigation */}
      <div className="flex border-b border-surface-2">
        <button
          onClick={() => setActiveTab('fans')}
          className={`px-4 py-2 font-medium transition-colors ${
            activeTab === 'fans'
              ? 'text-info border-b-2 border-info'
              : 'text-muted hover:text-fg-2'
          }`}
        >
          <Fan className="w-4 h-4 inline mr-2" />
          Fans
        </button>
        <button
          onClick={() => setActiveTab('zones')}
          className={`px-4 py-2 font-medium transition-colors ${
            activeTab === 'zones'
              ? 'text-info border-b-2 border-info'
              : 'text-muted hover:text-fg-2'
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
                  <h2 className="text-lg font-semibold text-fg-2">{zoneName(zoneId)}</h2>
                  <span className="text-xs bg-surface-2 text-fg-3 px-2 py-0.5 rounded">
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

          {isLoading && mergedFans.length === 0 && (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              {Array.from({ length: 6 }).map((_, i) => (
                <Card key={`skel-${i}`}>
                  <div className="space-y-4" aria-hidden="true">
                    <div className="flex items-center gap-3">
                      <div className="skeleton w-10 h-10 rounded-lg" />
                      <div className="flex-1 space-y-2">
                        <div className="skeleton h-4 w-1/2" />
                        <div className="skeleton h-3 w-1/3" />
                      </div>
                    </div>
                    <div className="grid grid-cols-2 gap-4">
                      <div className="skeleton h-16" />
                      <div className="skeleton h-16" />
                    </div>
                    <div className="skeleton h-8 w-full" />
                  </div>
                </Card>
              ))}
            </div>
          )}

          {mergedFans.length === 0 && !isLoading && (
            <div className="text-center py-12">
              <Fan className="w-12 h-12 text-surface-3 mx-auto mb-4" />
              <p className="text-muted">No fans detected</p>
              <p className="text-sm text-muted-2 mt-1">Click "Detect Fans" to scan for fan sensors</p>
            </div>
          )}
        </div>
      ) : (
        <Card title="Zone Naming (advanced)">
          <div className="space-y-4">
            <p className="text-sm text-muted">
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
            <p className="text-xs text-muted mt-1">
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
            <div className="flex justify-between text-xs text-muted mt-1">
              <span>0%</span>
              <span>50%</span>
              <span>100%</span>
            </div>
          </div>
          <p className="text-sm text-muted">
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
            <div className={`p-2 rounded-lg ${isSpinning ? 'bg-ok/15' : 'bg-surface-2'}`}>
              <Fan
                className={`w-6 h-6 ${isSpinning ? 'text-ok animate-spin' : 'text-muted-2'}`}
              />
            </div>
            <div>
              <h3 className="font-semibold text-fg-2">{fan.label || fan.ipmi_sensor_id}</h3>
              <p className="text-sm text-muted">
                {fan.ipmi_sensor_id}
                {fan.channel != null && <span className="ml-1 text-muted-2">· pwm{fan.channel}</span>}
              </p>
            </div>
          </div>
          {fan.manual_override && (
            <span className="text-xs bg-warn/15 text-warn px-2 py-1 rounded">Manual</span>
          )}
        </div>

        <div className="grid grid-cols-2 gap-4 text-center">
          <div className="p-3 bg-surface-2/50 rounded-lg">
            <p className="text-2xl font-bold text-fg">{fan.current_rpm || 0}</p>
            <p className="text-xs text-muted">RPM{!connected && ' (idle)'}</p>
          </div>
          <div className="p-3 bg-surface-2/50 rounded-lg">
            <p className="text-2xl font-bold text-fg">{fan.current_duty ?? '-'}</p>
            <p className="text-xs text-muted">Duty %</p>
          </div>
        </div>

        {/* Zone assignment */}
        <div className="flex items-center gap-2">
          <label className="text-xs text-muted whitespace-nowrap" htmlFor={`zone-${fan.id}`}>
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
