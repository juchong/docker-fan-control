export interface Fan {
  id: number;
  ipmi_sensor_id: string;
  ipmi_zone?: number;
  channel?: number;
  label?: string;
  detected_name?: string;
  created_at: string;
  updated_at: string;
}

export interface FanStatus {
  id: number;
  ipmi_sensor_id: string;
  label: string;
  current_rpm: number;
  current_duty: number;
  current_percent?: number;
  target_percent?: number;
  manual_override: boolean;
  ipmi_zone?: number;
  channel?: number;
  assigned_profiles?: number[];
}

export interface DetectedFan {
  sensor_id: string;
  name: string;
  rpm: number;
  duty_cycle: number;
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
