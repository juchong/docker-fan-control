import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { fansApi, settingsApi } from '../services/api';
import { useMonitoring } from '../hooks/useMonitoring';
import { Card } from '../components/common/Card';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
import { useToast } from '../components/common/Toast';
import { HelpTip } from '../components/common/HelpTip';
import { useFanHealth, FanState } from '../hooks/useFanHealth';
import { FanStatus } from '../types/fan';
import { DriverInfo } from '../types/settings';
import {
  Fan,
  Search,
  Edit2,
  Volume2,
  Sliders,
  RefreshCw,
  RotateCcw,
  AlertTriangle,
} from 'lucide-react';

interface ZoneOption {
  id: number;
  name: string;
}

export function Fans() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const fanHealth = useFanHealth();
  const { data: monitoring } = useMonitoring();
  const [speedFan, setSpeedFan] = useState<FanStatus | null>(null);
  const [speedValue, setSpeedValue] = useState(50);
  const [editingFanLabel, setEditingFanLabel] = useState<FanStatus | null>(null);
  const [newLabel, setNewLabel] = useState('');

  const { data: fans, isLoading, refetch } = useQuery({
    queryKey: ['fans'],
    queryFn: fansApi.list,
    refetchInterval: 5000,
  });

  const { data: drivers } = useQuery<DriverInfo[]>({
    queryKey: ['drivers'],
    queryFn: settingsApi.getAvailableDrivers,
  });

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
  // Show all fans in one wide grid, sorted by control zone. Each hwmon PWM
  // channel is its own zone, so per-zone sections would just stack single
  // cards; the per-card zone selector still shows and sets each fan's zone.
  const sortedFans = [...mergedFans].sort(
    (a, b) => (a.ipmi_zone ?? Infinity) - (b.ipmi_zone ?? Infinity)
  );

  const handleSetSpeed = (fan: FanStatus) => {
    setSpeedFan(fan);
    // Seed from the fan's current duty so "nudge" is intuitive (was hardcoded 50).
    setSpeedValue(fan.current_duty != null && fan.current_duty >= 0 ? fan.current_duty : 50);
  };

  const handleSaveLabel = () => {
    if (editingFanLabel) {
      updateFanMutation.mutate({
        id: editingFanLabel.id,
        data: { label: newLabel || undefined },
      });
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-fg">Fans</h1>
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

      <div className="space-y-6">
          {sortedFans.length > 0 && (
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
              {sortedFans.map((fan) => (
                <FanCard
                  key={fan.id}
                  fan={fan}
                  zones={zones}
                  state={fanHealth(fan)}
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
          )}

          {isLoading && mergedFans.length === 0 && (
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
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
  state: FanState;
  onEditLabel: () => void;
  onIdentify: () => void;
  onSetSpeed: () => void;
  onAssignZone: (zoneId: number) => void;
  isIdentifying: boolean;
}

function FanCard({ fan, zones, state, onEditLabel, onIdentify, onSetSpeed, onAssignZone, isIdentifying }: FanCardProps) {
  const isRunning = state === 'running';
  const isFailed = state === 'failed';
  const isIdle = state === 'idle';

  const cardCls = isFailed
    ? 'ring-2 ring-danger bg-danger/5'
    : isIdle
      ? 'opacity-70'
      : '';

  return (
    <Card className={cardCls}>
      <div className="space-y-4">
        <div className="flex items-start justify-between">
          <div className="flex items-center gap-3">
            <div
              className={`p-2 rounded-lg ${
                isFailed ? 'bg-danger/15' : isRunning ? 'bg-ok/15' : 'bg-surface-2'
              }`}
            >
              {isFailed ? (
                <AlertTriangle className="w-6 h-6 text-danger" />
              ) : (
                <Fan className={`w-6 h-6 ${isRunning ? 'text-ok animate-spin' : 'text-muted-2'}`} />
              )}
            </div>
            <div>
              <h3 className="font-semibold text-fg-2">{fan.label || fan.ipmi_sensor_id}</h3>
              <p className="text-sm text-muted">
                {fan.ipmi_sensor_id}
                {fan.channel != null && <span className="ml-1 text-muted-2">· pwm{fan.channel}</span>}
              </p>
            </div>
          </div>
          <div className="flex flex-col items-end gap-1">
            {isFailed && (
              <span className="text-xs bg-danger/15 text-danger px-2 py-1 rounded inline-flex items-center gap-1">
                <AlertTriangle className="w-3 h-3" /> No RPM
              </span>
            )}
            {fan.manual_override && (
              <span className="text-xs bg-warn/15 text-warn px-2 py-1 rounded">Manual</span>
            )}
            {fan.control_mode === 'firmware' && !fan.manual_override && (
              <span
                className="text-xs bg-surface-2 text-muted px-2 py-1 rounded"
                title="The board's own fan curve is driving this fan; it is not targeted by a profile or override."
              >
                Firmware
              </span>
            )}
          </div>
        </div>

        <div className="grid grid-cols-2 gap-4 text-center">
          <div className="p-3 bg-surface-2/50 rounded-lg">
            <p className={`text-2xl font-bold ${isFailed ? 'text-danger' : 'text-fg'}`}>{fan.current_rpm || 0}</p>
            <p className={`text-xs ${isFailed ? 'text-danger' : 'text-muted'}`}>
              RPM{isIdle ? ' (idle)' : ''}{isFailed ? ' (failed?)' : ''}
            </p>
          </div>
          <div className="p-3 bg-surface-2/50 rounded-lg">
            <p className="text-2xl font-bold text-fg">
              {fan.current_duty != null && fan.current_duty >= 0 ? fan.current_duty : '—'}
            </p>
            {/* Same one-line label on every card; the control mode lives in the
                tooltip so a firmware-managed fan doesn't get a taller tile. */}
            <p className="text-xs text-muted inline-flex items-center gap-1">
              Duty %
              <HelpTip
                label="Duty"
                text={
                  fan.control_mode === 'firmware'
                    ? "On the board's own fan curve — no profile or override targets this zone. The value is the chip's readback, which can be approximate on ITE chips; “—” means it cannot be read."
                    : fan.manual_override
                      ? 'Set by a manual override from this page. Use Speed → Reset to auto to hand the fan back.'
                      : 'Commanded by this app: a profile targets this zone.'
                }
              />
            </p>
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

        {/* Wrapping row: three across when the card is wide enough, otherwise
            the last button drops to its own line. A non-wrapping flex row
            overflowed the card, since flex-1 can't shrink below the label. */}
        <div className="flex flex-wrap gap-2">
          <Button variant="secondary" size="sm" className="flex-1 min-w-[6.5rem] whitespace-nowrap" onClick={onEditLabel}>
            <Edit2 className="w-4 h-4 mr-1 shrink-0" />
            Label
          </Button>
          <Button
            variant="secondary"
            size="sm"
            className="flex-1 min-w-[6.5rem] whitespace-nowrap"
            onClick={onIdentify}
            isLoading={isIdentifying}
          >
            <Volume2 className="w-4 h-4 mr-1 shrink-0" />
            Identify
          </Button>
          <Button variant="secondary" size="sm" className="flex-1 min-w-[6.5rem] whitespace-nowrap" onClick={onSetSpeed}>
            <Sliders className="w-4 h-4 mr-1 shrink-0" />
            Speed
          </Button>
        </div>
      </div>
    </Card>
  );
}
