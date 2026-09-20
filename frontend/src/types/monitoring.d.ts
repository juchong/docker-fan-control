import { FanStatus } from './fan';
import { ZoneLayout, ZoneDefinition } from './zone';

/** Per-device thermal annotation from the controller (absent on older backends). */
export type ThermalStatus = 'ok' | 'warning' | 'critical';

export interface ThermalInfo {
  /** The device's own limit (throttle/critical point), °C. */
  limit?: number;
  /** Where the limit came from: nvml, hwmon, coretemp, default, override, legacy. */
  limit_source?: string;
  /** °C to the limit (negative = over). */
  headroom?: number;
  status?: ThermalStatus;
}

export interface ThermalDevice {
  kind: 'gpu' | 'cpu' | 'drive';
  index: number;
  name: string;
  temperature: number;
  limit: number;
  headroom: number;
  status: ThermalStatus;
}

export interface ThermalState {
  mode: 'hardware' | 'legacy';
  status: ThermalStatus;
  emergency_active: boolean;
  warning_margin: number;
  /** The all-fans emergency temperature (the unchanged fan-behaviour rule). */
  emergency_temp: number;
  worst?: ThermalDevice;
}

export interface GPUMetrics extends ThermalInfo {
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

export interface CPUPackageMetrics extends ThermalInfo {
  index: number;
  name: string;
  model?: string;
  temperature: number;
}

export interface DriveSensor {
  label: string;
  temperature: number;
}

export interface DriveMetrics extends ThermalInfo {
  index: number;
  device: string;
  model: string;
  serial?: string;
  firmware?: string;
  type: 'hdd' | 'ssd' | 'nvme';
  /** Celsius; for NVMe the drive's Composite channel. */
  temperature: number;
  /** The drive's own warning / critical thresholds, when it reports them. */
  max?: number;
  crit?: number;
  /** Additional channels (NVMe "Sensor 1", "Sensor 2"). */
  sensors?: DriveSensor[];
  source?: 'hwmon' | 'smartctl';
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
  /** Zones no profile/override targets stay on the board's own fan curve. */
  firmware_fallback?: boolean;
}

/** Per active profile: the duty its curve computed on the last control cycle. */
export interface ProfileDuty {
  id: number;
  name: string;
  duty: number;
}

export interface ControllerState {
  running: boolean;
  active_profiles?: string[];
  active_profile_ids?: number[];
  /** Per-profile computed duty %, for the dashboard's per-profile plot (absent on older backends). */
  profile_duties?: ProfileDuty[];
  last_update?: string;
  manual_mode: boolean;
  motherboard_vendor?: string;
  motherboard_model?: string;
  motherboard_driver?: string;
  driver_vendor?: string;
  driver_model?: string;
  driver_capabilities?: DriverCapabilities;
  /** Live driver health issues, e.g. firmware overriding PWM writes. */
  driver_warnings?: string[];
  /** Last thermal evaluation against per-device limits (absent on older backends). */
  thermal?: ThermalState;
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
