import { useState, useEffect, useCallback } from 'react';
import { Button } from '../common/Button';
import { Modal } from '../common/Modal';
import { ZoneLayout, ZoneDefinition } from '../../types/zone';
import { createDefaultZoneLayout } from '../../utils/zone';
import { FanStatus } from '../../types/fan';
import { 
  Plus, 
  Trash2, 
  Edit2, 
  Save,
  AlertTriangle,
  Check,
  X,
  Layers
} from 'lucide-react';

interface ZoneLayoutEditorProps {
  zoneLayout: ZoneLayout | null;
  onSave: (layout: ZoneLayout) => void;
  onCancel?: () => void;
  fans?: FanStatus[];
  isSaving?: boolean;
  compact?: boolean;
}

export function ZoneLayoutEditor({ 
  zoneLayout, 
  onSave, 
  onCancel,
  fans = [],
  isSaving = false,
  compact = false
}: ZoneLayoutEditorProps) {
  const [zones, setZones] = useState<ZoneDefinition[]>([]);
  const [originalZones, setOriginalZones] = useState<ZoneDefinition[]>([]);
  const [editingZone, setEditingZone] = useState<number | null>(null);
  const [editZoneName, setEditZoneName] = useState('');
  const [editZoneDescription, setEditZoneDescription] = useState('');
  const [isAddingZone, setIsAddingZone] = useState(false);
  const [newZoneName, setNewZoneName] = useState('');
  const [newZoneDescription, setNewZoneDescription] = useState('');

  // Initialize from props
  useEffect(() => {
    const initialZones = zoneLayout?.zones ?? createDefaultZoneLayout().zones;
    setZones(JSON.parse(JSON.stringify(initialZones)));
    setOriginalZones(JSON.parse(JSON.stringify(initialZones)));
  }, [zoneLayout]);

  // Check if there are pending changes
  const hasChanges = useCallback(() => {
    return JSON.stringify(zones) !== JSON.stringify(originalZones);
  }, [zones, originalZones]);

  // Fan index to display name mapping
  const getFanDisplayName = (fanIndex: number): string => {
    const fan = fans.find(f => {
      // Match by sensor ID pattern (FAN1 = index 0, etc.)
      const match = f.ipmi_sensor_id?.match(/FAN(\d+)/i);
      return match && parseInt(match[1]) - 1 === fanIndex;
    });
    return fan?.label || `FAN${fanIndex + 1}`;
  };

  // Get fan RPM for display
  const getFanRpm = (fanIndex: number): number | null => {
    const fan = fans.find(f => {
      const match = f.ipmi_sensor_id?.match(/FAN(\d+)/i);
      return match && parseInt(match[1]) - 1 === fanIndex;
    });
    return fan?.current_rpm ?? null;
  };

  const handleAddZone = () => {
    if (!newZoneName.trim()) return;
    
    const newId = zones.length > 0 ? Math.max(...zones.map(z => z.id)) + 1 : 0;
    const newZone: ZoneDefinition = {
      id: newId,
      name: newZoneName.trim(),
      fan_indices: [],
      description: newZoneDescription.trim() || undefined,
      is_default: zones.length === 0, // First zone is default
    };
    
    setZones([...zones, newZone]);
    setNewZoneName('');
    setNewZoneDescription('');
    setIsAddingZone(false);
  };

  const handleEditZone = (zone: ZoneDefinition) => {
    setEditingZone(zone.id);
    setEditZoneName(zone.name);
    setEditZoneDescription(zone.description || '');
  };

  const handleSaveZoneEdit = () => {
    if (editingZone === null || !editZoneName.trim()) return;
    
    setZones(zones.map(z => 
      z.id === editingZone 
        ? { ...z, name: editZoneName.trim(), description: editZoneDescription.trim() || undefined }
        : z
    ));
    setEditingZone(null);
  };

  const handleDeleteZone = (zoneId: number) => {
    if (zones.length <= 1) {
      alert('You must have at least one zone');
      return;
    }
    
    const zone = zones.find(z => z.id === zoneId);
    if (zone?.is_default) {
      // Assign default to another zone
      const remainingZones = zones.filter(z => z.id !== zoneId);
      remainingZones[0].is_default = true;
      setZones(remainingZones);
    } else {
      setZones(zones.filter(z => z.id !== zoneId));
    }
    
    if (editingZone === zoneId) {
      setEditingZone(null);
    }
  };

  const handleFanToggle = (zoneId: number, fanIndex: number) => {
    setZones(zones.map(zone => {
      if (zone.id === zoneId) {
        const exists = zone.fan_indices.includes(fanIndex);
        if (exists) {
          return {
            ...zone,
            fan_indices: zone.fan_indices.filter(i => i !== fanIndex),
          };
        } else {
          // Remove fan from other zones first
          return {
            ...zone,
            fan_indices: [...zone.fan_indices, fanIndex].sort((a, b) => a - b),
          };
        }
      } else {
        // Remove from other zones if assigning to this zone
        return {
          ...zone,
          fan_indices: zone.fan_indices.filter(i => i !== fanIndex),
        };
      }
    }));
  };

  const handleSetDefaultZone = (zoneId: number) => {
    setZones(zones.map(zone => ({
      ...zone,
      is_default: zone.id === zoneId,
    })));
  };

  const handleSave = () => {
    // Validate
    if (!zones.some(z => z.is_default)) {
      alert('At least one zone must be marked as default');
      return;
    }

    const layout: ZoneLayout = {
      zones: zones.map(z => ({
        ...z,
        fan_indices: [...z.fan_indices].sort((a, b) => a - b),
      })),
    };

    onSave(layout);
    setOriginalZones(JSON.parse(JSON.stringify(zones)));
  };

  const handleCancel = () => {
    setZones(JSON.parse(JSON.stringify(originalZones)));
    onCancel?.();
  };

  // Check which fans are detected (have RPM readings)
  const detectedFanIndices = fans
    .filter(f => f.current_rpm > 0)
    .map(f => {
      const match = f.ipmi_sensor_id?.match(/FAN(\d+)/i);
      return match ? parseInt(match[1]) - 1 : -1;
    })
    .filter(i => i >= 0);

  const pendingChanges = hasChanges();

  return (
    <div className="space-y-4">
      {/* Pending Changes Banner */}
      {pendingChanges && (
        <div className="flex items-center justify-between p-3 bg-warn/15 border border-warn/30 rounded-lg">
          <div className="flex items-center gap-2 text-warn">
            <AlertTriangle className="w-5 h-5" />
            <span className="font-medium">You have unsaved changes</span>
          </div>
          <div className="flex gap-2">
            <Button variant="secondary" size="sm" onClick={handleCancel}>
              <X className="w-4 h-4 mr-1" />
              Discard
            </Button>
            <Button size="sm" onClick={handleSave} isLoading={isSaving}>
              <Save className="w-4 h-4 mr-1" />
              Save Changes
            </Button>
          </div>
        </div>
      )}

      {/* Zone List */}
      <div className="space-y-3">
        {zones.map((zone) => (
          <div 
            key={zone.id} 
            className={`border rounded-lg p-4 transition-colors ${
              zone.is_default 
                ? 'border-primary-600 bg-info/15' 
                : 'border-surface-3 bg-surface/50'
            }`}
          >
            {editingZone === zone.id ? (
              <div className="space-y-3">
                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="input-label">Zone Name</label>
                    <input
                      type="text"
                      className="input"
                      value={editZoneName}
                      onChange={(e) => setEditZoneName(e.target.value)}
                      autoFocus
                    />
                  </div>
                  <div>
                    <label className="input-label">Description</label>
                    <input
                      type="text"
                      className="input"
                      value={editZoneDescription}
                      onChange={(e) => setEditZoneDescription(e.target.value)}
                      placeholder="Optional"
                    />
                  </div>
                </div>
                <div className="flex gap-2">
                  <Button size="sm" onClick={handleSaveZoneEdit}>
                    <Check className="w-4 h-4 mr-1" />
                    Save
                  </Button>
                  <Button variant="secondary" size="sm" onClick={() => setEditingZone(null)}>
                    Cancel
                  </Button>
                </div>
              </div>
            ) : (
              <div className="space-y-3">
                {/* Zone Header */}
                <div className="flex justify-between items-start">
                  <div>
                    <div className="flex items-center gap-2">
                      <span className="text-lg font-semibold text-fg">{zone.name}</span>
                      {zone.is_default && (
                        <span className="text-xs bg-primary-600 text-info px-2 py-0.5 rounded">
                          Default
                        </span>
                      )}
                    </div>
                    {zone.description && (
                      <p className="text-sm text-muted mt-0.5">{zone.description}</p>
                    )}
                    <p className="text-xs text-muted-2 mt-1">
                      Zone ID: {zone.id} • {zone.fan_indices.length} fan{zone.fan_indices.length !== 1 ? 's' : ''} assigned
                    </p>
                  </div>
                  <div className="flex gap-1">
                    <Button variant="secondary" size="sm" onClick={() => handleEditZone(zone)}>
                      <Edit2 className="w-4 h-4" />
                    </Button>
                    <Button 
                      variant="danger" 
                      size="sm" 
                      onClick={() => handleDeleteZone(zone.id)}
                      disabled={zones.length <= 1}
                    >
                      <Trash2 className="w-4 h-4" />
                    </Button>
                  </div>
                </div>

                {/* Fan Selection Grid */}
                <div>
                  <label className="input-label mb-2">Fans in this zone (click to toggle)</label>
                  <div className={`grid gap-1 ${compact ? 'grid-cols-8' : 'grid-cols-4 md:grid-cols-8'}`}>
                    {Array.from({ length: 16 }, (_, i) => {
                      const isInZone = zone.fan_indices.includes(i);
                      const isDetected = detectedFanIndices.includes(i);
                      const rpm = getFanRpm(i);
                      
                      return (
                        <button
                          key={i}
                          onClick={() => handleFanToggle(zone.id, i)}
                          className={`p-2 rounded text-center transition-all ${
                            isInZone
                              ? 'bg-primary-600 border-2 border-primary-500 text-white'
                              : isDetected
                                ? 'bg-surface-2 border border-surface-3 hover:border-primary-500 text-fg-3'
                                : 'bg-surface border border-surface-2 text-muted-2 opacity-50'
                          }`}
                          title={rpm ? `${rpm} RPM` : 'Not detected'}
                        >
                          <span className="text-xs font-mono block">
                            {getFanDisplayName(i).replace('FAN', 'F')}
                          </span>
                          {rpm !== null && (
                            <span className="text-[10px] opacity-75 block">
                              {rpm > 0 ? `${rpm}` : '-'}
                            </span>
                          )}
                        </button>
                      );
                    })}
                  </div>
                </div>

                {/* Default Zone Toggle */}
                {!zone.is_default && (
                  <button
                    onClick={() => handleSetDefaultZone(zone.id)}
                    className="text-sm text-muted hover:text-info transition-colors"
                  >
                    Set as default zone
                  </button>
                )}
              </div>
            )}
          </div>
        ))}
      </div>

      {/* Add Zone Button / Form */}
      {isAddingZone ? (
        <div className="border border-surface-3 rounded-lg p-4 bg-surface/50">
          <h4 className="font-medium text-fg-2 mb-3">Add New Zone</h4>
          <div className="grid grid-cols-2 gap-3 mb-3">
            <div>
              <label className="input-label">Zone Name</label>
              <input
                type="text"
                className="input"
                value={newZoneName}
                onChange={(e) => setNewZoneName(e.target.value)}
                placeholder="e.g., GPU Zone"
                autoFocus
              />
            </div>
            <div>
              <label className="input-label">Description</label>
              <input
                type="text"
                className="input"
                value={newZoneDescription}
                onChange={(e) => setNewZoneDescription(e.target.value)}
                placeholder="Optional"
              />
            </div>
          </div>
          <div className="flex gap-2">
            <Button onClick={handleAddZone} disabled={!newZoneName.trim()}>
              <Plus className="w-4 h-4 mr-1" />
              Add Zone
            </Button>
            <Button variant="secondary" onClick={() => setIsAddingZone(false)}>
              Cancel
            </Button>
          </div>
        </div>
      ) : (
        <Button variant="secondary" onClick={() => setIsAddingZone(true)}>
          <Plus className="w-4 h-4 mr-2" />
          Add Zone
        </Button>
      )}

      {/* Save Button (when no pending changes banner) */}
      {!pendingChanges && !compact && (
        <div className="pt-4 border-t border-surface-2">
          <p className="text-sm text-muted mb-3">
            <Layers className="w-4 h-4 inline mr-1" />
            Zones group fans for profile-based control. Each profile can target specific zones.
          </p>
        </div>
      )}
    </div>
  );
}

// Modal wrapper for the editor
interface ZoneLayoutModalProps {
  isOpen: boolean;
  onClose: () => void;
  zoneLayout: ZoneLayout | null;
  onSave: (layout: ZoneLayout) => void;
  fans?: FanStatus[];
  isSaving?: boolean;
}

export function ZoneLayoutModal({
  isOpen,
  onClose,
  zoneLayout,
  onSave,
  fans = [],
  isSaving = false,
}: ZoneLayoutModalProps) {
  return (
    <Modal isOpen={isOpen} onClose={onClose} title="Configure Zone Layout" size="xl">
      <div className="max-h-[70vh] overflow-y-auto">
        <ZoneLayoutEditor
          zoneLayout={zoneLayout}
          onSave={(layout) => {
            onSave(layout);
            onClose();
          }}
          onCancel={onClose}
          fans={fans}
          isSaving={isSaving}
          compact
        />
      </div>
    </Modal>
  );
}
