import React, { createContext, useContext, useEffect, useState, type ReactNode } from 'react';
import type { User } from '../types/enterprise';
import { enterpriseAPI } from '../services/enterprise/api';

interface EnterpriseAuthContextType {
  isAuthenticated: boolean;
  isLoading: boolean;
  user: User | null;
  accessToken: string | null;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  refreshToken: () => Promise<void>;
  hasRole: (role: string) => boolean;
  hasPermission: (permission: string) => boolean;
}

const EnterpriseAuthContext = createContext<EnterpriseAuthContextType | undefined>(undefined);

export const useEnterpriseAuth = () => {
  const context = useContext(EnterpriseAuthContext);
  if (context === undefined) {
    throw new Error('useEnterpriseAuth must be used within an EnterpriseAuthProvider');
  }
  return context;
};

interface EnterpriseAuthProviderProps {
  children: ReactNode;
  enabled?: boolean;
}

export const EnterpriseAuthProvider: React.FC<EnterpriseAuthProviderProps> = ({ children, enabled = false }) => {
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const [user, setUser] = useState<User | null>(null);
  const [accessToken, setAccessToken] = useState<string | null>(null);

  // Role permissions mapping
  const rolePermissions: Record<string, string[]> = {
    admin: [
      'providers:read',
      'providers:write',
      'rules:read',
      'rules:write',
      'usage:read',
      'users:read',
      'users:write',
      'tokens:read',
      'tokens:write',
      'audit:read',
    ],
    user: [
      'providers:read',
      'providers:write',
      'rules:read',
      'rules:write',
      'usage:read',
      'tokens:read',
      'tokens:write',
    ],
    readonly: [
      'providers:read',
      'rules:read',
      'usage:read',
    ],
  };

  const login = async (username: string, password: string) => {
    const response = await enterpriseAPI.login({ username, password });

    // Store tokens
    localStorage.setItem('enterprise_access_token', response.access_token);
    localStorage.setItem('enterprise_refresh_token', response.refresh_token);
    setAccessToken(response.access_token);
    setUser(response.user);
    setIsAuthenticated(true);
  };

  const logout = async () => {
    try {
      await enterpriseAPI.logout();
    } catch (error) {
      // Ignore logout errors
    } finally {
      localStorage.removeItem('enterprise_access_token');
      localStorage.removeItem('enterprise_refresh_token');
      setAccessToken(null);
      setUser(null);
      setIsAuthenticated(false);
    }
  };

  const refreshToken = async () => {
    const refreshTokenValue = localStorage.getItem('enterprise_refresh_token');
    if (!refreshTokenValue) {
      throw new Error('No refresh token available');
    }

    const response = await enterpriseAPI.refreshToken({ refresh_token: refreshTokenValue });

    localStorage.setItem('enterprise_access_token', response.access_token);
    setAccessToken(response.access_token);
  };

  const hasRole = (role: string): boolean => {
    if (!user) return false;
    return user.role === role;
  };

  const hasPermission = (permission: string): boolean => {
    if (!user) return false;

    const permissions = rolePermissions[user.role] || [];
    return permissions.includes(permission);
  };

  useEffect(() => {
    if (!enabled) {
      setIsLoading(false);
      return;
    }

    const initializeAuth = async () => {
      try {
        const storedToken = localStorage.getItem('enterprise_access_token');
        if (storedToken) {
          setAccessToken(storedToken);

          // Validate token and get user info
          try {
            const currentUser = await enterpriseAPI.getCurrentUser();
            setUser(currentUser);
            setIsAuthenticated(true);
          } catch (error) {
            // Token is invalid, clear it
            localStorage.removeItem('enterprise_access_token');
            localStorage.removeItem('enterprise_refresh_token');
            setAccessToken(null);
          }
        }
      } catch (error) {
        console.error('Auth initialization error:', error);
      } finally {
        setIsLoading(false);
      }
    };

    initializeAuth();
  }, [enabled]);

  const value: EnterpriseAuthContextType = {
    isAuthenticated,
    isLoading,
    user,
    accessToken,
    login,
    logout,
    refreshToken,
    hasRole,
    hasPermission,
  };

  return (
    <EnterpriseAuthContext.Provider value={value}>
      {children}
    </EnterpriseAuthContext.Provider>
  );
};
