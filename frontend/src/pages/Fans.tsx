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
import { AppSettings } from '../types/settings';
import { 
  Fan, 
  Search, 
  Edit2, 
  Volume2, 
  Sliders,
  RefreshCw,
  Layers,
  Settings2
} from 'lucide-react';

export function Fans() {
  const queryClient = useQueryClient();
  const { data: monitoring } = useMonitoring();
  const [speedFan, setSpeedFan] = useState<FanStatus | null>(null);
  const [speedValue, setSpeedValue] = useState(50);
  const [editingFanLabel, setEditingFanLabel] = useState<FanStatus | null>(null);
  const [newLabel, setNewLabel] = useState('');
  const [activeTab, setActiveTab] = useState<'fans' | 'zones'>('fans');

  const { data: fans, isLoading, refetch } = useQuery({
    queryKey: ['fans'],
    queryFn: fansApi.list,
  });

  const { data: settings } = useQuery<AppSettings>({
    queryKey: ['settings'],
    queryFn: settingsApi.get,
  });

  const zoneLayout = settings?.zone_layout ?? null;

  const detectMutation = useMutation({
    mutationFn: fansApi.detect,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['fans'] });
    },
  });

  const updateFanMutation = useMutation({
    mutationFn: ({ id, data }: { id: number; data: { label?: string } }) =>
      fansApi.update(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['fans'] });
      setEditingFanLabel(null);
    },
  });

  const updateSettingsMutation = useMutation({
    mutationFn: settingsApi.update,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['settings'] });
    },
  });

  const identifyMutation = useMutation({
    mutationFn: (id: number) => fansApi.identify(id, 5),
  });

  const setSpeedMutation = useMutation({
    mutationFn: ({ id, percent }: { id: number; percent: number }) =>
      fansApi.setSpeed(id, percent),
    onSuccess: () => {
      setSpeedFan(null);
    },
  });

  const handleSetSpeed = (fan: FanStatus) => {
    setSpeedFan(fan);
    setSpeedValue(50);
  };

  const handleApplySpeed = () => {
    if (speedFan) {
      setSpeedMutation.mutate({ id: speedFan.id, percent: speedValue });
    }
  };

  const handleEditLabel = (fan: FanStatus) => {
    setEditingFanLabel(fan);
    setNewLabel(fan.label || '');
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

  // Merge static fan data with real-time monitoring data
  const mergedFans: FanStatus[] = (fans as FanStatus[] || []).map((fan) => {
    const liveData = monitoring?.fans?.find((f) => f.id === fan.id);
    return liveData ? { ...fan, ...liveData } : fan;
  });

  // Group fans by zone
  const getFanZoneId = (fan: FanStatus): number | null => {
    if (!zoneLayout) return null;
    // Extract fan index from sensor ID (FAN1 = 0, FAN2 = 1, etc.)
    const match = fan.ipmi_sensor_id?.match(/FAN(\d+)/i);
    if (!match) return null;
    const fanIndex = parseInt(match[1]) - 1;
    
    const zone = zoneLayout.zones.find(z => z.fan_indices.includes(fanIndex));
    return zone?.id ?? null;
  };

  const fansByZone = new Map<number | null, FanStatus[]>();
  mergedFans.forEach(fan => {
    const zoneId = getFanZoneId(fan);
    if (!fansByZone.has(zoneId)) {
      fansByZone.set(zoneId, []);
    }
    fansByZone.get(zoneId)!.push(fan);
  });

  // Sort zones by ID
  const sortedZoneIds = Array.from(fansByZone.keys()).sort((a, b) => {
    if (a === null) return 1;
    if (b === null) return -1;
    return a - b;
  });

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-slate-100">Fans & Zones</h1>
        <div className="flex items-center gap-2">
          <Button
            variant="secondary"
            onClick={() => refetch()}
            isLoading={isLoading}
          >
            <RefreshCw className="w-4 h-4 mr-2" />
            Refresh
          </Button>
          <Button
            onClick={() => detectMutation.mutate()}
            isLoading={detectMutation.isPending}
          >
            <Search className="w-4 h-4 mr-2" />
            Detect Fans
          </Button>
        </div>
      </div>

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
          Zone Configuration
        </button>
      </div>

      {activeTab === 'fans' ? (
        <>
          {/* Fans Tab Content */}
          {zoneLayout && zoneLayout.zones.length > 0 ? (
            // Display fans grouped by zone
            <div className="space-y-6">
              {sortedZoneIds.map(zoneId => {
                const zoneFans = fansByZone.get(zoneId) || [];
                const zone = zoneId !== null 
                  ? zoneLayout.zones.find(z => z.id === zoneId)
                  : null;
                
                return (
                  <div key={zoneId ?? 'unassigned'} className="space-y-3">
                    <div className="flex items-center gap-2">
                      <h2 className="text-lg font-semibold text-slate-200">
                        {zone ? zone.name : 'Unassigned Fans'}
                      </h2>
                      {zone?.description && (
                        <span className="text-sm text-slate-400">— {zone.description}</span>
                      )}
                      <span className="text-xs bg-slate-700 text-slate-300 px-2 py-0.5 rounded">
                        {zoneFans.length} fan{zoneFans.length !== 1 ? 's' : ''}
                      </span>
                    </div>
                    
                    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                      {zoneFans.map((fan) => (
                        <FanCard
                          key={fan.id}
                          fan={fan}
                          onEditLabel={() => handleEditLabel(fan)}
                          onIdentify={() => identifyMutation.mutate(fan.id)}
                          onSetSpeed={() => handleSetSpeed(fan)}
                          isIdentifying={identifyMutation.isPending && identifyMutation.variables === fan.id}
                        />
                      ))}
                    </div>
                  </div>
                );
              })}
            </div>
          ) : (
            // No zone layout - display fans in a simple grid
            <div className="space-y-4">
              {!zoneLayout && (
                <div className="p-4 bg-amber-900/30 border border-amber-700 rounded-lg">
                  <div className="flex items-center gap-2 text-amber-400 mb-2">
                    <Settings2 className="w-5 h-5" />
                    <span className="font-medium">No zone layout configured</span>
                  </div>
                  <p className="text-sm text-slate-400">
                    Configure zones in the "Zone Configuration" tab to organize your fans and enable profile-based control.
                  </p>
                </div>
              )}
              
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                {mergedFans.map((fan) => (
                  <FanCard
                    key={fan.id}
                    fan={fan}
                    onEditLabel={() => handleEditLabel(fan)}
                    onIdentify={() => identifyMutation.mutate(fan.id)}
                    onSetSpeed={() => handleSetSpeed(fan)}
                    isIdentifying={identifyMutation.isPending && identifyMutation.variables === fan.id}
                  />
                ))}
              </div>
            </div>
          )}

          {mergedFans.length === 0 && !isLoading && (
            <div className="text-center py-12">
              <Fan className="w-12 h-12 text-slate-600 mx-auto mb-4" />
              <p className="text-slate-400">No fans detected</p>
              <p className="text-sm text-slate-500 mt-1">Click "Detect Fans" to scan for IPMI fan sensors</p>
            </div>
          )}
        </>
      ) : (
        /* Zones Tab Content */
        <Card title="Zone Layout Configuration">
          <div className="space-y-4">
            <p className="text-sm text-slate-400">
              Configure how fans are grouped into zones. Profiles use these zones to control fan speeds based on temperature inputs.
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
      <Modal
        isOpen={!!editingFanLabel}
        onClose={() => setEditingFanLabel(null)}
        title="Edit Fan Label"
      >
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
            />
            <div className="flex justify-between text-xs text-slate-400 mt-1">
              <span>0%</span>
              <span>50%</span>
              <span>100%</span>
            </div>
          </div>
          <p className="text-sm text-slate-400">
            This will set a manual override for this fan's zone. Profile-based control will be bypassed.
          </p>
          <div className="flex justify-end gap-2">
            <Button variant="secondary" onClick={() => setSpeedFan(null)}>
              Cancel
            </Button>
            <Button onClick={handleApplySpeed} isLoading={setSpeedMutation.isPending}>
              Apply
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  );
}

// Fan Card Component
interface FanCardProps {
  fan: FanStatus;
  onEditLabel: () => void;
  onIdentify: () => void;
  onSetSpeed: () => void;
  isIdentifying: boolean;
}

function FanCard({ fan, onEditLabel, onIdentify, onSetSpeed, isIdentifying }: FanCardProps) {
  const isSpinning = fan.current_rpm > 0;
  
  return (
    <Card>
      <div className="space-y-4">
        <div className="flex items-start justify-between">
          <div className="flex items-center gap-3">
            <div className={`p-2 rounded-lg ${isSpinning ? 'bg-green-900/50' : 'bg-slate-700'}`}>
              <Fan 
                className={`w-6 h-6 ${isSpinning ? 'text-green-400' : 'text-slate-500'}`}
                style={isSpinning ? { animation: 'spin 1s linear infinite' } : undefined}
              />
            </div>
            <div>
              <h3 className="font-semibold text-slate-200">{fan.label || fan.ipmi_sensor_id}</h3>
              <p className="text-sm text-slate-400">{fan.ipmi_sensor_id}</p>
            </div>
          </div>
          {fan.manual_override && (
            <span className="text-xs bg-yellow-900/50 text-yellow-400 px-2 py-1 rounded">
              Manual
            </span>
          )}
        </div>

        <div className="grid grid-cols-2 gap-4 text-center">
          <div className="p-3 bg-slate-700/50 rounded-lg">
            <p className="text-2xl font-bold text-slate-100">{fan.current_rpm || 0}</p>
            <p className="text-xs text-slate-400">RPM</p>
          </div>
          <div className="p-3 bg-slate-700/50 rounded-lg">
            <p className="text-2xl font-bold text-slate-100">
              {fan.current_duty ?? '-'}
            </p>
            <p className="text-xs text-slate-400">Duty %</p>
          </div>
        </div>

        <div className="flex gap-2">
          <Button
            variant="secondary"
            size="sm"
            className="flex-1"
            onClick={onEditLabel}
          >
            <Edit2 className="w-4 h-4 mr-1" />
            Label
          </Button>
          <Button
            variant="secondary"
            size="sm"
            className="flex-1"
            onClick={onIdentify}
            isLoading={isIdentifying}
          >
            <Volume2 className="w-4 h-4 mr-1" />
            Identify
          </Button>
          <Button
            variant="secondary"
            size="sm"
            className="flex-1"
            onClick={onSetSpeed}
          >
            <Sliders className="w-4 h-4 mr-1" />
            Speed
          </Button>
        </div>
      </div>
    </Card>
  );
}
