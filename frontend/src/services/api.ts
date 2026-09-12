import type { AppSettings, DriverInfo, MotherboardDetectionResult } from '../types/settings';
import type { ZoneLayout } from '../types/zone';

const API_BASE = (import.meta as any).env?.VITE_API_BASE || '/api';

class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
    this.name = 'ApiError';
  }
}

async function request<T>(
  endpoint: string,
  options: RequestInit = {}
): Promise<T> {
  const token = localStorage.getItem('auth_token');

  const headers: HeadersInit = {
    'Content-Type': 'application/json',
    ...options.headers,
  };

  if (token) {
    (headers as Record<string, string>)['Authorization'] = `Bearer ${token}`;
  }

  const response = await fetch(`${API_BASE}${endpoint}`, {
    ...options,
    headers,
  });

  if (!response.ok) {
    const text = await response.text();
    throw new ApiError(response.status, text || response.statusText);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  return response.json();
}

// Auth API
export const authApi = {
  login: (username: string, password: string) =>
    request<{ token: string; user: unknown }>('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),

  logout: () => request<void>('/auth/logout', { method: 'POST' }),

  refresh: () => request<{ token: string }>('/auth/refresh', { method: 'POST' }),

  me: () => request<unknown>('/auth/me'),

  status: () => request<{ auth_enabled: boolean; proxy_enabled: boolean }>('/auth/status'),

  changePassword: (currentPassword: string, newPassword: string) =>
    request<void>('/auth/password', {
      method: 'PUT',
      body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
    }),

  listUsers: () => request<unknown[]>('/users'),

  createUser: (username: string, password: string, role: string) =>
    request<unknown>('/users', {
      method: 'POST',
      body: JSON.stringify({ username, password, role }),
    }),

  deleteUser: (id: number) =>
    request<void>(`/users/${id}`, { method: 'DELETE' }),
};

// Fans API
export const fansApi = {
  list: () => request<unknown[]>('/fans'),

  detect: () => request<unknown[]>('/fans/detect', { method: 'POST' }),

  update: (id: number, data: { label?: string; ipmi_zone?: number }) =>
    request<unknown>(`/fans/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  identify: (id: number, duration?: number) =>
    request<{ message: string; duration: number }>(`/fans/${id}/identify`, {
      method: 'POST',
      body: JSON.stringify({ duration }),
    }),

  setSpeed: (id: number, percent: number, durationSeconds?: number) =>
    request<{ message: string; percent: number }>(`/fans/${id}/speed`, {
      method: 'POST',
      body: JSON.stringify({ percent, duration_seconds: durationSeconds }),
    }),

  clearSpeed: (id: number) =>
    request<{ message: string }>(`/fans/${id}/speed`, { method: 'DELETE' }),
};

// Profiles API
export const profilesApi = {
  list: () => request<unknown[]>('/profiles'),

  get: (id: number) => request<unknown>(`/profiles/${id}`),

  create: (data: unknown) =>
    request<unknown>('/profiles', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  update: (id: number, data: unknown) =>
    request<unknown>(`/profiles/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  delete: (id: number) => request<void>(`/profiles/${id}`, { method: 'DELETE' }),

  activate: (id: number) =>
    request<{ message: string; profile_id: number }>(`/profiles/${id}/activate`, {
      method: 'POST',
    }),

  deactivate: (id: number) =>
    request<{ message: string; profile_id: number }>(`/profiles/${id}/deactivate`, {
      method: 'POST',
    }),
};

// Monitoring API
export const monitoringApi = {
  getMetrics: () => request<unknown>('/monitoring/metrics'),
  getGPUs: () => request<unknown[]>('/monitoring/gpus'),
  getSystem: () => request<unknown>('/monitoring/system'),
  getFans: () => request<unknown[]>('/monitoring/fans'),
};

// Logs API
export const logsApi = {
  list: (params?: Record<string, string | number>) => {
    const query = params
      ? '?' + new URLSearchParams(
          Object.entries(params).map(([k, v]) => [k, String(v)])
        ).toString()
      : '';
    return request<unknown>(`/logs${query}`);
  },

  export: (format: 'json' | 'csv', params?: Record<string, string>) => {
    const query = new URLSearchParams({ format, ...params }).toString();
    return `${API_BASE}/logs/export?${query}`;
  },

  clear: () => request<void>('/logs', { method: 'DELETE' }),
};

// Settings API
export const settingsApi = {
  get: () => request<AppSettings>('/settings'),

  update: (data: { zone_layout?: ZoneLayout } & Record<string, unknown>) =>
    request<AppSettings>('/settings', {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  testIPMI: () => 
    request<{ success: boolean; error?: string; message?: string }>('/settings/test-ipmi', { 
      method: 'POST' 
    }),

  detectMotherboard: () => 
    request<MotherboardDetectionResult>('/settings/detect-motherboard', { 
      method: 'POST' 
    }),

  getAvailableDrivers: () => request<DriverInfo[]>('/settings/drivers'),

  startController: () => 
    request<{ success: boolean; message: string }>('/controller/start', { 
      method: 'POST' 
    }),

  stopController: () => 
    request<{ success: boolean; message: string }>('/controller/stop', { 
      method: 'POST' 
    }),
};

export { ApiError };
