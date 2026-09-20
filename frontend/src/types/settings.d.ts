import { ZoneLayout, ZoneDefinition } from './zone';

export { ZoneLayout, ZoneDefinition };

export interface AppSettings {
  ipmi_mode: string;
  ipmi_host?: string;
  ipmi_user?: string;
  ipmi_command_format: string;
  control_interval: number;
  startup_mode: string;
  startup_percent?: number;
  emergency_temp: number;
  emergency_speed: number;
  warning_temp: number;
  warning_enabled: boolean;
  safety_on_shutdown: boolean;
  /** Thermal limits (absent on older backends). */
  thermal_limits_mode?: 'hardware' | 'legacy';
  warning_margin?: number;
  limit_gpu?: number;
  limit_cpu?: number;
  limit_drive?: number;
  motherboard_vendor?: string;
  motherboard_model?: string;
  motherboard_driver?: string;
  zone_layout?: ZoneLayout;
}

export interface UpdateSettingsRequest {
  ipmi_mode?: string;
  ipmi_host?: string;
  ipmi_user?: string;
  ipmi_pass?: string;
  ipmi_command_format?: string;
  control_interval?: number;
  startup_mode?: string;
  startup_percent?: number;
  emergency_temp?: number;
  emergency_speed?: number;
  warning_temp?: number;
  warning_enabled?: boolean;
  safety_on_shutdown?: boolean;
  thermal_limits_mode?: 'hardware' | 'legacy';
  warning_margin?: number;
  /** Per-class limit override in °C; 0 clears it. */
  limit_gpu?: number;
  limit_cpu?: number;
  limit_drive?: number;
  motherboard_vendor?: string;
  motherboard_model?: string;
  motherboard_driver?: string;
  zone_layout?: ZoneLayout;
}

export interface DriverInfo {
  vendor: string;
  model: string;
  capabilities: DriverCapabilities;
  zone_layout: ZoneLayout;
}

export interface DriverCapabilities {
  supports_manual_mode: boolean;
  supports_duty_cycle_reading: boolean;
  supports_per_zone_control: boolean;
  max_zones: number;
  max_fans: number;
  has_static_rpm_values: boolean;
}

export interface MotherboardDetectionResult {
  success: boolean;
  driver?: DriverInfo;
  error?: string;
  message?: string;
}
