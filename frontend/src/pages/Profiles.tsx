import React, { useState, useEffect } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { profilesApi, settingsApi } from '../services/api';
import { useMonitoring } from '../hooks/useMonitoring';
import { Card } from '../components/common/Card';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
import { ProfileSummary, Profile, ProfileInput, LinearParams, StepParams, PIDParams, AlgorithmParams } from '../types/profile';
import { ZoneLayout } from '../types/zone';
import { getZoneName } from '../utils/zone';
import { AppSettings } from '../types/settings';
import { 
  Plus, 
  Play, 
  Pause, 
  Trash2, 
  Edit2,
  Settings2,
  CheckCircle,
  ChevronDown,
  ChevronUp,
  Cpu,
  HardDrive,
  Activity,
  Layers,
  AlertTriangle
} from 'lucide-react';

export function Profiles() {
  const queryClient = useQueryClient();
  const [isCreating, setIsCreating] = useState(false);
  const [editingProfile, setEditingProfile] = useState<Profile | null>(null);

  const { data: profiles, isLoading } = useQuery({
    queryKey: ['profiles'],
    queryFn: profilesApi.list,
  });

  const { data: settings } = useQuery<AppSettings>({
    queryKey: ['settings'],
    queryFn: settingsApi.get,
  });

  const zoneLayout = settings?.zone_layout ?? null;

  const createMutation = useMutation({
    mutationFn: profilesApi.create,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['profiles'] });
      setIsCreating(false);
    },
  });

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: number; data: unknown }) => profilesApi.update(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['profiles'] });
      setEditingProfile(null);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: profilesApi.delete,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['profiles'] });
    },
  });

  const activateMutation = useMutation({
    mutationFn: profilesApi.activate,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['profiles'] });
    },
  });

  const deactivateMutation = useMutation({
    mutationFn: profilesApi.deactivate,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['profiles'] });
    },
  });

  const handleEdit = async (profileId: number) => {
    const profile = await profilesApi.get(profileId) as Profile;
    setEditingProfile(profile);
  };

  // Get zone display names for profiles
  const getZoneDisplayNames = (zones: number[] | undefined): string => {
    if (!zones || zones.length === 0) return 'All zones';
    return zones.map(id => getZoneName(zoneLayout, id)).join(', ');
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-slate-100">Profiles</h1>
        <Button onClick={() => setIsCreating(true)}>
          <Plus className="w-4 h-4 mr-2" />
          New Profile
        </Button>
      </div>

      {/* No zone layout warning */}
      {!zoneLayout && (
        <div className="p-4 bg-amber-900/30 border border-amber-700 rounded-lg">
          <div className="flex items-center gap-2 text-amber-400 mb-2">
            <AlertTriangle className="w-5 h-5" />
            <span className="font-medium">No zone layout configured</span>
          </div>
          <p className="text-sm text-slate-400">
            Go to the Fans page and configure zones before creating profiles. Profiles require zones to control fan speeds.
          </p>
        </div>
      )}

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {(profiles as ProfileSummary[] || []).map((profile) => (
          <Card key={profile.id}>
            <div className="space-y-4">
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-3">
                  <div className={`p-2 rounded-lg ${profile.is_active ? 'bg-green-900/50' : 'bg-slate-700'}`}>
                    <Settings2 className={`w-6 h-6 ${profile.is_active ? 'text-green-400' : 'text-slate-500'}`} />
                  </div>
                  <div>
                    <h3 className="font-semibold text-slate-200">{profile.name}</h3>
                    <p className="text-sm text-slate-400 capitalize">{profile.algorithm}</p>
                  </div>
                </div>
                {profile.is_active && (
                  <CheckCircle className="w-5 h-5 text-green-400" />
                )}
              </div>

              {profile.description && (
                <p className="text-sm text-slate-400">{profile.description}</p>
              )}

              <div className="flex flex-col gap-1 text-sm text-slate-400">
                <div className="flex items-center gap-1">
                  <Layers className="w-4 h-4" />
                  <span>{getZoneDisplayNames(profile.zones)}</span>
                </div>
                <span>{profile.input_count} input{profile.input_count !== 1 ? 's' : ''}</span>
              </div>

              <div className="flex gap-2">
                {profile.is_active ? (
                  <Button
                    variant="secondary"
                    size="sm"
                    className="flex-1"
                    onClick={() => deactivateMutation.mutate(profile.id)}
                    isLoading={deactivateMutation.isPending}
                  >
                    <Pause className="w-4 h-4 mr-1" />
                    Deactivate
                  </Button>
                ) : (
                  <Button
                    size="sm"
                    className="flex-1"
                    onClick={() => activateMutation.mutate(profile.id)}
                    isLoading={activateMutation.isPending}
                  >
                    <Play className="w-4 h-4 mr-1" />
                    Activate
                  </Button>
                )}
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => handleEdit(profile.id)}
                >
                  <Edit2 className="w-4 h-4" />
                </Button>
                <Button
                  variant="danger"
                  size="sm"
                  onClick={() => deleteMutation.mutate(profile.id)}
                  isLoading={deleteMutation.isPending}
                >
                  <Trash2 className="w-4 h-4" />
                </Button>
              </div>
            </div>
          </Card>
        ))}

        {(profiles as ProfileSummary[] || []).length === 0 && !isLoading && (
          <div className="col-span-full text-center py-12">
            <Settings2 className="w-12 h-12 text-slate-600 mx-auto mb-4" />
            <p className="text-slate-400">No profiles created</p>
            <p className="text-sm text-slate-500 mt-1">Create a profile to configure fan control</p>
          </div>
        )}
      </div>

      {/* Create/Edit Profile Modal */}
      <ProfileEditor
        isOpen={isCreating || !!editingProfile}
        onClose={() => {
          setIsCreating(false);
          setEditingProfile(null);
        }}
        profile={editingProfile}
        zoneLayout={zoneLayout}
        onSave={(data) => {
          if (editingProfile) {
            updateMutation.mutate({ id: editingProfile.id, data });
          } else {
            createMutation.mutate(data);
          }
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
  zoneLayout: ZoneLayout | null;
  onSave: (data: unknown) => void;
  isLoading: boolean;
}

// Input type configuration
interface InputTypeConfig {
  value: string;
  label: string;
  category: 'gpu' | 'cpu' | 'drive' | 'system';
  needsIndex: boolean;
}

const INPUT_TYPES: InputTypeConfig[] = [
  { value: 'gpu_temp', label: 'GPU Temperature', category: 'gpu', needsIndex: true },
  { value: 'gpu_load', label: 'GPU Load', category: 'gpu', needsIndex: true },
  { value: 'cpu_temp', label: 'CPU Temperature', category: 'cpu', needsIndex: true },
  { value: 'drive_temp', label: 'Drive Temperature', category: 'drive', needsIndex: true },
  { value: 'cpu_load', label: 'CPU Load', category: 'system', needsIndex: false },
];

function inputKey(input: ProfileInput): string {
  return `${input.input_type}-${input.input_index}`;
}

// Collapsible input group
function InputGroup({ 
  title, 
  icon: Icon, 
  iconColor, 
  children, 
  defaultExpanded = true 
}: { 
  title: string;
  icon: React.ComponentType<{ className?: string }>;
  iconColor: string;
  children: React.ReactNode;
  defaultExpanded?: boolean;
}) {
  const [expanded, setExpanded] = useState(defaultExpanded);
  
  return (
    <div className="border border-slate-600 rounded-lg overflow-hidden">
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center justify-between p-2 bg-slate-700/50 hover:bg-slate-700 transition-colors"
      >
        <div className="flex items-center gap-2">
          <Icon className={`w-4 h-4 ${iconColor}`} />
          <span className="text-sm font-medium text-slate-200">{title}</span>
        </div>
        {expanded ? <ChevronUp className="w-4 h-4 text-slate-400" /> : <ChevronDown className="w-4 h-4 text-slate-400" />}
      </button>
      {expanded && <div className="p-2 space-y-1 max-h-48 overflow-y-auto">{children}</div>}
    </div>
  );
}

function ProfileEditor({ isOpen, onClose, profile, zoneLayout, onSave, isLoading }: ProfileEditorProps) {
  const { data: monitoring } = useMonitoring();
  
  const gpuCount = monitoring?.gpus?.length || 0;
  const cpuCount = monitoring?.system?.cpu_packages?.length || 1;
  const driveCount = monitoring?.system?.drives?.length || 0;

  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [algorithm, setAlgorithm] = useState<'linear' | 'step' | 'pid'>('linear');
  const [selectedZones, setSelectedZones] = useState<number[]>([]);
  const [selectedInputs, setSelectedInputs] = useState<ProfileInput[]>([]);
  const [inputAggregation, setInputAggregation] = useState<'or' | 'and'>('or');

  const [linearParams, setLinearParams] = useState<LinearParams>({
    min_temp: 30, max_temp: 80, min_speed: 30, max_speed: 100,
  });
  
  const [stepParams, setStepParams] = useState<StepParams>({
    steps: [
      { temp: 30, speed: 30 },
      { temp: 50, speed: 50 },
      { temp: 70, speed: 75 },
      { temp: 80, speed: 100 },
    ],
  });
  
  const [pidParams, setPidParams] = useState<PIDParams>({
    setpoint: 70, kp: 2.0, ki: 0.1, kd: 1.0, min_speed: 30, max_speed: 100,
  });

  // Initialize form from profile
  useEffect(() => {
    if (profile) {
      setName(profile.name);
      setDescription(profile.description || '');
      setAlgorithm(profile.algorithm);
      setSelectedZones(profile.zones || []);
      setSelectedInputs(profile.inputs || []);
      setInputAggregation((profile.algorithm_params as AlgorithmParams & { input_aggregation?: 'or' | 'and' })?.input_aggregation || 'or');
      
      if (profile.algorithm === 'linear') setLinearParams(profile.algorithm_params as LinearParams);
      else if (profile.algorithm === 'step') setStepParams(profile.algorithm_params as StepParams);
      else if (profile.algorithm === 'pid') setPidParams(profile.algorithm_params as PIDParams);
    } else {
      setName('');
      setDescription('');
      setAlgorithm('linear');
      setSelectedZones([]);
      setSelectedInputs([]);
      setInputAggregation('or');
    }
  }, [profile, isOpen]);

  const handleSave = () => {
    let algorithmParams: Record<string, unknown>;
    if (algorithm === 'linear') algorithmParams = { ...linearParams };
    else if (algorithm === 'step') algorithmParams = { ...stepParams };
    else algorithmParams = { ...pidParams };

    algorithmParams.input_aggregation = inputAggregation;

    onSave({
      name,
      description: description || undefined,
      algorithm,
      algorithm_params: algorithmParams,
      zones: selectedZones,
      inputs: selectedInputs,
    });
  };

  const toggleInput = (inputType: string, inputIndex: number) => {
    const key = `${inputType}-${inputIndex}`;
    const existingIndex = selectedInputs.findIndex(i => inputKey(i) === key);
    
    if (existingIndex >= 0) {
      setSelectedInputs(selectedInputs.filter((_, i) => i !== existingIndex));
    } else {
      setSelectedInputs([...selectedInputs, { input_type: inputType, input_index: inputIndex, weight: 1.0 }]);
    }
  };

  const isInputSelected = (inputType: string, inputIndex: number): boolean => {
    return selectedInputs.some(i => i.input_type === inputType && i.input_index === inputIndex);
  };

  const getCurrentTemp = (inputType: string, inputIndex: number): number | null => {
    switch (inputType) {
      case 'gpu_temp':
        return monitoring?.gpus?.[inputIndex]?.temperature ?? null;
      case 'gpu_load':
        return monitoring?.gpus?.[inputIndex]?.load ?? null;
      case 'cpu_temp':
        return monitoring?.system?.cpu_packages?.[inputIndex]?.temperature ?? null;
      case 'drive_temp':
        return monitoring?.system?.drives?.[inputIndex]?.temperature ?? null;
      case 'cpu_load':
        return monitoring?.system?.cpu_load ?? null;
      default:
        return null;
    }
  };

  const getTempColor = (temp: number): string => {
    if (temp < 50) return 'text-blue-400';
    if (temp < 65) return 'text-green-400';
    if (temp < 80) return 'text-yellow-400';
    return 'text-red-400';
  };

  const renderInputCheckbox = (type: InputTypeConfig, index: number = 0, label?: string) => {
    const currentValue = getCurrentTemp(type.value, index);
    const displayLabel = label || type.label;
    const isTemp = type.value.includes('temp');
    
    return (
      <label
        key={`${type.value}-${index}`}
        className="flex items-center justify-between p-2 bg-slate-700/30 rounded cursor-pointer hover:bg-slate-700/50"
      >
        <div className="flex items-center gap-2">
          <input
            type="checkbox"
            checked={isInputSelected(type.value, index)}
            onChange={() => toggleInput(type.value, index)}
            className="rounded"
          />
          <span className="text-sm text-slate-300">{displayLabel}</span>
        </div>
        {currentValue !== null && (
          <span className={`text-sm font-mono ${isTemp ? getTempColor(currentValue) : 'text-slate-400'}`}>
            {currentValue.toFixed(isTemp ? 0 : 1)}{isTemp ? '°C' : '%'}
          </span>
        )}
      </label>
    );
  };

  const addStep = () => {
    const lastStep = stepParams.steps[stepParams.steps.length - 1];
    setStepParams({
      steps: [...stepParams.steps, { temp: lastStep.temp + 10, speed: Math.min(lastStep.speed + 10, 100) }],
    });
  };

  const removeStep = (index: number) => {
    if (stepParams.steps.length > 1) {
      setStepParams({ steps: stepParams.steps.filter((_, i) => i !== index) });
    }
  };

  const updateStep = (index: number, field: 'temp' | 'speed', value: number) => {
    const newSteps = [...stepParams.steps];
    newSteps[index] = { ...newSteps[index], [field]: value };
    setStepParams({ steps: newSteps });
  };

  // Get zones from layout
  const availableZones = zoneLayout?.zones || [];

  return (
    <Modal isOpen={isOpen} onClose={onClose} title={profile ? 'Edit Profile' : 'Create Profile'} size="lg">
      <div className="space-y-4 max-h-[70vh] overflow-y-auto">
        {/* Basic Info */}
        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="input-label">Name</label>
            <input
              type="text"
              className="input"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g., Gaming, Quiet"
            />
          </div>
          <div>
            <label className="input-label">Algorithm</label>
            <select className="select" value={algorithm} onChange={(e) => setAlgorithm(e.target.value as 'linear' | 'step' | 'pid')}>
              <option value="linear">Linear</option>
              <option value="step">Step</option>
              <option value="pid">PID</option>
            </select>
          </div>
        </div>

        <div>
          <label className="input-label">Description</label>
          <input
            type="text"
            className="input"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Optional description"
          />
        </div>

        {/* Algorithm Parameters */}
        <div className="border-t border-slate-700 pt-4">
          <h4 className="font-medium text-slate-200 mb-3">Algorithm Settings</h4>
          
          {algorithm === 'linear' && (
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="input-label">Min Temp (°C)</label>
                <input type="number" className="input" value={linearParams.min_temp}
                  onChange={(e) => setLinearParams({ ...linearParams, min_temp: parseFloat(e.target.value) })} />
              </div>
              <div>
                <label className="input-label">Max Temp (°C)</label>
                <input type="number" className="input" value={linearParams.max_temp}
                  onChange={(e) => setLinearParams({ ...linearParams, max_temp: parseFloat(e.target.value) })} />
              </div>
              <div>
                <label className="input-label">Min Speed (%)</label>
                <input type="number" className="input" value={linearParams.min_speed}
                  onChange={(e) => setLinearParams({ ...linearParams, min_speed: parseInt(e.target.value) })} />
              </div>
              <div>
                <label className="input-label">Max Speed (%)</label>
                <input type="number" className="input" value={linearParams.max_speed}
                  onChange={(e) => setLinearParams({ ...linearParams, max_speed: parseInt(e.target.value) })} />
              </div>
            </div>
          )}

          {algorithm === 'step' && (
            <div className="space-y-2">
              {stepParams.steps.map((step, index) => (
                <div key={index} className="flex gap-2 items-center">
                  <input type="number" className="input w-24" value={step.temp}
                    onChange={(e) => updateStep(index, 'temp', parseFloat(e.target.value))} placeholder="Temp" />
                  <span className="text-slate-400">°C →</span>
                  <input type="number" className="input w-24" value={step.speed}
                    onChange={(e) => updateStep(index, 'speed', parseInt(e.target.value))} placeholder="Speed" />
                  <span className="text-slate-400">%</span>
                  <Button variant="danger" size="sm" onClick={() => removeStep(index)} disabled={stepParams.steps.length <= 1}>
                    <Trash2 className="w-4 h-4" />
                  </Button>
                </div>
              ))}
              <Button variant="secondary" size="sm" onClick={addStep}>
                <Plus className="w-4 h-4 mr-1" />
                Add Step
              </Button>
            </div>
          )}

          {algorithm === 'pid' && (
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="input-label">Target Temp (°C)</label>
                <input type="number" className="input" value={pidParams.setpoint}
                  onChange={(e) => setPidParams({ ...pidParams, setpoint: parseFloat(e.target.value) })} />
              </div>
              <div>
                <label className="input-label">Kp (Proportional)</label>
                <input type="number" step="0.1" className="input" value={pidParams.kp}
                  onChange={(e) => setPidParams({ ...pidParams, kp: parseFloat(e.target.value) })} />
              </div>
              <div>
                <label className="input-label">Ki (Integral)</label>
                <input type="number" step="0.01" className="input" value={pidParams.ki}
                  onChange={(e) => setPidParams({ ...pidParams, ki: parseFloat(e.target.value) })} />
              </div>
              <div>
                <label className="input-label">Kd (Derivative)</label>
                <input type="number" step="0.1" className="input" value={pidParams.kd}
                  onChange={(e) => setPidParams({ ...pidParams, kd: parseFloat(e.target.value) })} />
              </div>
              <div>
                <label className="input-label">Min Speed (%)</label>
                <input type="number" className="input" value={pidParams.min_speed}
                  onChange={(e) => setPidParams({ ...pidParams, min_speed: parseInt(e.target.value) })} />
              </div>
              <div>
                <label className="input-label">Max Speed (%)</label>
                <input type="number" className="input" value={pidParams.max_speed}
                  onChange={(e) => setPidParams({ ...pidParams, max_speed: parseInt(e.target.value) })} />
              </div>
            </div>
          )}
        </div>

        {/* Input Selection */}
        <div className="border-t border-slate-700 pt-4">
          <h4 className="font-medium text-slate-200 mb-3">Temperature/Load Inputs</h4>
          
          {/* Input Aggregation */}
          <div className="mb-4 p-3 bg-slate-700/30 rounded-lg">
            <p className="text-sm text-slate-300 mb-2">Input Logic (when multiple inputs are selected):</p>
            <div className="flex gap-4">
              <label className="flex items-center gap-2 cursor-pointer">
                <input type="radio" name="inputAggregation" value="or" checked={inputAggregation === 'or'}
                  onChange={() => setInputAggregation('or')} className="text-blue-500" />
                <span className="text-sm text-slate-200">OR (highest value wins)</span>
              </label>
              <label className="flex items-center gap-2 cursor-pointer">
                <input type="radio" name="inputAggregation" value="and" checked={inputAggregation === 'and'}
                  onChange={() => setInputAggregation('and')} className="text-blue-500" />
                <span className="text-sm text-slate-200">AND (lowest value wins)</span>
              </label>
            </div>
          </div>

          <p className="text-sm text-slate-400 mb-3">
            Select one or more sensors. Selected: {selectedInputs.length}
          </p>

          <div className="space-y-2">
            {gpuCount > 0 && (
              <InputGroup title={`GPUs (${gpuCount})`} icon={Activity} iconColor="text-purple-400">
                {Array.from({ length: gpuCount }, (_, i) => (
                  <React.Fragment key={`gpu-${i}`}>
                    {renderInputCheckbox(INPUT_TYPES.find(t => t.value === 'gpu_temp')!, i,
                      `GPU ${i} Temperature${monitoring?.gpus?.[i]?.name ? ` - ${monitoring.gpus[i].name}` : ''}`)}
                    {renderInputCheckbox(INPUT_TYPES.find(t => t.value === 'gpu_load')!, i, `GPU ${i} Load`)}
                  </React.Fragment>
                ))}
              </InputGroup>
            )}

            <InputGroup title={`CPUs (${cpuCount})`} icon={Cpu} iconColor="text-blue-400">
              {Array.from({ length: Math.max(cpuCount, 1) }, (_, i) => 
                renderInputCheckbox(INPUT_TYPES.find(t => t.value === 'cpu_temp')!, i,
                  `CPU ${i}${monitoring?.system?.cpu_packages?.[i]?.name ? ` - ${monitoring.system.cpu_packages[i].name}` : ''}`)
              )}
              {renderInputCheckbox(INPUT_TYPES.find(t => t.value === 'cpu_load')!)}
            </InputGroup>

            {driveCount > 0 && (
              <InputGroup title={`Drives (${driveCount})`} icon={HardDrive} iconColor="text-cyan-400" defaultExpanded={driveCount <= 6}>
                {Array.from({ length: driveCount }, (_, i) => {
                  const drive = monitoring?.system?.drives?.[i];
                  return renderInputCheckbox(INPUT_TYPES.find(t => t.value === 'drive_temp')!, i,
                    `${drive?.device || `Drive ${i}`}${drive?.model ? ` - ${drive.model}` : ''}`);
                })}
              </InputGroup>
            )}
          </div>
        </div>

        {/* Zone Selection */}
        <div className="border-t border-slate-700 pt-4">
          <h4 className="font-medium text-slate-200 mb-3">Target Zones</h4>
          <p className="text-sm text-slate-400 mb-3">
            Select which zones this profile controls. Multiple profiles can be active for different zones.
          </p>
          
          {availableZones.length > 0 ? (
            <div className="space-y-2">
              <div className="grid grid-cols-2 gap-2">
                {availableZones.map((zone) => (
                  <label key={zone.id} className={`flex items-center gap-3 p-3 rounded-lg cursor-pointer transition-colors ${
                    selectedZones.includes(zone.id)
                      ? 'bg-blue-900/50 border border-blue-600'
                      : 'bg-slate-700/50 border border-slate-600 hover:border-slate-500'
                  }`}>
                    <input
                      type="checkbox"
                      checked={selectedZones.includes(zone.id)}
                      onChange={(e) => {
                        if (e.target.checked) {
                          setSelectedZones([...selectedZones, zone.id]);
                        } else {
                          setSelectedZones(selectedZones.filter(z => z !== zone.id));
                        }
                      }}
                      className="rounded"
                    />
                    <div>
                      <span className="text-sm font-medium text-slate-200">{zone.name}</span>
                      {zone.description && (
                        <p className="text-xs text-slate-400">{zone.description}</p>
                      )}
                      <p className="text-xs text-slate-500">
                        {zone.fan_indices.length} fan{zone.fan_indices.length !== 1 ? 's' : ''}
                      </p>
                    </div>
                  </label>
                ))}
              </div>
              <p className="text-xs text-slate-500 mt-2">
                {selectedZones.length === 0 
                  ? 'No zones selected - profile will not control any fans'
                  : `Selected: ${selectedZones.map(id => getZoneName(zoneLayout, id)).join(', ')}`}
              </p>
            </div>
          ) : (
            <div className="p-4 bg-amber-900/30 border border-amber-700 rounded-lg">
              <div className="flex items-center gap-2 text-amber-400 mb-1">
                <AlertTriangle className="w-4 h-4" />
                <span className="font-medium text-sm">No zones configured</span>
              </div>
              <p className="text-sm text-slate-400">
                Go to the Fans page to configure zones before creating profiles.
              </p>
            </div>
          )}
        </div>

        {/* Actions */}
        <div className="flex justify-end gap-2 pt-4 border-t border-slate-700">
          <Button variant="secondary" onClick={onClose}>Cancel</Button>
          <Button onClick={handleSave} isLoading={isLoading} disabled={!name || selectedZones.length === 0}>
            {profile ? 'Update' : 'Create'}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
