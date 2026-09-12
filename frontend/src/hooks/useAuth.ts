import { useState, useEffect, useCallback } from 'react';
import { authApi, ApiError } from '../services/api';
import { wsService } from '../services/websocket';
import { User, AuthState } from '../types/auth';

export function useAuth() {
  const [state, setState] = useState<AuthState>({
    user: null,
    token: localStorage.getItem('auth_token'),
    isAuthenticated: false,
    isLoading: true,
  });

  const checkAuth = useCallback(async () => {
    const token = localStorage.getItem('auth_token');
    if (!token) {
      setState({
        user: null,
        token: null,
        isAuthenticated: false,
        isLoading: false,
      });
      return;
    }

    try {
      const user = await authApi.me() as User;
      setState({
        user,
        token,
        isAuthenticated: true,
        isLoading: false,
      });
    } catch (error) {
      // Token might be expired
      localStorage.removeItem('auth_token');
      setState({
        user: null,
        token: null,
        isAuthenticated: false,
        isLoading: false,
      });
    }
  }, []);

  useEffect(() => {
    checkAuth();
  }, [checkAuth]);

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
    // Stop the live feed so it doesn't 401-loop against the now-invalid session.
    wsService.disconnect();
    localStorage.removeItem('auth_token');
    setState({
      user: null,
      token: null,
      isAuthenticated: false,
      isLoading: false,
    });
  };

  const refreshToken = async () => {
    try {
      const response = await authApi.refresh();
      localStorage.setItem('auth_token', response.token);
      setState((prev) => ({ ...prev, token: response.token }));
    } catch {
      await logout();
    }
  };

  return {
    ...state,
    login,
    logout,
    refreshToken,
    checkAuth,
  };
}
