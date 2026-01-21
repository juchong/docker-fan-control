export interface Fan {
  id: number;
  ipmi_sensor_id: string;
  ipmi_zone?: number;
  label?: string;
  detected_name?: string;
  min_rpm?: number;
  max_rpm?: number;
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
  assigned_profiles?: number[];
}

export interface DetectedFan {
  sensor_id: string;
  name: string;
  rpm: number;
  status: string;
  unit: string;
}

export interface UpdateFanRequest {
  label?: string;
  ipmi_zone?: number;
}

export interface SetFanSpeedRequest {
  percent: number;
}

export interface IdentifyFanRequest {
  duration?: number;
}
