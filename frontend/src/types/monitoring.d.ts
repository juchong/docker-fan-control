import { FanStatus } from './fan';
import { ZoneLayout, ZoneDefinition } from './zone';

export interface GPUMetrics {
  index: number;
  name: string;
  temperature: number;
  load: number;
  fan_speed: number;
  memory_used: number;
  memory_total: number;
  power_usage?: number;
  power_limit?: number;
}

export interface CPUPackageMetrics {
  index: number;
  name: string;
  model?: string;
  temperature: number;
}

export interface DriveMetrics {
  index: number;
  device: string;
  model: string;
  serial?: string;
  type: 'hdd' | 'ssd' | 'nvme';
  temperature: number;
}

export interface BoardTempMetrics {
  index: number;
  name: string;
  temperature: number;
}

export interface SystemMetrics {
  cpu_packages: CPUPackageMetrics[];
  drives: DriveMetrics[];
  board_temps: BoardTempMetrics[];
  cpu_load: number;
  memory_used: number;
  memory_total: number;
  cpu_temp?: number;
}

export interface DriverCapabilities {
  supports_manual_mode: boolean;
  supports_duty_cycle_reading: boolean;
  supports_per_zone_control: boolean;
  max_zones: number;
  max_fans: number;
  has_static_rpm_values: boolean;
}

export interface ControllerState {
  running: boolean;
  active_profiles?: string[];
  active_profile_ids?: number[];
  last_update?: string;
  manual_mode: boolean;
  motherboard_vendor?: string;
  motherboard_model?: string;
  motherboard_driver?: string;
  driver_vendor?: string;
  driver_model?: string;
  driver_capabilities?: DriverCapabilities;
}


export interface Monitoring {
  gpus: GPUMetrics[];
  system: SystemMetrics;
  fans: FanStatus[];
  controller: ControllerState;
}

export interface Event {
  id: number;
  timestamp: string;
  level: 'info' | 'warning' | 'error';
  category: string;
  message: string;
  details?: Record<string, unknown>;
}

export interface EventListResponse {
  events: Event[];
  total_count: number;
  limit: number;
  offset: number;
}

export interface WebSocketMessage {
  type: string;
  timestamp: string;
  data: unknown;
}

// Re-export zone types for convenience
export { ZoneLayout, ZoneDefinition };
