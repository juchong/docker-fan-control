import { FanStatus } from './fan';

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

export interface SystemMetrics {
  cpu_packages: CPUPackageMetrics[];
  drives: DriveMetrics[];
  cpu_load: number;
  memory_used: number;
  memory_total: number;
  // Legacy field for backward compatibility
  cpu_temp?: number;
}

export interface ControllerState {
  running: boolean;
  active_profile_id?: number;    // Deprecated, use active_profile_ids
  active_profile?: string;       // Deprecated, use active_profiles
  active_profiles?: string[];    // Names of all active profiles
  active_profile_ids?: number[]; // IDs of all active profiles
  last_update?: string;
  manual_mode: boolean;
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

export interface EventQuery {
  level?: string;
  category?: string;
  search?: string;
  start_time?: string;
  end_time?: string;
  limit?: number;
  offset?: number;
}

export interface AppSettings {
  ipmi_mode: string;
  ipmi_host?: string;
  ipmi_user?: string;
  ipmi_command_format: string;
  control_interval: number;
  temp_unit: string;
  startup_mode: string;
  startup_percent?: number;
  emergency_temp: number;
  emergency_speed: number;
  warning_temp: number;
  warning_enabled: boolean;
  safety_on_shutdown: boolean;
}

export interface UpdateSettingsRequest {
  ipmi_mode?: string;
  ipmi_host?: string;
  ipmi_user?: string;
  ipmi_pass?: string;
  ipmi_command_format?: string;
  control_interval?: number;
  temp_unit?: string;
  startup_mode?: string;
  startup_percent?: number;
  emergency_temp?: number;
  emergency_speed?: number;
  warning_temp?: number;
  warning_enabled?: boolean;
  safety_on_shutdown?: boolean;
}

export interface WebSocketMessage {
  type: string;
  timestamp: string;
  data: unknown;
}
