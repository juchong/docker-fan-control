import { useState, useEffect, useMemo } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { settingsApi, authApi } from '../services/api';
import { useAuthContext } from '../components/auth/AuthProvider';
import { useMonitoring } from '../hooks/useMonitoring';
import { Card } from '../components/common/Card';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
import { AppSettings, UpdateSettingsRequest } from '../types/settings';
import type { ThermalInfo, ThermalStatus } from '../types/monitoring';
import { User } from '../types/auth';
import {
  Save,
  TestTube,
  Play,
  Square,
  Plus,
  Trash2,
  CheckCircle,
  XCircle,
  KeyRound,
} from 'lucide-react';

export function Settings() {
  const queryClient = useQueryClient();
  const { user } = useAuthContext();
  const { data: monitoring } = useMonitoring();
  const [isAddingUser, setIsAddingUser] = useState(false);
  const [isChangingPassword, setIsChangingPassword] = useState(false);
  const [actionMsg, setActionMsg] = useState<{ text: string; error?: boolean } | null>(null);

  // The IPMI connection controls only apply to BMC drivers; hide them on the
  // direct-sysfs (Hwmon) path where "Test Connection" always fails.
  const activeDriverVendor = monitoring?.controller?.driver_vendor;
  const isIpmiDriver = !!activeDriverVendor && activeDriverVendor !== 'Hwmon';

  const { data: settings } = useQuery<AppSettings>({
    queryKey: ['settings'],
    queryFn: settingsApi.get,
  });

  const { data: users } = useQuery({
    queryKey: ['users'],
    queryFn: authApi.listUsers,
    enabled: user?.role === 'admin',
  });

  const driversQuery = useQuery({
    queryKey: ['drivers'],
    queryFn: settingsApi.getAvailableDrivers,
  });

  const [formData, setFormData] = useState<Partial<UpdateSettingsRequest>>({
    ipmi_mode: 'local',
    ipmi_command_format: 'auto',
    control_interval: 5,
    startup_mode: 'resume',
    emergency_temp: 90,
    emergency_speed: 100,
    warning_temp: 70,
    warning_enabled: true,
    safety_on_shutdown: true,
    thermal_limits_mode: 'hardware',
    warning_margin: 10,
  });

  useEffect(() => {
    if (settings) {
      // Every field the form can send must be loaded here, or saving (which
      // sends the whole form) would reset a stored value to the default.
      setFormData({
        thermal_limits_mode: settings.thermal_limits_mode ?? 'hardware',
        warning_margin: settings.warning_margin ?? 10,
        limit_gpu: settings.limit_gpu ?? 0,
        limit_cpu: settings.limit_cpu ?? 0,
        limit_drive: settings.limit_drive ?? 0,
        ipmi_mode: settings.ipmi_mode,
        ipmi_host: settings.ipmi_host,
        ipmi_user: settings.ipmi_user,
        ipmi_command_format: settings.ipmi_command_format,
        control_interval: settings.control_interval,
        startup_mode: settings.startup_mode,
        startup_percent: settings.startup_percent,
        emergency_temp: settings.emergency_temp,
        emergency_speed: settings.emergency_speed,
        warning_temp: settings.warning_temp,
        warning_enabled: settings.warning_enabled,
        safety_on_shutdown: settings.safety_on_shutdown,
        motherboard_vendor: settings.motherboard_vendor,
        motherboard_model: settings.motherboard_model,
        motherboard_driver: settings.motherboard_driver,
      });
    }
  }, [settings]);

  const updateMutation = useMutation({
    mutationFn: settingsApi.update,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['settings'] });
    },
  });

  const testMutation = useMutation({
    mutationFn: settingsApi.testIPMI,
  });

  const startMutation = useMutation({
    mutationFn: settingsApi.startController,
    onSuccess: (d) => {
      setActionMsg({ text: d.message || 'Controller started' });
      queryClient.invalidateQueries({ queryKey: ['monitoring'] });
    },
    onError: (e: Error) => setActionMsg({ text: e.message, error: true }),
  });

  const stopMutation = useMutation({
    mutationFn: settingsApi.stopController,
    onSuccess: (d) => {
      setActionMsg({ text: d.message || 'Controller stopped' });
      queryClient.invalidateQueries({ queryKey: ['monitoring'] });
    },
    onError: (e: Error) => setActionMsg({ text: e.message, error: true }),
  });

  const changePasswordMutation = useMutation({
    mutationFn: ({ current, next }: { current: string; next: string }) =>
      authApi.changePassword(current, next),
    onSuccess: () => {
      setActionMsg({ text: 'Password changed' });
      setIsChangingPassword(false);
    },
    onError: (e: Error) => setActionMsg({ text: e.message, error: true }),
  });

  const detectMutation = useMutation({
    mutationFn: settingsApi.detectMotherboard,
    onSuccess: (data) => {
      if (data.success && data.driver) {
        setFormData(prev => ({
          ...prev,
          motherboard_vendor: data.driver!.vendor,
          motherboard_model: data.driver!.model,
          motherboard_driver: data.driver!.vendor,
        }));
      }
    },
  });

  const createUserMutation = useMutation({
    mutationFn: ({ username, password, role }: { username: string; password: string; role: string }) =>
      authApi.createUser(username, password, role),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['users'] });
      setIsAddingUser(false);
    },
  });

  const deleteUserMutation = useMutation({
    mutationFn: authApi.deleteUser,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['users'] });
    },
  });

  const handleSave = () => {
    updateMutation.mutate(formData);
  };

  const handleChange = (field: keyof UpdateSettingsRequest, value: string | number | boolean) => {
    setFormData({ ...formData, [field]: value });
  };

  // Thermal limits (warnings and display only — never fan behaviour): hardware
  // mode warns by headroom to each device's own limit; legacy mode is the
  // single warning temperature.
  const hardwareMode = (formData.thermal_limits_mode ?? 'hardware') === 'hardware';
  const warningMargin = formData.warning_margin ?? 10;
  const marginError =
    hardwareMode && (warningMargin < 1 || warningMargin > 40) ? 'Warning margin must be between 1 and 40 °C.' : null;

  // Live "effective limits" rows from the monitoring feed: what each device's
  // limit is, where it came from, and where the current form values would put
  // its warning / emergency points. "—" when the backend hasn't annotated it.
  const limitRows = useMemo(() => {
    type Row = { kind: string; name: string; temp: number; info: ThermalInfo };
    const rows: Row[] = [];
    (monitoring?.gpus ?? []).forEach((g) => rows.push({ kind: `GPU ${g.index}`, name: g.name, temp: g.temperature, info: g }));
    (monitoring?.system?.cpu_packages ?? []).forEach((c) =>
      rows.push({ kind: `CPU ${c.index}`, name: c.model || c.name, temp: c.temperature, info: c })
    );
    (monitoring?.system?.drives ?? []).forEach((d) => rows.push({ kind: `Drive ${d.index}`, name: d.model, temp: d.temperature, info: d }));
    return rows;
  }, [monitoring]);

  const statusText: Record<ThermalStatus, string> = { ok: 'text-ok', warning: 'text-warn', critical: 'text-danger' };
  const overrideFor = (kind: string): number | undefined => {
    const v = kind.startsWith('GPU') ? formData.limit_gpu : kind.startsWith('CPU') ? formData.limit_cpu : formData.limit_drive;
    return v && v > 0 ? v : undefined;
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-fg">Settings</h1>
        <Button onClick={handleSave} isLoading={updateMutation.isPending} disabled={!!marginError} title={marginError ?? undefined}>
          <Save className="w-4 h-4 mr-2" />
          Save Changes
        </Button>
      </div>

      {updateMutation.isSuccess && (
        <div className="p-3 bg-ok/15 border border-ok/30 rounded-lg text-ok">
          Settings saved successfully
        </div>
      )}

      {actionMsg && (
        <div className={`p-3 rounded-lg flex items-center justify-between ${
          actionMsg.error ? 'bg-danger/15 border border-danger/30 text-danger' : 'bg-ok/15 border border-ok/30 text-ok'}`}>
          <span>{actionMsg.text}</span>
          <button className="opacity-70 hover:opacity-100" onClick={() => setActionMsg(null)}>✕</button>
        </div>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* IPMI Connection (BMC drivers only) */}
        {isIpmiDriver && (
        <Card title="IPMI Connection">
          <div className="space-y-4">
            <div>
              <label className="input-label">Mode</label>
              <div className="flex gap-4">
                <label className="flex items-center gap-2">
                  <input
                    type="radio"
                    name="ipmi_mode"
                    value="local"
                    checked={formData.ipmi_mode === 'local'}
                    onChange={(e) => handleChange('ipmi_mode', e.target.value)}
                  />
                  <span className="text-fg-3">Local (/dev/ipmi0)</span>
                </label>
                <label className="flex items-center gap-2">
                  <input
                    type="radio"
                    name="ipmi_mode"
                    value="lan"
                    checked={formData.ipmi_mode === 'lan'}
                    onChange={(e) => handleChange('ipmi_mode', e.target.value)}
                  />
                  <span className="text-fg-3">Network (LAN)</span>
                </label>
              </div>
            </div>

            {formData.ipmi_mode === 'lan' && (
              <>
                <div>
                  <label className="input-label">BMC Host</label>
                  <input
                    type="text"
                    className="input"
                    value={formData.ipmi_host || ''}
                    onChange={(e) => handleChange('ipmi_host', e.target.value)}
                    placeholder="192.168.1.100"
                  />
                </div>
                <div>
                  <label className="input-label">Username</label>
                  <input
                    type="text"
                    className="input"
                    value={formData.ipmi_user || ''}
                    onChange={(e) => handleChange('ipmi_user', e.target.value)}
                    placeholder="admin"
                  />
                </div>
                <div>
                  <label className="input-label">Password</label>
                  <input
                    type="password"
                    className="input"
                    value={formData.ipmi_pass || ''}
                    onChange={(e) => handleChange('ipmi_pass', e.target.value)}
                    placeholder="Leave empty to keep current"
                  />
                </div>
              </>
            )}

            <div>
              <label className="input-label">Command Format</label>
              <select
                className="select"
                value={formData.ipmi_command_format || 'auto'}
                onChange={(e) => handleChange('ipmi_command_format', e.target.value)}
              >
                <option value="auto">Auto-detect</option>
                <option value="asrock_romed8">ASRock Rack ROMED8</option>
                <option value="asrock_legacy">ASRock Rack (Legacy)</option>
                <option value="dell">Dell</option>
                <option value="supermicro">Supermicro</option>
              </select>
              <p className="text-xs text-muted mt-1">
                Override if auto-detection selects the wrong format
              </p>
            </div>

            <Button
              variant="secondary"
              onClick={() => testMutation.mutate()}
              isLoading={testMutation.isPending}
            >
              <TestTube className="w-4 h-4 mr-2" />
              Test Connection
            </Button>

            {testMutation.data && (
              <div className={`p-3 rounded-lg ${
                testMutation.data.success
                  ? 'bg-ok/15 border border-ok/30 text-ok'
                  : 'bg-danger/15 border border-danger/30 text-danger'
              }`}>
                {testMutation.data.success ? (
                  <div className="flex items-center gap-2">
                    <CheckCircle className="w-4 h-4" />
                    {testMutation.data.message}
                  </div>
                ) : (
                  <div className="flex items-center gap-2">
                    <XCircle className="w-4 h-4" />
                    {testMutation.data.error}
                  </div>
                )}
              </div>
            )}
          </div>
        </Card>
        )}

        {/* Motherboard Detection */}
        <Card title="Motherboard Detection">
          <div className="space-y-4">
            <div>
              <label className="input-label">Detected Motherboard</label>
              <div className="text-fg-3">
                {settings?.motherboard_vendor && settings?.motherboard_model
                  ? `${settings.motherboard_vendor} ${settings.motherboard_model}`
                  : 'Not detected'}
              </div>
            </div>

            <div>
              <label className="input-label">Driver (override)</label>
              <select
                className="select"
                value={formData.motherboard_driver || ''}
                onChange={(e) => handleChange('motherboard_driver', e.target.value)}
              >
                <option value="">Auto-detect</option>
                {Array.from(new Set((driversQuery.data || []).map((d) => d.vendor))).map((v) => (
                  <option key={v} value={v}>{v}</option>
                ))}
              </select>
              <p className="text-xs text-muted mt-1">Force a specific driver, then Save to apply.</p>
            </div>

            <Button
              variant="secondary"
              onClick={() => detectMutation.mutate()}
              isLoading={detectMutation.isPending}
            >
              <TestTube className="w-4 h-4 mr-2" />
              Detect Motherboard
            </Button>

            {detectMutation.data && (
              <div className={`p-3 rounded-lg ${
                detectMutation.data.success
                  ? 'bg-ok/15 border border-ok/30 text-ok'
                  : 'bg-danger/15 border border-danger/30 text-danger'
              }`}>
                {detectMutation.data.success && detectMutation.data.driver ? (
                  <div>
                    <div className="font-semibold mb-1">
                      Detected: {detectMutation.data.driver.vendor} {detectMutation.data.driver.model}
                    </div>
                    <div className="text-xs">
                      <div>Zones: {detectMutation.data.driver.zone_layout.zones.length}</div>
                      <div>Max Fans: {detectMutation.data.driver.capabilities.max_fans}</div>
                    </div>
                  </div>
                ) : (
                  <div className="flex items-center gap-2">
                    <XCircle className="w-4 h-4" />
                    {detectMutation.data.error || 'Detection failed'}
                  </div>
                )}
              </div>
            )}
          </div>
        </Card>

        {/* Available Drivers */}
        <Card title="Available Drivers">
          <div className="space-y-4">
            <p className="text-sm text-muted">
              Supported motherboard vendors and their capabilities:
            </p>

            {driversQuery.data?.map((driver) => (
              <div key={`${driver.vendor}-${driver.model}`} className="border border-surface-2 rounded-lg p-3">
                <div className="flex justify-between items-start mb-2">
                  <div>
                    <div className="font-semibold text-fg-2">{driver.vendor}</div>
                    <div className="text-sm text-muted">{driver.model}</div>
                  </div>
                  <span className="text-xs bg-surface-3 px-2 py-1 rounded">
                    {driver.capabilities.max_zones} zones, {driver.capabilities.max_fans} fans
                  </span>
                </div>

                <div className="grid grid-cols-2 gap-2 text-xs">
                  <div>
                    <span className="text-muted">Manual Mode: </span>
                    <span>{driver.capabilities.supports_manual_mode ? '✓' : '✗'}</span>
                  </div>
                  <div>
                    <span className="text-muted">Duty Cycle: </span>
                    <span>{driver.capabilities.supports_duty_cycle_reading ? '✓' : '✗'}</span>
                  </div>
                  <div>
                    <span className="text-muted">Per-Zone: </span>
                    <span>{driver.capabilities.supports_per_zone_control ? '✓' : '✗'}</span>
                  </div>
                  <div>
                    <span className="text-muted">Static RPM: </span>
                    <span>{driver.capabilities.has_static_rpm_values ? '✓' : '✗'}</span>
                  </div>
                </div>

                <div className="mt-2 text-xs text-muted">
                  <strong>Default Zones:</strong> {driver.zone_layout.zones.map(zone => zone.name).join(', ')}
                </div>
              </div>
            ))}

            <Button
              variant="secondary"
              onClick={() => driversQuery.refetch()}
              isLoading={driversQuery.isFetching}
            >
              Refresh Drivers
            </Button>
          </div>
        </Card>

        {/* Control Settings */}
        <Card title="Control Settings">
          <div className="space-y-4">
            <div>
              <label className="input-label">Control Interval (seconds)</label>
              <input
                type="number"
                className="input"
                value={formData.control_interval || 5}
                onChange={(e) => handleChange('control_interval', parseInt(e.target.value))}
                min={1}
                max={60}
              />
            </div>

            <div>
              <label className="input-label">On Startup</label>
              <select
                className="select"
                value={formData.startup_mode || 'resume'}
                onChange={(e) => handleChange('startup_mode', e.target.value)}
              >
                <option value="resume">Resume last profile</option>
                <option value="full">Set all fans to 100%</option>
                <option value="percent">Set all fans to specific %</option>
              </select>
            </div>

            {formData.startup_mode === 'percent' && (
              <div>
                <label className="input-label">Startup Fan Speed (%)</label>
                <input
                  type="number"
                  className="input"
                  value={formData.startup_percent || 50}
                  onChange={(e) => handleChange('startup_percent', parseInt(e.target.value))}
                  min={0}
                  max={100}
                />
              </div>
            )}

            <div className="flex gap-2">
              <Button
                onClick={() => startMutation.mutate()}
                isLoading={startMutation.isPending}
              >
                <Play className="w-4 h-4 mr-2" />
                Start Controller
              </Button>
              <Button
                variant="secondary"
                onClick={() => stopMutation.mutate()}
                isLoading={stopMutation.isPending}
              >
                <Square className="w-4 h-4 mr-2" />
                Stop Controller
              </Button>
            </div>
          </div>
        </Card>

        {/* Safety Thresholds */}
        <Card title="Safety Thresholds">
          <div className="space-y-5">
            {/* ── Warnings & limits: monitoring only. Nothing here changes how
                   fans are driven; that is the Emergency group below. ── */}
            <div>
              <h4 className="font-medium text-fg-2">Warnings &amp; limits</h4>
              <p className="text-xs text-muted mb-3">
                Monitoring only: these decide what is logged and how devices are coloured. They never change fan
                behaviour.
              </p>
              <label className="flex items-start gap-2">
                <input
                  type="checkbox"
                  className="mt-1"
                  checked={hardwareMode}
                  onChange={(e) => handleChange('thermal_limits_mode', e.target.checked ? 'hardware' : 'legacy')}
                />
                <span>
                  <span className="text-fg-3">Use each device's own limit (hardware-derived)</span>
                  <span className="block text-xs text-muted">
                    GPUs report their throttle point (NVML), NVMe drives their critical temperature (hwmon), Intel
                    CPUs their TjMax; anything else gets a conservative class default. A device is "running warm"
                    when it gets within the margin of <em>its</em> limit.
                  </span>
                </span>
              </label>
              <div className="grid grid-cols-2 gap-4 mt-3 items-end">
                {hardwareMode ? (
                  <div>
                    <label className="input-label">Warn when headroom ≤ (°C)</label>
                    <input
                      type="number"
                      className="input"
                      min={1}
                      max={40}
                      value={warningMargin}
                      onChange={(e) => handleChange('warning_margin', parseInt(e.target.value))}
                      disabled={!(formData.warning_enabled ?? true)}
                    />
                  </div>
                ) : (
                  <div>
                    <label className="input-label">Warn when any temperature ≥ (°C)</label>
                    <input
                      type="number"
                      className="input"
                      value={formData.warning_temp || 70}
                      onChange={(e) => handleChange('warning_temp', parseInt(e.target.value))}
                      disabled={!(formData.warning_enabled ?? true)}
                    />
                  </div>
                )}
                <label className="flex items-center gap-2 pb-2">
                  <input
                    type="checkbox"
                    checked={formData.warning_enabled ?? true}
                    onChange={(e) => handleChange('warning_enabled', e.target.checked)}
                  />
                  <span className="text-fg-3">Log warnings</span>
                </label>
              </div>
              {marginError && (
                <p className="text-sm text-danger mt-2" role="alert">
                  {marginError}
                </p>
              )}

              {/* Effective limits: read-only, from the live monitoring feed, with
                  the warning point the current form values imply. */}
              <div className="mt-4">
                <h5 className="text-sm font-medium text-fg-2 mb-2">Effective limits</h5>
                {hardwareMode && (
                  <div className="grid grid-cols-3 gap-3 mb-3">
                    {(
                      [
                        ['limit_gpu', 'GPU override'],
                        ['limit_cpu', 'CPU override'],
                        ['limit_drive', 'Drive override'],
                      ] as const
                    ).map(([field, label]) => (
                      <div key={field}>
                        <label className="input-label">{label} (°C)</label>
                        <input
                          type="number"
                          className="input"
                          min={0}
                          max={120}
                          placeholder="auto"
                          value={formData[field] || ''}
                          onChange={(e) => handleChange(field, e.target.value === '' ? 0 : parseInt(e.target.value))}
                        />
                      </div>
                    ))}
                  </div>
                )}
                {limitRows.length === 0 ? (
                  <p className="text-sm text-muted">No devices reported yet.</p>
                ) : (
                  <table className="w-full text-xs table-fixed">
                    <colgroup>
                      <col />
                      <col className="w-14" />
                      <col className="w-28" />
                      <col className="w-16" />
                      <col className="w-16" />
                    </colgroup>
                    <thead>
                      <tr className="text-left text-[10px] text-muted-2 uppercase tracking-wide">
                        <th className="py-1 pr-2">Device</th>
                        <th className="py-1 pr-2">Now</th>
                        <th className="py-1 pr-2">Limit</th>
                        <th className="py-1 pr-2">Warn at</th>
                        <th className="py-1">Status</th>
                      </tr>
                    </thead>
                    <tbody>
                      {limitRows.map((r) => {
                        const override = overrideFor(r.kind);
                        const limit = hardwareMode ? override ?? r.info.limit : undefined;
                        const source = hardwareMode ? (override != null ? 'override' : r.info.limit_source) : undefined;
                        const warnAt = hardwareMode ? (limit != null ? limit - warningMargin : undefined) : formData.warning_temp;
                        const status = r.info.status;
                        return (
                          <tr key={r.kind} className="border-t border-surface-2 align-top">
                            <td className="py-1.5 pr-2 min-w-0">
                              <span className="text-fg-2">{r.kind}</span>
                              <span className="block text-muted truncate" title={r.name}>
                                {r.name}
                              </span>
                            </td>
                            <td className="py-1.5 pr-2 tabular-nums text-fg-2 whitespace-nowrap">{Math.round(r.temp)} °C</td>
                            <td className="py-1.5 pr-2 tabular-nums text-fg-2 whitespace-nowrap">
                              {limit != null ? `${limit} °C` : '—'}
                              {source && <span className="block text-[10px] text-muted-2">{source}</span>}
                            </td>
                            <td className="py-1.5 pr-2 tabular-nums text-fg-2 whitespace-nowrap">
                              {warnAt != null ? `${warnAt} °C` : '—'}
                            </td>
                            <td className={`py-1.5 capitalize whitespace-nowrap ${status ? statusText[status] : 'text-muted'}`}>
                              {status ?? '—'}
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                )}
              </div>
            </div>

            {/* ── Emergency: the one rule that changes fan behaviour — unchanged. ── */}
            <div className="border-t border-surface-2 pt-4">
              <h4 className="font-medium text-fg-2">Emergency</h4>
              <p className="text-xs text-muted mb-3">
                The one rule that changes fan behaviour: when any GPU, CPU or drive reaches the emergency
                temperature, all fans run at the emergency speed until it drops below it.
              </p>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="input-label">Emergency Temp (°C)</label>
                  <input
                    type="number"
                    className="input"
                    value={formData.emergency_temp || 90}
                    onChange={(e) => handleChange('emergency_temp', parseInt(e.target.value))}
                  />
                </div>
                <div>
                  <label className="input-label">Emergency Speed (%)</label>
                  <input
                    type="number"
                    className="input"
                    value={formData.emergency_speed || 100}
                    onChange={(e) => handleChange('emergency_speed', parseInt(e.target.value))}
                  />
                </div>
              </div>
            </div>

            <div className="border-t border-surface-2 pt-4">
              <label className="flex items-center gap-2">
                <input
                  type="checkbox"
                  checked={formData.safety_on_shutdown ?? true}
                  onChange={(e) => handleChange('safety_on_shutdown', e.target.checked)}
                />
                <span className="text-fg-3">Set fans to 100% on shutdown</span>
              </label>
              <p className="text-sm text-muted mt-1">
                When enabled, fans will be set to maximum speed when the controller stops.
              </p>
            </div>
          </div>
        </Card>

        {/* Account */}
        <Card title="Account">
          <div className="space-y-3">
            <p className="text-sm text-muted">
              Signed in as <span className="text-fg-2">{user?.username}</span> ({user?.role}).
            </p>
            <Button variant="secondary" onClick={() => setIsChangingPassword(true)}>
              <KeyRound className="w-4 h-4 mr-2" />
              Change Password
            </Button>
          </div>
        </Card>

        {/* User Management (Admin only) */}
        {user?.role === 'admin' && (
          <Card
            title="User Management"
            action={
              <Button size="sm" onClick={() => setIsAddingUser(true)}>
                <Plus className="w-4 h-4 mr-1" />
                Add User
              </Button>
            }
          >
            <div className="space-y-2">
              {(users as User[] || []).map((u) => (
                <div
                  key={u.id}
                  className="flex items-center justify-between p-3 bg-surface-2/50 rounded-lg"
                >
                  <div>
                    <p className="font-medium text-fg-2">{u.username}</p>
                    <p className="text-sm text-muted">
                      {u.role} • Last login: {u.last_login ? new Date(u.last_login).toLocaleString() : 'Never'}
                    </p>
                  </div>
                  {u.id !== user?.id && (
                    <Button
                      variant="danger"
                      size="sm"
                      onClick={() => {
                        if (confirm(`Delete user ${u.username}?`)) {
                          deleteUserMutation.mutate(u.id);
                        }
                      }}
                      isLoading={deleteUserMutation.isPending}
                    >
                      <Trash2 className="w-4 h-4" />
                    </Button>
                  )}
                </div>
              ))}
            </div>
          </Card>
        )}
      </div>

      {/* Add User Modal */}
      <AddUserModal
        isOpen={isAddingUser}
        onClose={() => setIsAddingUser(false)}
        onSave={(username, password, role) =>
          createUserMutation.mutate({ username, password, role })
        }
        isLoading={createUserMutation.isPending}
      />

      {/* Change Password Modal */}
      <ChangePasswordModal
        isOpen={isChangingPassword}
        onClose={() => setIsChangingPassword(false)}
        onSave={(current, next) => changePasswordMutation.mutate({ current, next })}
        isLoading={changePasswordMutation.isPending}
      />
    </div>
  );
}

function ChangePasswordModal({ isOpen, onClose, onSave, isLoading }: {
  isOpen: boolean; onClose: () => void; onSave: (current: string, next: string) => void; isLoading: boolean;
}) {
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [confirm, setConfirm] = useState('');
  const mismatch = next.length > 0 && next !== confirm;
  const weak = next.length > 0 && next.length < 8;

  return (
    <Modal isOpen={isOpen} onClose={onClose} title="Change Password">
      <div className="space-y-4">
        <div>
          <label className="input-label">Current password</label>
          <input type="password" className="input" value={current} onChange={(e) => setCurrent(e.target.value)} />
        </div>
        <div>
          <label className="input-label">New password</label>
          <input type="password" className="input" value={next} onChange={(e) => setNext(e.target.value)} />
          <p className="text-xs text-muted-2 mt-1">At least 8 chars with upper, lower, digit, and a symbol.</p>
        </div>
        <div>
          <label className="input-label">Confirm new password</label>
          <input type="password" className="input" value={confirm} onChange={(e) => setConfirm(e.target.value)} />
        </div>
        {(mismatch || weak) && (
          <p className="text-sm text-danger">{mismatch ? 'Passwords do not match' : 'Password is too short'}</p>
        )}
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={onClose}>Cancel</Button>
          <Button onClick={() => onSave(current, next)} isLoading={isLoading}
            disabled={!current || !next || mismatch || weak}>
            Update
          </Button>
        </div>
      </div>
    </Modal>
  );
}

function AddUserModal({
  isOpen,
  onClose,
  onSave,
  isLoading,
}: {
  isOpen: boolean;
  onClose: () => void;
  onSave: (username: string, password: string, role: string) => void;
  isLoading: boolean;
}) {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [role, setRole] = useState('user');

  const handleSubmit = () => {
    onSave(username, password, role);
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} title="Add User">
      <div className="space-y-4">
        <div>
          <label className="input-label">Username</label>
          <input
            type="text"
            className="input"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
          />
        </div>
        <div>
          <label className="input-label">Password</label>
          <input
            type="password"
            className="input"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </div>
        <div>
          <label className="input-label">Role</label>
          <select
            className="select"
            value={role}
            onChange={(e) => setRole(e.target.value)}
          >
            <option value="user">User</option>
            <option value="admin">Admin</option>
          </select>
        </div>
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button
            onClick={handleSubmit}
            isLoading={isLoading}
            disabled={!username || !password}
          >
            Create User
          </Button>
        </div>
      </div>
    </Modal>
  );
}
