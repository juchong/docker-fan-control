import { AlertTriangle, WifiOff, OctagonAlert } from 'lucide-react';
import { useMonitoring } from '../../hooks/useMonitoring';

type Severity = 'danger' | 'warning' | 'info';

interface Banner {
  severity: Severity;
  icon: React.ComponentType<{ className?: string }>;
  message: string;
}

const SEV_CLS: Record<Severity, string> = {
  danger: 'bg-danger/15 border-danger/40 text-danger',
  warning: 'bg-warn/15 border-warn/40 text-warn',
  info: 'bg-info/15 border-info/40 text-info',
};

// App-wide strip that surfaces the most important degraded condition from the
// live monitoring feed, so the UI warns instead of silently misbehaving.
export function DegradedBanner() {
  const { data, isConnected } = useMonitoring();
  const controller = data?.controller;

  if (!data) return null;

  const banner: Banner | null = (() => {
    const caps = controller?.driver_capabilities;
    const noDriver =
      controller?.driver_vendor === 'Generic' || caps?.supports_manual_mode === false;

    if (noDriver) {
      return {
        severity: 'danger',
        icon: OctagonAlert,
        message:
          'No controllable fan driver is active — fan speeds are not being managed. Check the driver in Settings.',
      };
    }
    if (controller && controller.running === false) {
      return {
        severity: 'warning',
        icon: AlertTriangle,
        message:
          'The fan controller is stopped. Fans are running at firmware defaults until you start it.',
      };
    }
    // Driver-reported health issues, e.g. the board firmware/EC re-asserting
    // its own duty over what we command (Gigabyte boards need the BIOS Smart
    // Fan handed over, or the it87 fork's MMIO path).
    const warnings = controller?.driver_warnings ?? [];
    if (warnings.length > 0) {
      return {
        severity: 'warning',
        icon: AlertTriangle,
        message: `${warnings[0]}${warnings.length > 1 ? ` (+${warnings.length - 1} more)` : ''}`,
      };
    }
    if (!isConnected) {
      return {
        severity: 'info',
        icon: WifiOff,
        message: 'Live updates are disconnected — showing polled data while reconnecting.',
      };
    }
    return null;
  })();

  if (!banner) return null;
  const Icon = banner.icon;

  return (
    <div
      role={banner.severity === 'danger' ? 'alert' : 'status'}
      className={`flex items-center gap-2 px-4 py-2.5 border-b text-sm ${SEV_CLS[banner.severity]}`}
    >
      <Icon className="w-4 h-4 shrink-0" aria-hidden="true" />
      <span className="text-fg-2">{banner.message}</span>
    </div>
  );
}
