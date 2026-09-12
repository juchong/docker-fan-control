import { useState, useEffect, useCallback } from 'react';
import { authApi, ApiError, setUnauthorizedHandler } from '../services/api';
import { wsService } from '../services/websocket';
import { User, AuthState } from '../types/auth';

export function useAuth() {
  const [state, setState] = useState<AuthState>({
    user: null,
    token: localStorage.getItem('auth_token'),
    isAuthenticated: false,
    isLoading: true,
  });

  // Clear local session without calling the (already-invalid) logout endpoint.
  const forceLoggedOut = useCallback(() => {
    wsService.disconnect();
    localStorage.removeItem('auth_token');
    setState({ user: null, token: null, isAuthenticated: false, isLoading: false });
  }, []);

  const checkAuth = useCallback(async () => {
    const token = localStorage.getItem('auth_token');

    if (!token) {
      // No token: auth may be disabled, or a reverse proxy may inject identity.
      try {
        const status = await authApi.status();
        if (!status.auth_enabled) {
          setState({
            user: { username: 'local', role: 'admin' } as User,
            token: null, isAuthenticated: true, isLoading: false,
          });
          return;
        }
        if (status.proxy_enabled) {
          const user = (await authApi.me()) as User;
          setState({ user, token: null, isAuthenticated: true, isLoading: false });
          return;
        }
      } catch {
        // status unavailable or proxy me() failed → treat as unauthenticated
      }
      setState({ user: null, token: null, isAuthenticated: false, isLoading: false });
      return;
    }

    try {
      const user = (await authApi.me()) as User;
      setState({ user, token, isAuthenticated: true, isLoading: false });
    } catch {
      forceLoggedOut();
    }
  }, [forceLoggedOut]);

  useEffect(() => {
    checkAuth();
  }, [checkAuth]);

  // Route any 401 from an authenticated call into a clean logout.
  useEffect(() => {
    setUnauthorizedHandler(() => forceLoggedOut());
    return () => setUnauthorizedHandler(null);
  }, [forceLoggedOut]);

  const login = async (username: string, password: string) => {
    try {
      const response = await authApi.login(username, password);
      localStorage.setItem('auth_token', response.token);
      setState({
        user: response.user as User,
        token: response.token,
        isAuthenticated: true,
        isLoading: false,
      });
      return { success: true };
    } catch (error) {
      if (error instanceof ApiError) {
        return { success: false, error: error.message };
      }
      return { success: false, error: 'Login failed' };
    }
  };

  const logout = async () => {
    try {
      await authApi.logout();
    } catch {
      // Ignore errors
    }
    forceLoggedOut();
  };

  const refreshToken = useCallback(async () => {
    try {
      const response = await authApi.refresh();
      localStorage.setItem('auth_token', response.token);
      setState((prev) => ({ ...prev, token: response.token }));
    } catch {
      forceLoggedOut();
    }
  }, [forceLoggedOut]);

  // Periodically refresh the token so a long-lived session doesn't expire
  // mid-use (TTL defaults to 24h).
  useEffect(() => {
    if (!state.isAuthenticated || !state.token) return;
    const interval = setInterval(() => { refreshToken(); }, 30 * 60 * 1000);
    return () => clearInterval(interval);
  }, [state.isAuthenticated, state.token, refreshToken]);

  return {
    ...state,
    login,
    logout,
    refreshToken,
    changePassword: authApi.changePassword,
    checkAuth,
  };
}
