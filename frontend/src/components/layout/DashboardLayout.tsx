import React, { useState, useEffect } from 'react';
import { useLocation } from 'react-router-dom';
import { Sidebar } from './Sidebar';
import { Header } from './Header';
import { DegradedBanner } from '../common/DegradedBanner';
import { useMonitoring } from '../../hooks/useMonitoring';
import { recordSample } from '../../hooks/useMetricsHistory';
import { recordFanHealth } from '../../hooks/useFanHealth';

interface DashboardLayoutProps {
  children: React.ReactNode;
}

// Isolated leaf: records metric history + fan health from the live feed on
// every page. Kept separate so its per-frame re-renders don't re-render the
// whole shell.
function LiveRecorder() {
  const { data } = useMonitoring();
  useEffect(() => {
    recordSample(data);
    recordFanHealth(data);
  }, [data]);
  return null;
}

export function DashboardLayout({ children }: DashboardLayoutProps) {
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const location = useLocation();

  // Close the mobile drawer whenever the route changes.
  useEffect(() => {
    setSidebarOpen(false);
  }, [location.pathname]);

  return (
    <div className="min-h-screen bg-app text-fg flex">
      <LiveRecorder />
      <Sidebar open={sidebarOpen} onClose={() => setSidebarOpen(false)} />
      <div className="flex-1 flex flex-col min-w-0">
        <Header onMenuClick={() => setSidebarOpen(true)} />
        <DegradedBanner />
        <main className="flex-1 p-4 sm:p-6 overflow-auto">{children}</main>
      </div>
    </div>
  );
}
