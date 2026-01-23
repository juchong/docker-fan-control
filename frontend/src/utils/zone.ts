import { ZoneLayout, ZoneDefinition } from '../types/zone';

// Create a default zone layout
export function createDefaultZoneLayout(): ZoneLayout {
  return {
    zones: [
      {
        id: 0,
        name: 'CPU Zone',
        fan_indices: [0],
        description: 'CPU cooling fan',
        is_default: false,
      },
      {
        id: 1,
        name: 'System Zone',
        fan_indices: [1, 2, 3, 4, 5, 6],
        description: 'System cooling fans',
        is_default: true,
      },
    ],
  };
}

// Get zone by ID
export function getZoneById(layout: ZoneLayout | null | undefined, zoneId: number): ZoneDefinition | undefined {
  return layout?.zones.find(z => z.id === zoneId);
}

// Get zone name by ID (with fallback)
export function getZoneName(layout: ZoneLayout | null | undefined, zoneId: number): string {
  const zone = getZoneById(layout, zoneId);
  return zone?.name ?? `Zone ${zoneId}`;
}

// Get all zone IDs from layout
export function getZoneIds(layout: ZoneLayout | null | undefined): number[] {
  return layout?.zones.map(z => z.id) ?? [];
}

// Check if fan index is assigned to any zone
export function getFanZone(layout: ZoneLayout | null | undefined, fanIndex: number): ZoneDefinition | undefined {
  return layout?.zones.find(z => z.fan_indices.includes(fanIndex));
}
