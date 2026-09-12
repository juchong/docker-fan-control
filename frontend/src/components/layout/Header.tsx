import { useAuthContext } from '../auth/AuthProvider';
import { useMonitoring } from '../../hooks/useMonitoring';
import { useTheme } from '../../hooks/useTheme';
import { User, LogOut, Wifi, WifiOff, Menu, Sun, Moon } from 'lucide-react';

interface HeaderProps {
  onMenuClick?: () => void;
}

export function Header({ onMenuClick }: HeaderProps) {
  const { user, logout } = useAuthContext();
  const { isConnected, data } = useMonitoring();
  const { theme, toggleTheme } = useTheme();

  const running = data?.controller?.running;
  const activeProfiles = data?.controller?.active_profiles ?? [];

  return (
    <header className="h-16 bg-surface border-b border-line flex items-center justify-between gap-3 px-4 sm:px-6">
      <div className="flex items-center gap-3 min-w-0">
        {/* Mobile nav toggle */}
        <button
          onClick={onMenuClick}
          className="lg:hidden p-2 -ml-2 text-muted hover:text-fg-2 hover:bg-surface-2 rounded-lg transition-colors"
          aria-label="Open navigation menu"
        >
          <Menu className="w-5 h-5" />
        </button>

        {data?.controller && (
          <div className="flex items-center gap-2 min-w-0">
            <span
              className={`shrink-0 w-2 h-2 rounded-full ${running ? 'bg-ok' : 'bg-danger'}`}
              aria-hidden="true"
            />
            <span className="text-sm text-muted whitespace-nowrap">
              {running ? 'Controller Running' : 'Controller Stopped'}
            </span>
            {activeProfiles.length > 0 && (
              <span className="hidden sm:inline text-sm text-fg-3 truncate" title={activeProfiles.join(', ')}>
                · {activeProfiles.join(', ')}
              </span>
            )}
          </div>
        )}
      </div>

      <div className="flex items-center gap-2 sm:gap-4 shrink-0">
        {/* Connection status */}
        <div
          className="flex items-center gap-2"
          title={isConnected ? 'Live updates connected' : 'Live updates disconnected — polling'}
        >
          {isConnected ? (
            <Wifi className="w-4 h-4 text-ok" aria-hidden="true" />
          ) : (
            <WifiOff className="w-4 h-4 text-danger" aria-hidden="true" />
          )}
          <span className="hidden sm:inline text-xs text-muted">
            {isConnected ? 'Connected' : 'Disconnected'}
          </span>
          <span className="sr-only">{isConnected ? 'Connected' : 'Disconnected'}</span>
        </div>

        {/* Theme toggle */}
        <button
          onClick={toggleTheme}
          className="p-2 text-muted hover:text-fg-2 hover:bg-surface-2 rounded-lg transition-colors"
          aria-label={`Switch to ${theme === 'dark' ? 'light' : 'dark'} theme`}
          title={`Switch to ${theme === 'dark' ? 'light' : 'dark'} theme`}
        >
          {theme === 'dark' ? <Sun className="w-5 h-5" /> : <Moon className="w-5 h-5" />}
        </button>

        {/* User menu */}
        {user && (
          <div className="flex items-center gap-2 sm:gap-3">
            <div className="hidden sm:flex items-center gap-2 px-3 py-1.5 bg-surface-2 rounded-lg">
              <User className="w-4 h-4 text-muted" aria-hidden="true" />
              <span className="text-sm text-fg-3">{user.username}</span>
              {user.role === 'admin' && (
                <span className="text-xs bg-primary-600 text-white px-1.5 py-0.5 rounded">
                  Admin
                </span>
              )}
            </div>
            <button
              onClick={logout}
              className="p-2 text-muted hover:text-fg-2 hover:bg-surface-2 rounded-lg transition-colors"
              aria-label="Log out"
              title="Log out"
            >
              <LogOut className="w-5 h-5" />
            </button>
          </div>
        )}
      </div>
    </header>
  );
}
