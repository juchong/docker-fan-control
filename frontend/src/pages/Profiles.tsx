import React, { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { profilesApi, fansApi } from '../services/api';
import { useMonitoring } from '../hooks/useMonitoring';
import { Card } from '../components/common/Card';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
import { ProfileSummary, Profile, ProfileInput, LinearParams, StepParams, PIDParams, AlgorithmParams } from '../types/profile';
import { Fan } from '../types/fan';
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
  Activity
} from 'lucide-react';

export function Profiles() {
  const queryClient = useQueryClient();
  const [isCreating, setIsCreating] = useState(false);
  const [editingProfile, setEditingProfile] = useState<Profile | null>(null);

  const { data: profiles, isLoading } = useQuery({
    queryKey: ['profiles'],
    queryFn: profilesApi.list,
  });

  const { data: fans } = useQuery({
    queryKey: ['fans'],
    queryFn: fansApi.list,
  });

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

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-slate-100">Profiles</h1>
        <Button onClick={() => setIsCreating(true)}>
          <Plus className="w-4 h-4 mr-2" />
          New Profile
        </Button>
      </div>

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

              <div className="flex gap-4 text-sm text-slate-400">
                <span>
                  {profile.zone_count > 0 
                    ? `Zone${profile.zone_count !== 1 ? 's' : ''}: ${profile.zones?.join(', ') || 'None'}`
                    : 'All zones'}
                </span>
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
        fans={fans as Fan[] || []}
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
  fans: Fan[];
  onSave: (data: unknown) => void;
  isLoading: boolean;
}

// Input type definition with categorization
interface InputTypeConfig {
  value: string;
  label: string;
  category: 'aggregate' | 'gpu' | 'cpu' | 'drive' | 'system';
  needsIndex: boolean;
  indexType?: 'gpu' | 'cpu' | 'drive';
}

const INPUT_TYPES: InputTypeConfig[] = [
  // Aggregate types (no index needed)
  { value: 'max_temp', label: 'Max GPU Temperature', category: 'aggregate', needsIndex: false },
  { value: 'avg_temp', label: 'Average GPU Temperature', category: 'aggregate', needsIndex: false },
  { value: 'max_cpu', label: 'Max CPU Temperature', category: 'aggregate', needsIndex: false },
  { value: 'max_drive', label: 'Max Drive Temperature', category: 'aggregate', needsIndex: false },
  // GPU types
  { value: 'gpu_temp', label: 'GPU Temperature', category: 'gpu', needsIndex: true, indexType: 'gpu' },
  { value: 'gpu_load', label: 'GPU Load', category: 'gpu', needsIndex: true, indexType: 'gpu' },
  // CPU types
  { value: 'cpu_temp', label: 'CPU Temperature', category: 'cpu', needsIndex: true, indexType: 'cpu' },
  // Drive types
  { value: 'drive_temp', label: 'Drive Temperature', category: 'drive', needsIndex: true, indexType: 'drive' },
  // System types
  { value: 'cpu_load', label: 'CPU Load', category: 'system', needsIndex: false },
];

// Helper to generate input key for comparison
function inputKey(input: ProfileInput): string {
  return `${input.input_type}-${input.input_index}`;
}

// Collapsible input group component
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
        {expanded ? (
          <ChevronUp className="w-4 h-4 text-slate-400" />
        ) : (
          <ChevronDown className="w-4 h-4 text-slate-400" />
        )}
      </button>
      {expanded && (
        <div className="p-2 space-y-1 max-h-48 overflow-y-auto">
          {children}
        </div>
      )}
    </div>
  );
}

function ProfileEditor({ isOpen, onClose, profile, fans, onSave, isLoading }: ProfileEditorProps) {
  const { data: monitoring } = useMonitoring();
  
  const gpuCount = monitoring?.gpus?.length || 0;
  const cpuCount = monitoring?.system?.cpu_packages?.length || 1;
  const driveCount = monitoring?.system?.drives?.length || 0;

  const [name, setName] = useState(profile?.name || '');
  const [description, setDescription] = useState(profile?.description || '');
  const [algorithm, setAlgorithm] = useState<'linear' | 'step' | 'pid'>(profile?.algorithm || 'linear');
  const [selectedZones, setSelectedZones] = useState<number[]>(profile?.zones || []);
  const [selectedInputs, setSelectedInputs] = useState<ProfileInput[]>(
    profile?.inputs || [{ input_type: 'max_temp', input_index: 0, weight: 1.0 }]
  );
  const [inputAggregation, setInputAggregation] = useState<'or' | 'and'>(
    (profile?.algorithm_params as AlgorithmParams & { input_aggregation?: 'or' | 'and' })?.input_aggregation || 'or'
  );

  // Extract unique zones from fans
  const availableZones = React.useMemo(() => {
    const zones = new Set<number>();
    fans.forEach(fan => {
      if (fan.ipmi_zone !== undefined && fan.ipmi_zone !== null) {
        zones.add(fan.ipmi_zone);
      }
    });
    return Array.from(zones).sort((a, b) => a - b);
  }, [fans]);
  
  // Algorithm params
  const [linearParams, setLinearParams] = useState<LinearParams>(
    profile?.algorithm === 'linear' ? (profile.algorithm_params as LinearParams) : {
      min_temp: 30,
      max_temp: 80,
      min_speed: 30,
      max_speed: 100,
    }
  );
  
  const [stepParams, setStepParams] = useState<StepParams>(
    profile?.algorithm === 'step' ? (profile.algorithm_params as StepParams) : {
      steps: [
        { temp: 30, speed: 30 },
        { temp: 50, speed: 50 },
        { temp: 70, speed: 75 },
        { temp: 80, speed: 100 },
      ],
    }
  );
  
  const [pidParams, setPidParams] = useState<PIDParams>(
    profile?.algorithm === 'pid' ? (profile.algorithm_params as PIDParams) : {
      setpoint: 70,
      kp: 2.0,
      ki: 0.1,
      kd: 1.0,
      min_speed: 30,
      max_speed: 100,
    }
  );

  React.useEffect(() => {
    if (profile) {
      setName(profile.name);
      setDescription(profile.description || '');
      setAlgorithm(profile.algorithm);
      setSelectedZones(profile.zones || []);
      setSelectedInputs(profile.inputs || [{ input_type: 'max_temp', input_index: 0, weight: 1.0 }]);
      setInputAggregation(
        (profile.algorithm_params as AlgorithmParams & { input_aggregation?: 'or' | 'and' })?.input_aggregation || 'or'
      );
      
      if (profile.algorithm === 'linear') {
        setLinearParams(profile.algorithm_params as LinearParams);
      } else if (profile.algorithm === 'step') {
        setStepParams(profile.algorithm_params as StepParams);
      } else if (profile.algorithm === 'pid') {
        setPidParams(profile.algorithm_params as PIDParams);
      }
    } else {
      setName('');
      setDescription('');
      setAlgorithm('linear');
      setSelectedZones([]);
      setSelectedInputs([{ input_type: 'max_temp', input_index: 0, weight: 1.0 }]);
      setInputAggregation('or');
    }
  }, [profile]);

  const handleSave = () => {
    let algorithmParams: Record<string, unknown>;
    if (algorithm === 'linear') algorithmParams = { ...linearParams };
    else if (algorithm === 'step') algorithmParams = { ...stepParams };
    else algorithmParams = { ...pidParams };

    // Add input aggregation to algorithm params
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

  const addStep = () => {
    const lastStep = stepParams.steps[stepParams.steps.length - 1];
    setStepParams({
      steps: [...stepParams.steps, { temp: lastStep.temp + 10, speed: Math.min(lastStep.speed + 10, 100) }],
    });
  };

  const removeStep = (index: number) => {
    if (stepParams.steps.length > 1) {
      setStepParams({
        steps: stepParams.steps.filter((_, i) => i !== index),
      });
    }
  };

  const updateStep = (index: number, field: 'temp' | 'speed', value: number) => {
    const newSteps = [...stepParams.steps];
    newSteps[index] = { ...newSteps[index], [field]: value };
    setStepParams({ steps: newSteps });
  };

  // Toggle input selection
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

  // Get current temperature for display
  const getCurrentTemp = (inputType: string, inputIndex: number): number | null => {
    switch (inputType) {
      case 'max_temp':
        return Math.max(...(monitoring?.gpus?.map(g => g.temperature) || [0]));
      case 'avg_temp':
        const gpuTemps = monitoring?.gpus?.map(g => g.temperature) || [];
        return gpuTemps.length > 0 ? gpuTemps.reduce((a, b) => a + b, 0) / gpuTemps.length : null;
      case 'max_cpu':
        return Math.max(...(monitoring?.system?.cpu_packages?.map(c => c.temperature) || [0]));
      case 'max_drive':
        return Math.max(...(monitoring?.system?.drives?.map(d => d.temperature) || [0]));
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

  // Render input checkbox with current value
  const renderInputCheckbox = (type: InputTypeConfig, index: number = 0, label?: string) => {
    const currentValue = getCurrentTemp(type.value, index);
    const displayLabel = label || type.label;
    // Check if this is a temperature input (includes temp, max_cpu, max_drive)
    const isTemp = type.value.includes('temp') || type.value === 'max_cpu' || type.value === 'max_drive';
    
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

  function getTempColor(temp: number): string {
    if (temp < 50) return 'text-blue-400';
    if (temp < 65) return 'text-green-400';
    if (temp < 80) return 'text-yellow-400';
    return 'text-red-400';
  }

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={profile ? 'Edit Profile' : 'Create Profile'}
      size="lg"
    >
      <div className="space-y-4 max-h-[70vh] overflow-y-auto">
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
            <select
              className="select"
              value={algorithm}
              onChange={(e) => setAlgorithm(e.target.value as 'linear' | 'step' | 'pid')}
            >
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
                <input
                  type="number"
                  className="input"
                  value={linearParams.min_temp}
                  onChange={(e) => setLinearParams({ ...linearParams, min_temp: parseFloat(e.target.value) })}
                />
              </div>
              <div>
                <label className="input-label">Max Temp (°C)</label>
                <input
                  type="number"
                  className="input"
                  value={linearParams.max_temp}
                  onChange={(e) => setLinearParams({ ...linearParams, max_temp: parseFloat(e.target.value) })}
                />
              </div>
              <div>
                <label className="input-label">Min Speed (%)</label>
                <input
                  type="number"
                  className="input"
                  value={linearParams.min_speed}
                  onChange={(e) => setLinearParams({ ...linearParams, min_speed: parseInt(e.target.value) })}
                />
              </div>
              <div>
                <label className="input-label">Max Speed (%)</label>
                <input
                  type="number"
                  className="input"
                  value={linearParams.max_speed}
                  onChange={(e) => setLinearParams({ ...linearParams, max_speed: parseInt(e.target.value) })}
                />
              </div>
            </div>
          )}

          {algorithm === 'step' && (
            <div className="space-y-2">
              {stepParams.steps.map((step, index) => (
                <div key={index} className="flex gap-2 items-center">
                  <input
                    type="number"
                    className="input w-24"
                    value={step.temp}
                    onChange={(e) => updateStep(index, 'temp', parseFloat(e.target.value))}
                    placeholder="Temp"
                  />
                  <span className="text-slate-400">°C →</span>
                  <input
                    type="number"
                    className="input w-24"
                    value={step.speed}
                    onChange={(e) => updateStep(index, 'speed', parseInt(e.target.value))}
                    placeholder="Speed"
                  />
                  <span className="text-slate-400">%</span>
                  <Button
                    variant="danger"
                    size="sm"
                    onClick={() => removeStep(index)}
                    disabled={stepParams.steps.length <= 1}
                  >
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
                <input
                  type="number"
                  className="input"
                  value={pidParams.setpoint}
                  onChange={(e) => setPidParams({ ...pidParams, setpoint: parseFloat(e.target.value) })}
                />
              </div>
              <div>
                <label className="input-label">Kp (Proportional)</label>
                <input
                  type="number"
                  step="0.1"
                  className="input"
                  value={pidParams.kp}
                  onChange={(e) => setPidParams({ ...pidParams, kp: parseFloat(e.target.value) })}
                />
              </div>
              <div>
                <label className="input-label">Ki (Integral)</label>
                <input
                  type="number"
                  step="0.01"
                  className="input"
                  value={pidParams.ki}
                  onChange={(e) => setPidParams({ ...pidParams, ki: parseFloat(e.target.value) })}
                />
              </div>
              <div>
                <label className="input-label">Kd (Derivative)</label>
                <input
                  type="number"
                  step="0.1"
                  className="input"
                  value={pidParams.kd}
                  onChange={(e) => setPidParams({ ...pidParams, kd: parseFloat(e.target.value) })}
                />
              </div>
              <div>
                <label className="input-label">Min Speed (%)</label>
                <input
                  type="number"
                  className="input"
                  value={pidParams.min_speed}
                  onChange={(e) => setPidParams({ ...pidParams, min_speed: parseInt(e.target.value) })}
                />
              </div>
              <div>
                <label className="input-label">Max Speed (%)</label>
                <input
                  type="number"
                  className="input"
                  value={pidParams.max_speed}
                  onChange={(e) => setPidParams({ ...pidParams, max_speed: parseInt(e.target.value) })}
                />
              </div>
            </div>
          )}
        </div>

        {/* Input Selection */}
        <div className="border-t border-slate-700 pt-4">
          <h4 className="font-medium text-slate-200 mb-3">Temperature/Load Inputs</h4>
          
          {/* AND/OR Logic Toggle */}
          <div className="mb-4 p-3 bg-slate-700/30 rounded-lg">
            <p className="text-sm text-slate-300 mb-2">Input Logic (when multiple inputs are selected):</p>
            <div className="flex gap-4">
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="radio"
                  name="inputAggregation"
                  value="or"
                  checked={inputAggregation === 'or'}
                  onChange={() => setInputAggregation('or')}
                  className="text-blue-500"
                />
                <span className="text-sm text-slate-200">OR (highest value wins)</span>
              </label>
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="radio"
                  name="inputAggregation"
                  value="and"
                  checked={inputAggregation === 'and'}
                  onChange={() => setInputAggregation('and')}
                  className="text-blue-500"
                />
                <span className="text-sm text-slate-200">AND (lowest value wins)</span>
              </label>
            </div>
            <p className="text-xs text-slate-400 mt-2">
              {inputAggregation === 'or' 
                ? 'Fans respond to the hottest sensor (use for safety/responsiveness)'
                : 'Fans respond to the coolest sensor (use for quieter operation when all temps are low)'}
            </p>
          </div>

          <p className="text-sm text-slate-400 mb-3">
            Select one or more sensors to control fan speed. Selected: {selectedInputs.length}
          </p>

          <div className="space-y-2">
            {/* Aggregate Inputs */}
            <InputGroup title="Aggregate Sensors" icon={Activity} iconColor="text-purple-400">
              {INPUT_TYPES.filter(t => t.category === 'aggregate').map(type => 
                renderInputCheckbox(type)
              )}
            </InputGroup>

            {/* GPU Inputs */}
            {gpuCount > 0 && (
              <InputGroup title={`GPUs (${gpuCount})`} icon={Activity} iconColor="text-purple-400">
                {Array.from({ length: gpuCount }, (_, i) => (
                  <React.Fragment key={`gpu-${i}`}>
                    {renderInputCheckbox(
                      INPUT_TYPES.find(t => t.value === 'gpu_temp')!,
                      i,
                      `GPU ${i} Temperature${monitoring?.gpus?.[i]?.name ? ` - ${monitoring.gpus[i].name}` : ''}`
                    )}
                    {renderInputCheckbox(
                      INPUT_TYPES.find(t => t.value === 'gpu_load')!,
                      i,
                      `GPU ${i} Load`
                    )}
                  </React.Fragment>
                ))}
              </InputGroup>
            )}

            {/* CPU Inputs */}
            <InputGroup title={`CPUs (${cpuCount})`} icon={Cpu} iconColor="text-blue-400">
              {Array.from({ length: Math.max(cpuCount, 1) }, (_, i) => 
                renderInputCheckbox(
                  INPUT_TYPES.find(t => t.value === 'cpu_temp')!,
                  i,
                  `CPU ${i}${monitoring?.system?.cpu_packages?.[i]?.name ? ` - ${monitoring.system.cpu_packages[i].name}` : ''}`
                )
              )}
              {renderInputCheckbox(INPUT_TYPES.find(t => t.value === 'cpu_load')!)}
            </InputGroup>

            {/* Drive Inputs */}
            {driveCount > 0 && (
              <InputGroup 
                title={`Drives (${driveCount})`} 
                icon={HardDrive} 
                iconColor="text-cyan-400"
                defaultExpanded={driveCount <= 6}
              >
                {Array.from({ length: driveCount }, (_, i) => {
                  const drive = monitoring?.system?.drives?.[i];
                  return renderInputCheckbox(
                    INPUT_TYPES.find(t => t.value === 'drive_temp')!,
                    i,
                    `${drive?.device || `Drive ${i}`}${drive?.model ? ` - ${drive.model}` : ''}`
                  );
                })}
              </InputGroup>
            )}
          </div>
        </div>

        {/* Zone Selection */}
        <div className="border-t border-slate-700 pt-4">
          <h4 className="font-medium text-slate-200 mb-3">Fan Zones</h4>
          <p className="text-sm text-slate-400 mb-3">
            Select which fan zones this profile controls. Multiple profiles can be active simultaneously for different zones.
          </p>
          {availableZones.length > 0 ? (
            <div className="space-y-2">
              <div className="grid grid-cols-4 gap-2">
                {availableZones.map((zone) => {
                  const zoneFans = fans.filter(f => f.ipmi_zone === zone);
                  return (
                    <label key={zone} className="flex items-center gap-2 p-2 bg-slate-700/50 rounded cursor-pointer hover:bg-slate-700">
                      <input
                        type="checkbox"
                        checked={selectedZones.includes(zone)}
                        onChange={(e) => {
                          if (e.target.checked) {
                            setSelectedZones([...selectedZones, zone]);
                          } else {
                            setSelectedZones(selectedZones.filter(z => z !== zone));
                          }
                        }}
                        className="rounded"
                      />
                      <div>
                        <span className="text-sm text-slate-300">Zone {zone}</span>
                        <span className="text-xs text-slate-500 block">{zoneFans.length} fan{zoneFans.length !== 1 ? 's' : ''}</span>
                      </div>
                    </label>
                  );
                })}
              </div>
              <p className="text-xs text-slate-500 mt-2">
                {selectedZones.length === 0 
                  ? 'No zones selected - profile will control all fans when activated' 
                  : `Selected zones: ${selectedZones.sort((a, b) => a - b).join(', ')}`}
              </p>
            </div>
          ) : (
            <div className="p-3 bg-slate-700/30 rounded-lg">
              <p className="text-sm text-slate-400">
                No fan zones configured. Go to the Fans page to assign zones to fans, or leave empty to control all fans.
              </p>
            </div>
          )}
        </div>

        <div className="flex justify-end gap-2 pt-4 border-t border-slate-700">
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button onClick={handleSave} isLoading={isLoading} disabled={!name}>
            {profile ? 'Update' : 'Create'}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
