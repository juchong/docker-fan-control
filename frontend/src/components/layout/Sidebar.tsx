import { NavLink } from 'react-router-dom';
import {
  LayoutDashboard,
  Fan,
  Settings2,
  FileText,
  Settings,
  Thermometer,
  X,
} from 'lucide-react';

const navItems = [
  { to: '/', icon: LayoutDashboard, label: 'Dashboard', end: true },
  { to: '/fans', icon: Fan, label: 'Fans' },
  { to: '/profiles', icon: Settings2, label: 'Profiles' },
  { to: '/logs', icon: FileText, label: 'Logs' },
  { to: '/settings', icon: Settings, label: 'Settings' },
];

interface SidebarProps {
  /** Mobile drawer open state; ignored on lg+ where the sidebar is always shown. */
  open?: boolean;
  onClose?: () => void;
}

export function Sidebar({ open = false, onClose }: SidebarProps) {
  return (
    <>
      {/* Mobile backdrop */}
      <div
        className={`lg:hidden fixed inset-0 z-30 bg-black/50 transition-opacity ${
          open ? 'opacity-100' : 'opacity-0 pointer-events-none'
        }`}
        onClick={onClose}
        aria-hidden="true"
      />

      <aside
        className={`w-64 bg-surface border-r border-line flex flex-col z-40
          fixed inset-y-0 left-0 transition-transform duration-200
          lg:static lg:translate-x-0
          ${open ? 'translate-x-0' : '-translate-x-full'}`}
        aria-label="Primary navigation"
      >
        <div className="p-4 border-b border-line flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="p-2 bg-primary-600 rounded-lg">
              <Thermometer className="w-6 h-6 text-white" />
            </div>
            <div>
              <h1 className="text-lg font-bold text-fg">Fan Control</h1>
              <p className="text-xs text-muted">Thermal Management</p>
            </div>
          </div>
          {/* Close button (mobile only) */}
          <button
            onClick={onClose}
            className="lg:hidden p-1 text-muted hover:text-fg-2 rounded transition-colors"
            aria-label="Close navigation menu"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        <nav className="flex-1 p-4">
          <ul className="space-y-1">
            {navItems.map(({ to, icon: Icon, label, end }) => (
              <li key={to}>
                <NavLink
                  to={to}
                  end={end}
                  onClick={onClose}
                  className={({ isActive }) =>
                    `flex items-center gap-3 px-3 py-2 rounded-lg transition-colors ${
                      isActive
                        ? 'bg-primary-600 text-white'
                        : 'text-muted hover:text-fg-2 hover:bg-surface-2'
                    }`
                  }
                >
                  <Icon className="w-5 h-5" aria-hidden="true" />
                  <span>{label}</span>
                </NavLink>
              </li>
            ))}
          </ul>
        </nav>

        <div className="p-4 border-t border-line">
          <a
            href="https://github.com/juchong/docker-fan-control"
            target="_blank"
            rel="noopener noreferrer"
            className="block text-xs text-muted-2 text-center hover:text-muted transition-colors"
          >
            docker-fan-control
          </a>
        </div>
      </aside>
    </>
  );
}
