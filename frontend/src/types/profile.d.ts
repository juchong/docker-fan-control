export interface Profile {
  id: number;
  name: string;
  description?: string;
  algorithm: 'linear' | 'step' | 'pid';
  algorithm_params: AlgorithmParams;
  is_active: boolean;
  priority: number;
  zones?: number[];
  created_at: string;
  updated_at: string;
  fans?: Fan[]; // Deprecated, use zones
  inputs?: ProfileInput[];
}

export interface ProfileSummary {
  id: number;
  name: string;
  description?: string;
  algorithm: string;
  is_active: boolean;
  zones?: number[];
  zone_count: number;
  fan_count: number; // Deprecated
  input_count: number;
}

export interface ProfileInput {
  id?: number;
  profile_id?: number;
  input_type: string;
  input_index: number;
  weight: number;
}

// Base params that all algorithms can have
export interface BaseAlgorithmParams {
  input_aggregation?: 'or' | 'and'; // 'or' = max (default), 'and' = min
}

export type AlgorithmParams = (LinearParams | StepParams | PIDParams) & BaseAlgorithmParams;

export interface LinearParams {
  min_temp: number;
  max_temp: number;
  min_speed: number;
  max_speed: number;
}

export interface StepParams {
  steps: StepPoint[];
}

export interface StepPoint {
  temp: number;
  speed: number;
}

export interface PIDParams {
  setpoint: number;
  kp: number;
  ki: number;
  kd: number;
  min_speed: number;
  max_speed: number;
}

export interface CreateProfileRequest {
  name: string;
  description?: string;
  algorithm: string;
  algorithm_params?: AlgorithmParams;
  priority?: number;
  zones?: number[];
  fan_ids?: number[]; // Deprecated, use zones
  inputs?: ProfileInput[];
}

export interface UpdateProfileRequest {
  name?: string;
  description?: string;
  algorithm?: string;
  algorithm_params?: AlgorithmParams;
  priority?: number;
  zones?: number[];
  fan_ids?: number[]; // Deprecated, use zones
  inputs?: ProfileInput[];
}

import { Fan } from './fan';
