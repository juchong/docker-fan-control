// Zone layout types - the authoritative zone system

export interface ZoneDefinition {
  id: number;
  name: string;
  fan_indices: number[];
  description?: string;
  is_default: boolean;
  color?: string;
}

export interface ZoneLayout {
  zones: ZoneDefinition[];
}
