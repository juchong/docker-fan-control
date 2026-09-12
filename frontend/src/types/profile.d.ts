export interface Profile {
  id: number;
  name: string;
  description?: string;
  algorithm: 'linear' | 'step' | 'pid';
  algorithm_params: AlgorithmParams;
  is_active: boolean;
  priority: number;
  zones?: number[];
  smooth_transition?: boolean;
  transition_time?: number;
  min_run_time?: number;
  hysteresis?: number;
  created_at: string;
  updated_at: string;
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
export type InputAggregation = 'or' | 'and' | 'max' | 'min' | 'avg' | 'weighted';
export interface BaseAlgorithmParams {
  // 'or'/'max' (default), 'and'/'min', 'avg', or 'weighted' (uses per-input weights)
  input_aggregation?: InputAggregation;
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

export interface ProfileTuning {
  smooth_transition?: boolean;
  transition_time?: number;
  min_run_time?: number;
  hysteresis?: number;
}

export interface CreateProfileRequest extends ProfileTuning {
  name: string;
  description?: string;
  algorithm: string;
  algorithm_params?: AlgorithmParams;
  priority?: number;
  zones?: number[];
  inputs?: ProfileInput[];
}

export interface UpdateProfileRequest extends ProfileTuning {
  name?: string;
  description?: string;
  algorithm?: string;
  algorithm_params?: AlgorithmParams;
  priority?: number;
  zones?: number[];
  inputs?: ProfileInput[];
}

import { Fan } from './fan';
