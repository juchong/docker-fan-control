import { useAuthContext } from '../auth/AuthProvider';
import { useMonitoring } from '../../hooks/useMonitoring';
import { User, LogOut, Wifi, WifiOff } from 'lucide-react';

export function Header() {
  const { user, logout } = useAuthContext();
  const { isConnected, data } = useMonitoring();

  return (
    <header className="h-16 bg-slate-800 border-b border-slate-700 flex items-center justify-between px-6">
      <div className="flex items-center gap-4">
        {data?.controller && (
          <div className="flex items-center gap-2">
            <span className={`w-2 h-2 rounded-full ${data.controller.running ? 'bg-green-500' : 'bg-red-500'}`} />
            <span className="text-sm text-slate-400">
              {data.controller.running ? 'Controller Running' : 'Controller Stopped'}
            </span>
            {data.controller.active_profile && (
              <span className="text-sm text-slate-300">
                - {data.controller.active_profile}
              </span>
            )}
          </div>
        )}
      </div>

      <div className="flex items-center gap-4">
        {/* Connection Status */}
        <div className="flex items-center gap-2">
          {isConnected ? (
            <Wifi className="w-4 h-4 text-green-500" />
          ) : (
            <WifiOff className="w-4 h-4 text-red-500" />
          )}
          <span className="text-xs text-slate-400">
            {isConnected ? 'Connected' : 'Disconnected'}
          </span>
        </div>

        {/* User Menu */}
        {user && (
          <div className="flex items-center gap-3">
            <div className="flex items-center gap-2 px-3 py-1.5 bg-slate-700 rounded-lg">
              <User className="w-4 h-4 text-slate-400" />
              <span className="text-sm text-slate-300">{user.username}</span>
              {user.role === 'admin' && (
                <span className="text-xs bg-primary-600 text-white px-1.5 py-0.5 rounded">
                  Admin
                </span>
              )}
            </div>
            <button
              onClick={logout}
              className="p-2 text-slate-400 hover:text-slate-200 hover:bg-slate-700 rounded-lg transition-colors"
              title="Logout"
            >
              <LogOut className="w-5 h-5" />
            </button>
          </div>
        )}
      </div>
    </header>
  );
}
