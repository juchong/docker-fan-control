import { lazy, Suspense } from 'react';
import { BrowserRouter as Router, Routes, Route } from 'react-router-dom';
import { AuthProvider } from './components/auth/AuthProvider';
import { ProtectedRoute } from './components/auth/ProtectedRoute';
import { DashboardLayout } from './components/layout/DashboardLayout';
import { Loader2 } from 'lucide-react';

const Login = lazy(() => import('./pages/Login').then((m) => ({ default: m.Login })));
const Dashboard = lazy(() => import('./pages/Dashboard').then((m) => ({ default: m.Dashboard })));
const Fans = lazy(() => import('./pages/Fans').then((m) => ({ default: m.Fans })));
const Profiles = lazy(() => import('./pages/Profiles').then((m) => ({ default: m.Profiles })));
const Logs = lazy(() => import('./pages/Logs').then((m) => ({ default: m.Logs })));
const Settings = lazy(() => import('./pages/Settings').then((m) => ({ default: m.Settings })));

function PageLoader() {
  return (
    <div className="flex items-center justify-center py-20 text-muted">
      <Loader2 className="w-6 h-6 animate-spin" aria-label="Loading" />
    </div>
  );
}

function App() {
  return (
    <AuthProvider>
      <Router>
        <Suspense fallback={<PageLoader />}>
          <Routes>
            <Route path="/login" element={<Login />} />

            <Route
              path="/*"
              element={
                <ProtectedRoute>
                  <DashboardLayout>
                    <Suspense fallback={<PageLoader />}>
                      <Routes>
                        <Route path="/" element={<Dashboard />} />
                        <Route path="/fans" element={<Fans />} />
                        <Route path="/profiles" element={<Profiles />} />
                        <Route path="/logs" element={<Logs />} />
                        <Route path="/settings" element={<Settings />} />
                      </Routes>
                    </Suspense>
                  </DashboardLayout>
                </ProtectedRoute>
              }
            />
          </Routes>
        </Suspense>
      </Router>
    </AuthProvider>
  );
}

export default App;
