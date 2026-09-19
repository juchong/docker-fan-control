// Zone layout types - the authoritative zone system

export interface ZoneDefinition {
  id: number;
  name: string;
  fan_indices: number[];
  description?: string;
  is_default: boolean;
  /** Controller chip the zone's channel lives on (multi-chip hwmon boards). */
  chip?: string;
  color?: string;
}

export interface ZoneLayout {
  zones: ZoneDefinition[];
}
