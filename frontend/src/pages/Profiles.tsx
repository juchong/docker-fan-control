import React, { useState, useEffect, useMemo } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ReferenceLine, ResponsiveContainer,
} from 'recharts';
import { profilesApi, settingsApi } from '../services/api';
import { useMonitoring } from '../hooks/useMonitoring';
import { Card } from '../components/common/Card';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
import { useToast } from '../components/common/Toast';
import {
  ProfileSummary, Profile, ProfileInput, LinearParams, StepParams, PIDParams,
  AlgorithmParams, InputAggregation,
} from '../types/profile';
import { DriverInfo } from '../types/settings';
import {
  Plus, Play, Pause, Trash2, Edit2, Settings2, CheckCircle, ChevronDown, ChevronUp,
  Cpu, HardDrive, Activity, Layers, Thermometer,
} from 'lucide-react';

interface ZoneOption { id: number; name: string }

function useDriverZones(): ZoneOption[] {
  const { data: monitoring } = useMonitoring();
  const { data: drivers } = useQuery<DriverInfo[]>({
    queryKey: ['drivers'],
    queryFn: settingsApi.getAvailableDrivers,
  });
  const activeVendor =
    monitoring?.controller?.driver_vendor || monitoring?.controller?.motherboard_vendor;
  const active = (drivers || []).find((d) => d.vendor === activeVendor) || (drivers || [])[0];
  return active?.zone_layout?.zones?.map((z) => ({ id: z.id, name: z.name })) ?? [];
}

export function Profiles() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const [isCreating, setIsCreating] = useState(false);
  const [editingProfile, setEditingProfile] = useState<Profile | null>(null);
  const [error, setError] = useState<string | null>(null);
  const zones = useDriverZones();
  const zoneName = (id: number) => zones.find((z) => z.id === id)?.name ?? `Zone ${id}`;

  const { data: profiles, isLoading } = useQuery({
    queryKey: ['profiles'],
    queryFn: profilesApi.list,
  });
  const profileList = (profiles ?? []) as ProfileSummary[];

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ['profiles'] });
    // The controller's active-profile set can change — refresh live state too.
    queryClient.invalidateQueries({ queryKey: ['monitoring'] });
  };
  const onErr = (e: Error) => {
    setError(e.message);
    toast.error(e.message);
  };

  const createMutation = useMutation({
    mutationFn: profilesApi.create,
    onSuccess: () => { invalidate(); setIsCreating(false); toast.success('Profile created'); },
    onError: onErr,
  });
  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: number; data: unknown }) => profilesApi.update(id, data),
    onSuccess: () => { invalidate(); setEditingProfile(null); toast.success('Profile saved'); },
    onError: onErr,
  });
  const deleteMutation = useMutation({
    mutationFn: profilesApi.delete,
    onSuccess: () => { invalidate(); toast.success('Profile deleted'); },
    onError: onErr,
  });
  const activateMutation = useMutation({
    mutationFn: profilesApi.activate,
    onSuccess: () => { invalidate(); toast.success('Profile activated'); },
    onError: onErr,
  });
  const deactivateMutation = useMutation({
    mutationFn: profilesApi.deactivate,
    onSuccess: () => { invalidate(); toast.info('Profile deactivated'); },
    onError: onErr,
  });

  const handleEdit = async (profileId: number) => {
    try {
      setEditingProfile(await profilesApi.get(profileId) as Profile);
    } catch (e) {
      setError((e as Error).message);
    }
  };

  const handleDelete = (p: ProfileSummary) => {
    if (window.confirm(`Delete profile "${p.name}"? This cannot be undone.`)) {
      deleteMutation.mutate(p.id);
    }
  };

  const getZoneDisplayNames = (z: number[] | undefined): string =>
    !z || z.length === 0 ? 'No zones' : z.map(zoneName).join(', ');

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-fg">Profiles</h1>
        <Button onClick={() => setIsCreating(true)}>
          <Plus className="w-4 h-4 mr-2" />
          New Profile
        </Button>
      </div>

      {error && (
        <div className="p-3 bg-danger/15 border border-danger/30 rounded-lg text-danger flex items-center justify-between">
          <span>{error}</span>
          <button className="text-danger hover:text-danger" onClick={() => setError(null)}>✕</button>
        </div>
      )}

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {isLoading && profileList.length === 0 &&
          Array.from({ length: 3 }).map((_, i) => (
            <Card key={`skel-${i}`}>
              <div className="space-y-4" aria-hidden="true">
                <div className="flex items-center gap-3">
                  <div className="skeleton w-10 h-10 rounded-lg" />
                  <div className="flex-1 space-y-2">
                    <div className="skeleton h-4 w-1/2" />
                    <div className="skeleton h-3 w-1/3" />
                  </div>
                </div>
                <div className="skeleton h-3 w-3/4" />
                <div className="skeleton h-8 w-full" />
              </div>
            </Card>
          ))}
        {profileList.map((profile) => (
          <Card key={profile.id}>
            <div className="space-y-4">
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-3">
                  <div className={`p-2 rounded-lg ${profile.is_active ? 'bg-ok/15' : 'bg-surface-2'}`}>
                    <Settings2 className={`w-6 h-6 ${profile.is_active ? 'text-ok' : 'text-muted-2'}`} />
                  </div>
                  <div>
                    <h3 className="font-semibold text-fg-2">{profile.name}</h3>
                    <p className="text-sm text-muted capitalize">{profile.algorithm}</p>
                  </div>
                </div>
                {profile.is_active && <CheckCircle className="w-5 h-5 text-ok" />}
              </div>

              {profile.description && <p className="text-sm text-muted">{profile.description}</p>}

              <div className="flex flex-col gap-1 text-sm text-muted">
                <div className="flex items-center gap-1">
                  <Layers className="w-4 h-4" />
                  <span>{getZoneDisplayNames(profile.zones)}</span>
                </div>
                <span>{profile.input_count} input{profile.input_count !== 1 ? 's' : ''}</span>
              </div>

              <div className="flex gap-2">
                {profile.is_active ? (
                  <Button variant="secondary" size="sm" className="flex-1"
                    onClick={() => deactivateMutation.mutate(profile.id)} isLoading={deactivateMutation.isPending}>
                    <Pause className="w-4 h-4 mr-1" /> Deactivate
                  </Button>
                ) : (
                  <Button size="sm" className="flex-1"
                    onClick={() => activateMutation.mutate(profile.id)} isLoading={activateMutation.isPending}>
                    <Play className="w-4 h-4 mr-1" /> Activate
                  </Button>
                )}
                <Button variant="secondary" size="sm" onClick={() => handleEdit(profile.id)} aria-label={`Edit ${profile.name}`}>
                  <Edit2 className="w-4 h-4" />
                </Button>
                <Button variant="danger" size="sm" onClick={() => handleDelete(profile)} isLoading={deleteMutation.isPending} aria-label={`Delete ${profile.name}`}>
                  <Trash2 className="w-4 h-4" />
                </Button>
              </div>
            </div>
          </Card>
        ))}

        {profileList.length === 0 && !isLoading && (
          <div className="col-span-full text-center py-12">
            <Settings2 className="w-12 h-12 text-surface-3 mx-auto mb-4" />
            <p className="text-muted">No profiles created</p>
            <p className="text-sm text-muted-2 mt-1">Create a profile to configure fan control</p>
          </div>
        )}
      </div>

      <ProfileEditor
        isOpen={isCreating || !!editingProfile}
        onClose={() => { setIsCreating(false); setEditingProfile(null); }}
        profile={editingProfile}
        zones={zones}
        onSave={(data) => {
          if (editingProfile) updateMutation.mutate({ id: editingProfile.id, data });
          else createMutation.mutate(data as never);
        }}
        isLoading={createMutation.isPending || updateMutation.isPending}
      />
    </div>
  );
}

interface ProfileEditorProps {
  isOpen: boolean;
  onClose: () => void;
  profile: Profile | null;
  zones: ZoneOption[];
  onSave: (data: unknown) => void;
  isLoading: boolean;
}

interface InputTypeConfig { value: string; label: string; needsIndex: boolean }
const INPUT_TYPES: InputTypeConfig[] = [
  { value: 'gpu_temp', label: 'GPU Temperature', needsIndex: true },
  { value: 'gpu_load', label: 'GPU Load', needsIndex: true },
  { value: 'cpu_temp', label: 'CPU Temperature', needsIndex: true },
  { value: 'drive_temp', label: 'Drive Temperature', needsIndex: true },
  { value: 'board_temp', label: 'Board Temperature', needsIndex: true },
  { value: 'cpu_load', label: 'CPU Load', needsIndex: false },
];

const inputKey = (i: ProfileInput) => `${i.input_type}-${i.input_index}`;

function InputGroup({ title, icon: Icon, iconColor, children, defaultExpanded = true }: {
  title: string; icon: React.ComponentType<{ className?: string }>; iconColor: string;
  children: React.ReactNode; defaultExpanded?: boolean;
}) {
  const [expanded, setExpanded] = useState(defaultExpanded);
  return (
    <div className="border border-surface-3 rounded-lg overflow-hidden">
      <button type="button" onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center justify-between p-2 bg-surface-2/50 hover:bg-surface-2 transition-colors">
        <div className="flex items-center gap-2">
          <Icon className={`w-4 h-4 ${iconColor}`} />
          <span className="text-sm font-medium text-fg-2">{title}</span>
        </div>
        {expanded ? <ChevronUp className="w-4 h-4 text-muted" /> : <ChevronDown className="w-4 h-4 text-muted" />}
      </button>
      {expanded && <div className="p-2 space-y-1 max-h-48 overflow-y-auto">{children}</div>}
    </div>
  );
}

function ProfileEditor({ isOpen, onClose, profile, zones, onSave, isLoading }: ProfileEditorProps) {
  const { data: monitoring } = useMonitoring();
  const gpuCount = monitoring?.gpus?.length || 0;
  const cpuCount = monitoring?.system?.cpu_packages?.length || 1;
  const driveCount = monitoring?.system?.drives?.length || 0;
  const boardTemps = monitoring?.system?.board_temps || [];

  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [algorithm, setAlgorithm] = useState<'linear' | 'step' | 'pid'>('linear');
  const [priority, setPriority] = useState(0);
  const [selectedZones, setSelectedZones] = useState<number[]>([]);
  const [selectedInputs, setSelectedInputs] = useState<ProfileInput[]>([]);
  const [inputAggregation, setInputAggregation] = useState<InputAggregation>('or');
  const [smoothTransition, setSmoothTransition] = useState(true);
  const [transitionTime, setTransitionTime] = useState(10);
  const [minRunTime, setMinRunTime] = useState(30);
  const [hysteresis, setHysteresis] = useState(2);

  const [linearParams, setLinearParams] = useState<LinearParams>({ min_temp: 30, max_temp: 80, min_speed: 30, max_speed: 100 });
  const [stepParams, setStepParams] = useState<StepParams>({
    steps: [{ temp: 30, speed: 30 }, { temp: 50, speed: 50 }, { temp: 70, speed: 75 }, { temp: 80, speed: 100 }],
  });
  const [pidParams, setPidParams] = useState<PIDParams>({ setpoint: 70, kp: 2.0, ki: 0.1, kd: 1.0, min_speed: 30, max_speed: 100 });

  // Unsaved-changes tracking: capture a snapshot of the seeded form, compare
  // against the live form to warn before discarding edits on close.
  const pristineRef = React.useRef('');
  const [pendingSnapshot, setPendingSnapshot] = useState(false);

  useEffect(() => {
    if (profile) {
      setName(profile.name);
      setDescription(profile.description || '');
      setAlgorithm(profile.algorithm);
      setPriority(profile.priority ?? 0);
      setSelectedZones(profile.zones || []);
      setSelectedInputs(profile.inputs || []);
      setInputAggregation(
        (profile.algorithm_params as AlgorithmParams & { input_aggregation?: InputAggregation })?.input_aggregation || 'or'
      );
      setSmoothTransition(profile.smooth_transition ?? true);
      setTransitionTime(profile.transition_time ?? 10);
      setMinRunTime(profile.min_run_time ?? 30);
      setHysteresis(profile.hysteresis ?? 2);
      if (profile.algorithm === 'linear') setLinearParams(profile.algorithm_params as LinearParams);
      else if (profile.algorithm === 'step') setStepParams(profile.algorithm_params as StepParams);
      else if (profile.algorithm === 'pid') setPidParams(profile.algorithm_params as PIDParams);
    } else {
      setName(''); setDescription(''); setAlgorithm('linear'); setPriority(0);
      setSelectedZones([]); setSelectedInputs([]); setInputAggregation('or');
      setSmoothTransition(true); setTransitionTime(10); setMinRunTime(30); setHysteresis(2);
      setLinearParams({ min_temp: 30, max_temp: 80, min_speed: 30, max_speed: 100 });
    }
    // Capture the seeded state as the pristine baseline once it settles.
    setPendingSnapshot(true);
  }, [profile, isOpen]);

  const currentSnapshot = useMemo(
    () =>
      JSON.stringify({
        name, description, algorithm, priority, selectedZones, selectedInputs,
        inputAggregation, smoothTransition, transitionTime, minRunTime, hysteresis,
        linearParams, stepParams, pidParams,
      }),
    [name, description, algorithm, priority, selectedZones, selectedInputs, inputAggregation,
      smoothTransition, transitionTime, minRunTime, hysteresis, linearParams, stepParams, pidParams]
  );

  useEffect(() => {
    if (pendingSnapshot) {
      pristineRef.current = currentSnapshot;
      setPendingSnapshot(false);
    }
  }, [pendingSnapshot, currentSnapshot]);

  const isDirty = !pendingSnapshot && currentSnapshot !== pristineRef.current;

  const handleClose = () => {
    if (isDirty && !window.confirm('Discard unsaved changes to this profile?')) return;
    onClose();
  };

  const liveValue = (type: string, index: number): number | null => {
    switch (type) {
      case 'gpu_temp': return monitoring?.gpus?.[index]?.temperature ?? null;
      case 'gpu_load': return monitoring?.gpus?.[index]?.load ?? null;
      case 'cpu_temp': return monitoring?.system?.cpu_packages?.[index]?.temperature ?? null;
      case 'drive_temp': return monitoring?.system?.drives?.[index]?.temperature ?? null;
      case 'board_temp': return monitoring?.system?.board_temps?.[index]?.temperature ?? null;
      case 'cpu_load': return monitoring?.system?.cpu_load ?? null;
      default: return null;
    }
  };

  // Aggregated current input value (mirrors the backend), for the "you are here" marker.
  const currentInput = useMemo(() => {
    const vals: number[] = []; const weights: number[] = [];
    for (const inp of selectedInputs) {
      const v = liveValue(inp.input_type, inp.input_index);
      if (v != null) { vals.push(v); weights.push(inp.weight || 1); }
    }
    if (vals.length === 0) return null;
    switch (inputAggregation) {
      case 'and': case 'min': return Math.min(...vals);
      case 'avg': return vals.reduce((a, b) => a + b, 0) / vals.length;
      case 'weighted': {
        const wsum = weights.reduce((a, b) => a + b, 0);
        return wsum ? vals.reduce((a, v, i) => a + v * weights[i], 0) / wsum : Math.max(...vals);
      }
      default: return Math.max(...vals);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedInputs, inputAggregation, monitoring]);

  const toggleInput = (type: string, index: number) => {
    const key = `${type}-${index}`;
    const existing = selectedInputs.findIndex((i) => inputKey(i) === key);
    if (existing >= 0) setSelectedInputs(selectedInputs.filter((_, i) => i !== existing));
    else setSelectedInputs([...selectedInputs, { input_type: type, input_index: index, weight: 1.0 }]);
  };
  const isSelected = (type: string, index: number) => selectedInputs.some((i) => i.input_type === type && i.input_index === index);
  const setWeight = (key: string, w: number) =>
    setSelectedInputs(selectedInputs.map((i) => (inputKey(i) === key ? { ...i, weight: w } : i)));

  const tempColor = (t: number) => (t < 50 ? 'text-info' : t < 65 ? 'text-ok' : t < 80 ? 'text-warn' : 'text-danger');

  const renderCheckbox = (type: InputTypeConfig, index = 0, label?: string) => {
    const v = liveValue(type.value, index);
    const isTemp = type.value.includes('temp');
    return (
      <label key={`${type.value}-${index}`}
        className="flex items-center justify-between p-2 bg-surface-2/30 rounded cursor-pointer hover:bg-surface-2/50">
        <div className="flex items-center gap-2">
          <input type="checkbox" checked={isSelected(type.value, index)} onChange={() => toggleInput(type.value, index)} className="rounded" />
          <span className="text-sm text-fg-3">{label || type.label}</span>
        </div>
        {v !== null && (
          <span className={`text-sm font-mono ${isTemp ? tempColor(v) : 'text-muted'}`}>
            {v.toFixed(isTemp ? 0 : 1)}{isTemp ? '°C' : '%'}
          </span>
        )}
      </label>
    );
  };

  const addStep = () => {
    const last = stepParams.steps[stepParams.steps.length - 1];
    setStepParams({ steps: [...stepParams.steps, { temp: last.temp + 10, speed: Math.min(last.speed + 10, 100) }] });
  };
  const removeStep = (i: number) => stepParams.steps.length > 1 && setStepParams({ steps: stepParams.steps.filter((_, x) => x !== i) });
  const updateStep = (i: number, field: 'temp' | 'speed', value: number) => {
    const steps = [...stepParams.steps]; steps[i] = { ...steps[i], [field]: value }; setStepParams({ steps });
  };

  // Client-side validation mirroring the backend.
  const validationError = useMemo((): string | null => {
    if (!name.trim()) return 'Name is required';
    if (selectedZones.length === 0) return 'Select at least one target zone';
    if (algorithm === 'linear') {
      if (linearParams.min_temp >= linearParams.max_temp) return 'Min temp must be below max temp';
      if (linearParams.min_speed > linearParams.max_speed) return 'Min speed must be ≤ max speed';
      if ([linearParams.min_speed, linearParams.max_speed].some((s) => s < 0 || s > 100)) return 'Speeds must be 0–100%';
    }
    if (algorithm === 'pid') {
      if (pidParams.ki < 0 || pidParams.kd < 0) return 'PID gains must be ≥ 0';
      if (pidParams.min_speed > pidParams.max_speed) return 'Min speed must be ≤ max speed';
    }
    if (transitionTime < 0 || transitionTime > 300) return 'Transition time must be 0–300s';
    if (minRunTime < 0 || minRunTime > 300) return 'Min run time must be 0–300s';
    if (hysteresis < 0 || hysteresis > 10) return 'Hysteresis must be 0–10°C';
    return null;
  }, [name, selectedZones, algorithm, linearParams, pidParams, transitionTime, minRunTime, hysteresis]);

  const handleSave = () => {
    let algorithmParams: Record<string, unknown>;
    if (algorithm === 'linear') algorithmParams = { ...linearParams };
    else if (algorithm === 'step') algorithmParams = { ...stepParams };
    else algorithmParams = { ...pidParams };
    algorithmParams.input_aggregation = inputAggregation;
    onSave({
      name, description: description || undefined, algorithm, algorithm_params: algorithmParams,
      priority, zones: selectedZones, inputs: selectedInputs,
      smooth_transition: smoothTransition, transition_time: transitionTime,
      min_run_time: minRunTime, hysteresis,
    });
  };

  return (
    <Modal isOpen={isOpen} onClose={handleClose} title={profile ? 'Edit Profile' : 'Create Profile'} size="lg">
      <div className="space-y-4 max-h-[72vh] overflow-y-auto pr-1">
        {/* Basic Info */}
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
          <div className="sm:col-span-1">
            <label className="input-label">Name</label>
            <input type="text" className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g., Gaming" />
          </div>
          <div>
            <label className="input-label">Algorithm</label>
            <select className="select" value={algorithm} onChange={(e) => setAlgorithm(e.target.value as 'linear' | 'step' | 'pid')}>
              <option value="linear">Linear</option>
              <option value="step">Step</option>
              <option value="pid">PID</option>
            </select>
          </div>
          <div>
            <label className="input-label">Priority</label>
            <input type="number" className="input" value={priority} onChange={(e) => setPriority(parseInt(e.target.value) || 0)} />
          </div>
        </div>

        <div>
          <label className="input-label">Description</label>
          <input type="text" className="input" value={description} onChange={(e) => setDescription(e.target.value)} placeholder="Optional" />
        </div>

        {/* Algorithm Settings + curve preview */}
        <div className="border-t border-surface-2 pt-4 grid grid-cols-1 lg:grid-cols-2 gap-4">
          <div>
            <h4 className="font-medium text-fg-2 mb-3">Algorithm Settings</h4>
            {algorithm === 'linear' && (
              <div className="grid grid-cols-2 gap-3">
                <div><label className="input-label">Min Temp (°C)</label>
                  <input type="number" className="input" value={linearParams.min_temp}
                    onChange={(e) => setLinearParams({ ...linearParams, min_temp: parseFloat(e.target.value) || 0 })} /></div>
                <div><label className="input-label">Max Temp (°C)</label>
                  <input type="number" className="input" value={linearParams.max_temp}
                    onChange={(e) => setLinearParams({ ...linearParams, max_temp: parseFloat(e.target.value) || 0 })} /></div>
                <div><label className="input-label">Min Speed (%)</label>
                  <input type="number" className="input" value={linearParams.min_speed}
                    onChange={(e) => setLinearParams({ ...linearParams, min_speed: parseInt(e.target.value) || 0 })} /></div>
                <div><label className="input-label">Max Speed (%)</label>
                  <input type="number" className="input" value={linearParams.max_speed}
                    onChange={(e) => setLinearParams({ ...linearParams, max_speed: parseInt(e.target.value) || 0 })} /></div>
              </div>
            )}
            {algorithm === 'step' && (
              <div className="space-y-2">
                {stepParams.steps.map((step, i) => (
                  <div key={i} className="flex gap-2 items-center">
                    <input type="number" className="input w-20" value={step.temp} onChange={(e) => updateStep(i, 'temp', parseFloat(e.target.value) || 0)} />
                    <span className="text-muted">°C →</span>
                    <input type="number" className="input w-20" value={step.speed} onChange={(e) => updateStep(i, 'speed', parseInt(e.target.value) || 0)} />
                    <span className="text-muted">%</span>
                    <Button variant="danger" size="sm" onClick={() => removeStep(i)} disabled={stepParams.steps.length <= 1}>
                      <Trash2 className="w-4 h-4" />
                    </Button>
                  </div>
                ))}
                <Button variant="secondary" size="sm" onClick={addStep}><Plus className="w-4 h-4 mr-1" /> Add Step</Button>
              </div>
            )}
            {algorithm === 'pid' && (
              <div className="grid grid-cols-2 gap-3">
                <div><label className="input-label">Target Temp (°C)</label>
                  <input type="number" className="input" value={pidParams.setpoint} onChange={(e) => setPidParams({ ...pidParams, setpoint: parseFloat(e.target.value) || 0 })} /></div>
                <div><label className="input-label">Kp</label>
                  <input type="number" step="0.1" className="input" value={pidParams.kp} onChange={(e) => setPidParams({ ...pidParams, kp: parseFloat(e.target.value) || 0 })} /></div>
                <div><label className="input-label">Ki</label>
                  <input type="number" step="0.01" className="input" value={pidParams.ki} onChange={(e) => setPidParams({ ...pidParams, ki: parseFloat(e.target.value) || 0 })} /></div>
                <div><label className="input-label">Kd</label>
                  <input type="number" step="0.1" className="input" value={pidParams.kd} onChange={(e) => setPidParams({ ...pidParams, kd: parseFloat(e.target.value) || 0 })} /></div>
                <div><label className="input-label">Min Speed (%)</label>
                  <input type="number" className="input" value={pidParams.min_speed} onChange={(e) => setPidParams({ ...pidParams, min_speed: parseInt(e.target.value) || 0 })} /></div>
                <div><label className="input-label">Max Speed (%)</label>
                  <input type="number" className="input" value={pidParams.max_speed} onChange={(e) => setPidParams({ ...pidParams, max_speed: parseInt(e.target.value) || 0 })} /></div>
              </div>
            )}
          </div>
          <div>
            <h4 className="font-medium text-fg-2 mb-3">Curve Preview</h4>
            <CurvePreview
              algorithm={algorithm}
              linear={linearParams}
              step={stepParams}
              pid={pidParams}
              currentInput={currentInput}
            />
          </div>
        </div>

        {/* Inputs */}
        <div className="border-t border-surface-2 pt-4">
          <div className="flex items-center justify-between mb-3">
            <h4 className="font-medium text-fg-2">Temperature / Load Inputs</h4>
            <div className="flex items-center gap-2">
              <label className="text-xs text-muted">Combine:</label>
              <select className="select py-1 text-sm w-40" value={inputAggregation} onChange={(e) => setInputAggregation(e.target.value as InputAggregation)}>
                <option value="or">Max (respond to hottest)</option>
                <option value="and">Min (all must be cool)</option>
                <option value="avg">Average</option>
                <option value="weighted">Weighted average</option>
              </select>
            </div>
          </div>

          <div className="space-y-2">
            {gpuCount > 0 && (
              <InputGroup title={`GPUs (${gpuCount})`} icon={Activity} iconColor="text-purple-400">
                {Array.from({ length: gpuCount }, (_, i) => (
                  <React.Fragment key={`gpu-${i}`}>
                    {renderCheckbox(INPUT_TYPES[0], i, `GPU ${i} Temperature${monitoring?.gpus?.[i]?.name ? ` — ${monitoring.gpus[i].name}` : ''}`)}
                    {renderCheckbox(INPUT_TYPES[1], i, `GPU ${i} Load`)}
                  </React.Fragment>
                ))}
              </InputGroup>
            )}
            <InputGroup title={`CPUs (${cpuCount})`} icon={Cpu} iconColor="text-info">
              {Array.from({ length: Math.max(cpuCount, 1) }, (_, i) =>
                renderCheckbox(INPUT_TYPES[2], i, `CPU ${i}${monitoring?.system?.cpu_packages?.[i]?.name ? ` — ${monitoring.system.cpu_packages[i].name}` : ''}`))}
              {renderCheckbox(INPUT_TYPES[5])}
            </InputGroup>
            {driveCount > 0 && (
              <InputGroup title={`Drives (${driveCount})`} icon={HardDrive} iconColor="text-cyan-400" defaultExpanded={driveCount <= 6}>
                {Array.from({ length: driveCount }, (_, i) => {
                  const d = monitoring?.system?.drives?.[i];
                  return renderCheckbox(INPUT_TYPES[3], i, `${d?.device || `Drive ${i}`}${d?.model ? ` — ${d.model}` : ''}`);
                })}
              </InputGroup>
            )}
            {boardTemps.length > 0 && (
              <InputGroup title={`Board sensors (${boardTemps.length})`} icon={Thermometer} iconColor="text-orange-400" defaultExpanded={false}>
                {boardTemps.map((bt) => renderCheckbox(INPUT_TYPES[4], bt.index, bt.name))}
              </InputGroup>
            )}
          </div>

          {/* Selected inputs + per-input weights */}
          {selectedInputs.length > 0 && (
            <div className="mt-3 space-y-1">
              <p className="text-xs text-muted">Selected inputs{inputAggregation === 'weighted' ? ' (weights used)' : ''}:</p>
              {selectedInputs.map((inp) => (
                <div key={inputKey(inp)} className="flex items-center justify-between text-sm bg-surface-2/30 rounded px-2 py-1">
                  <span className="text-fg-3">{inp.input_type}{INPUT_TYPES.find((t) => t.value === inp.input_type)?.needsIndex ? ` #${inp.input_index}` : ''}</span>
                  <div className="flex items-center gap-2">
                    <label className="text-xs text-muted-2">weight</label>
                    <input type="number" step="0.5" min="0" className="input w-20 py-1"
                      value={inp.weight} disabled={inputAggregation !== 'weighted'}
                      onChange={(e) => setWeight(inputKey(inp), parseFloat(e.target.value) || 0)} />
                    <button type="button" className="text-muted-2 hover:text-danger" onClick={() => toggleInput(inp.input_type, inp.input_index)}>✕</button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Zones */}
        <div className="border-t border-surface-2 pt-4">
          <h4 className="font-medium text-fg-2 mb-3">Target Zones</h4>
          {zones.length > 0 ? (
            <>
              <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
                {zones.map((zone) => (
                  <label key={zone.id} className={`flex items-center gap-2 p-2 rounded-lg cursor-pointer transition-colors ${
                    selectedZones.includes(zone.id) ? 'bg-info/15 border border-primary-600' : 'bg-surface-2/50 border border-surface-3 hover:border-muted-2'}`}>
                    <input type="checkbox" checked={selectedZones.includes(zone.id)} className="rounded"
                      onChange={(e) => setSelectedZones(e.target.checked ? [...selectedZones, zone.id] : selectedZones.filter((z) => z !== zone.id))} />
                    <span className="text-sm text-fg-2">{zone.name}</span>
                  </label>
                ))}
              </div>
              <p className="text-xs text-muted-2 mt-2">
                {selectedZones.length === 0 ? 'No zones selected' : `Controls ${selectedZones.length} zone(s)`}
              </p>
            </>
          ) : (
            <p className="text-sm text-muted">No control zones available. Detect fans first on the Fans page.</p>
          )}
        </div>

        {/* Advanced tuning */}
        <div className="border-t border-surface-2 pt-4">
          <h4 className="font-medium text-fg-2 mb-3">Advanced</h4>
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 items-end">
            <label className="flex items-center gap-2 text-sm text-fg-3">
              <input type="checkbox" checked={smoothTransition} onChange={(e) => setSmoothTransition(e.target.checked)} className="rounded" />
              Smooth transitions
            </label>
            <div><label className="input-label">Transition (s)</label>
              <input type="number" className="input" value={transitionTime} onChange={(e) => setTransitionTime(parseInt(e.target.value) || 0)} /></div>
            <div><label className="input-label">Min run (s)</label>
              <input type="number" className="input" value={minRunTime} onChange={(e) => setMinRunTime(parseInt(e.target.value) || 0)} /></div>
            <div><label className="input-label">Hysteresis (°C)</label>
              <input type="number" step="0.5" className="input" value={hysteresis} onChange={(e) => setHysteresis(parseFloat(e.target.value) || 0)} /></div>
          </div>
        </div>

        {/* Actions */}
        <div className="flex justify-between items-center gap-2 pt-4 border-t border-surface-2 sticky bottom-0 bg-surface">
          <span className="text-sm text-danger">{validationError}</span>
          <div className="flex gap-2">
            <Button variant="secondary" onClick={handleClose}>Cancel</Button>
            <Button onClick={handleSave} isLoading={isLoading} disabled={!!validationError}>
              {profile ? 'Update' : 'Create'}
            </Button>
          </div>
        </div>
      </div>
    </Modal>
  );
}

function CurvePreview({ algorithm, linear, step, pid, currentInput }: {
  algorithm: 'linear' | 'step' | 'pid';
  linear: LinearParams; step: StepParams; pid: PIDParams; currentInput: number | null;
}) {
  const data = useMemo(() => {
    if (algorithm === 'linear') {
      const { min_temp, max_temp, min_speed, max_speed } = linear;
      const lo = Math.min(min_temp, max_temp) - 5;
      const hi = Math.max(min_temp, max_temp) + 5;
      const pts: { temp: number; speed: number }[] = [];
      for (let t = lo; t <= hi; t += Math.max((hi - lo) / 40, 1)) {
        let s: number;
        if (t <= min_temp) s = min_speed;
        else if (t >= max_temp) s = max_speed;
        else s = min_speed + ((t - min_temp) / (max_temp - min_temp)) * (max_speed - min_speed);
        pts.push({ temp: Math.round(t), speed: Math.max(0, Math.min(100, Math.round(s))) });
      }
      return pts;
    }
    if (algorithm === 'step') {
      return [...step.steps].sort((a, b) => a.temp - b.temp).map((s) => ({ temp: s.temp, speed: s.speed }));
    }
    // PID: show the setpoint band conceptually as a ramp around the target.
    const { setpoint, min_speed, max_speed } = pid;
    return [
      { temp: setpoint - 20, speed: min_speed },
      { temp: setpoint, speed: Math.round((min_speed + max_speed) / 2) },
      { temp: setpoint + 20, speed: max_speed },
    ];
  }, [algorithm, linear, step, pid]);

  return (
    <div className="h-48 bg-app/40 rounded-lg p-2">
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={data} margin={{ top: 8, right: 12, bottom: 4, left: -20 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="#334155" />
          <XAxis dataKey="temp" type="number" domain={['dataMin', 'dataMax']} stroke="#94a3b8" fontSize={11}
            tickFormatter={(v) => `${v}°`} />
          <YAxis domain={[0, 100]} stroke="#94a3b8" fontSize={11} tickFormatter={(v) => `${v}%`} />
          <Tooltip contentStyle={{ background: '#1e293b', border: '1px solid #334155', fontSize: 12 }}
            formatter={(v: number) => [`${v}%`, 'Duty']} labelFormatter={(l) => `${l}°C`} />
          <Line type={algorithm === 'step' ? 'stepAfter' : 'monotone'} dataKey="speed" stroke="#38bdf8" strokeWidth={2} dot={false} isAnimationActive={false} />
          {currentInput != null && (
            <ReferenceLine x={Math.round(currentInput)} stroke="#f59e0b" strokeDasharray="4 2"
              label={{ value: `now ${Math.round(currentInput)}°`, fill: '#f59e0b', fontSize: 11, position: 'top' }} />
          )}
          {algorithm === 'pid' && (
            <ReferenceLine x={pid.setpoint} stroke="#22c55e" strokeDasharray="2 2" />
          )}
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}
