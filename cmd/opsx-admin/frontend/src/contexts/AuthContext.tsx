import React, { createContext, useContext, useState, useEffect, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { setAuthToken, clearAuthToken, login as apiLogin } from '@/services/api';

interface AuthContextType {
  token: string | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  login: (token: string) => Promise<boolean>;
  logout: () => void;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export const useAuth = () => {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
};

export const AuthProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [token, setToken] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const navigate = useNavigate();

  useEffect(() => {
    // Check for stored token on mount
    const storedToken = localStorage.getItem('opsx_admin_token');
    if (storedToken) {
      setToken(storedToken);
      setAuthToken(storedToken);
    }
    setIsLoading(false);
  }, []);

  const login = useCallback(async (newToken: string): Promise<boolean> => {
    try {
      // Validate token by making a handshake request
      const result = await apiLogin(newToken);
      if (result.success) {
        setToken(result.token);
        setAuthToken(result.token);
        localStorage.setItem('opsx_admin_token', result.token);
        return true;
      }
      return false;
    } catch {
      return false;
    }
  }, []);

  const logout = useCallback(() => {
    setToken(null);
    clearAuthToken();
    localStorage.removeItem('opsx_admin_token');
    navigate('/login');
  }, [navigate]);

  return (
    <AuthContext.Provider
      value={{
        token,
        isAuthenticated: !!token,
        isLoading,
        login,
        logout,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
};
