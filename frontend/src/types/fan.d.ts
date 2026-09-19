export interface Fan {
  id: number;
  ipmi_sensor_id: string;
  ipmi_zone?: number;
  channel?: number;
  chip?: string;
  label?: string;
  detected_name?: string;
  created_at: string;
  updated_at: string;
}

/** Who is driving a fan right now (reported by drivers that can tell). */
export type FanControlMode = 'manual' | 'firmware';

export interface FanStatus {
  id: number;
  ipmi_sensor_id: string;
  label: string;
  current_rpm: number;
  /** 0-100; -1 when the driver cannot read the duty (e.g. firmware-managed ITE channels). */
  current_duty: number;
  current_percent?: number;
  target_percent?: number;
  manual_override: boolean;
  ipmi_zone?: number;
  channel?: number;
  chip?: string;
  control_mode?: FanControlMode;
  assigned_profiles?: number[];
}

export interface DetectedFan {
  sensor_id: string;
  name: string;
  rpm: number;
  duty_cycle: number;
  chip: string;
  channel: number;
  zone_id: number;
  status: string;
  unit: string;
}

export interface UpdateFanRequest {
  label?: string;
  ipmi_zone?: number;
}

export interface SetFanSpeedRequest {
  percent: number;
  duration_seconds?: number;
}

export interface IdentifyFanRequest {
  duration?: number;
}
