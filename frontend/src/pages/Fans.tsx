import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { fansApi } from '../services/api';
import { useMonitoring } from '../hooks/useMonitoring';
import { Card } from '../components/common/Card';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
import { FanStatus } from '../types/fan';
import { 
  Fan, 
  Search, 
  Edit2, 
  Volume2, 
  Sliders,
  RefreshCw
} from 'lucide-react';

export function Fans() {
  const queryClient = useQueryClient();
  const { data: monitoring } = useMonitoring();
  const [editingFan, setEditingFan] = useState<FanStatus | null>(null);
  const [speedFan, setSpeedFan] = useState<FanStatus | null>(null);
  const [editLabel, setEditLabel] = useState('');
  const [editZone, setEditZone] = useState<number | undefined>();
  const [speedValue, setSpeedValue] = useState(50);

  const { data: fans, isLoading, refetch } = useQuery({
    queryKey: ['fans'],
    queryFn: fansApi.list,
  });

  const detectMutation = useMutation({
    mutationFn: fansApi.detect,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['fans'] });
    },
  });

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: number; data: { label?: string; ipmi_zone?: number } }) =>
      fansApi.update(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['fans'] });
      setEditingFan(null);
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

  const handleEdit = (fan: FanStatus) => {
    setEditingFan(fan);
    setEditLabel(fan.label || '');
    setEditZone(fan.ipmi_zone);
  };

  const handleSaveEdit = () => {
    if (editingFan) {
      updateMutation.mutate({
        id: editingFan.id,
        data: {
          label: editLabel || undefined,
          ipmi_zone: editZone,
        },
      });
    }
  };

  const handleSetSpeed = (fan: FanStatus) => {
    setSpeedFan(fan);
    setSpeedValue(50);
  };

  const handleApplySpeed = () => {
    if (speedFan) {
      setSpeedMutation.mutate({ id: speedFan.id, percent: speedValue });
    }
  };

  // Merge static fan data with real-time monitoring data
  const mergedFans = (fans as FanStatus[] || []).map((fan) => {
    const liveData = monitoring?.fans?.find((f) => f.id === fan.id);
    return liveData ? { ...fan, ...liveData } : fan;
  });

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-slate-100">Fans</h1>
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

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {mergedFans.map((fan) => (
          <Card key={fan.id}>
            <div className="space-y-4">
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-3">
                  <div className={`p-2 rounded-lg ${fan.current_rpm > 0 ? 'bg-green-900/50' : 'bg-slate-700'}`}>
                    <Fan 
                      className={`w-6 h-6 ${fan.current_rpm > 0 ? 'text-green-400' : 'text-slate-500'}`}
                      style={fan.current_rpm > 0 ? { animation: 'spin 1s linear infinite' } : undefined}
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
                    {fan.ipmi_zone !== undefined ? fan.ipmi_zone : '-'}
                  </p>
                  <p className="text-xs text-slate-400">Zone</p>
                </div>
              </div>

              <div className="flex gap-2">
                <Button
                  variant="secondary"
                  size="sm"
                  className="flex-1"
                  onClick={() => handleEdit(fan)}
                >
                  <Edit2 className="w-4 h-4 mr-1" />
                  Edit
                </Button>
                <Button
                  variant="secondary"
                  size="sm"
                  className="flex-1"
                  onClick={() => identifyMutation.mutate(fan.id)}
                  isLoading={identifyMutation.isPending && identifyMutation.variables === fan.id}
                  disabled={fan.ipmi_zone === undefined}
                >
                  <Volume2 className="w-4 h-4 mr-1" />
                  Identify
                </Button>
                <Button
                  variant="secondary"
                  size="sm"
                  className="flex-1"
                  onClick={() => handleSetSpeed(fan)}
                  disabled={fan.ipmi_zone === undefined}
                >
                  <Sliders className="w-4 h-4 mr-1" />
                  Speed
                </Button>
              </div>
            </div>
          </Card>
        ))}

        {mergedFans.length === 0 && !isLoading && (
          <div className="col-span-full text-center py-12">
            <Fan className="w-12 h-12 text-slate-600 mx-auto mb-4" />
            <p className="text-slate-400">No fans detected</p>
            <p className="text-sm text-slate-500 mt-1">Click "Detect Fans" to scan for IPMI fan sensors</p>
          </div>
        )}
      </div>

      {/* Edit Fan Modal */}
      <Modal
        isOpen={!!editingFan}
        onClose={() => setEditingFan(null)}
        title="Edit Fan"
      >
        <div className="space-y-4">
          <div>
            <label className="input-label">Label</label>
            <input
              type="text"
              className="input"
              value={editLabel}
              onChange={(e) => setEditLabel(e.target.value)}
              placeholder="e.g., CPU Cooler, GPU Zone"
            />
          </div>
          <div>
            <label className="input-label">PWM Zone</label>
            <input
              type="number"
              className="input"
              value={editZone ?? ''}
              onChange={(e) => setEditZone(e.target.value ? parseInt(e.target.value) : undefined)}
              placeholder="e.g., 0, 1, 2"
              min={0}
              max={7}
            />
            <p className="text-xs text-slate-400 mt-1">
              PWM zone for speed control. Multiple fans may share a zone.
            </p>
          </div>
          <div className="flex justify-end gap-2">
            <Button variant="secondary" onClick={() => setEditingFan(null)}>
              Cancel
            </Button>
            <Button onClick={handleSaveEdit} isLoading={updateMutation.isPending}>
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
            This will set a manual override for this fan. The profile-based control will be bypassed.
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
