import React, { createContext, useContext, useEffect, useState, type ReactNode } from 'react';
import { api } from '../services/api';

interface AuthContextType {
    token: string | null;
    isAuthenticated: boolean;
    isLoading: boolean;
    login: (token: string) => Promise<void>;
    logout: () => void;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export const useAuth = () => {
    const context = useContext(AuthContext);
    if (context === undefined) {
        throw new Error('useAuth must be used within an AuthProvider');
    }
    return context;
};

interface AuthProviderProps {
    children: ReactNode;
}

export const AuthProvider: React.FC<AuthProviderProps> = ({ children }) => {
    const [token, setToken] = useState<string | null>(null);
    const [isLoading, setIsLoading] = useState(true);

    const isAuthenticated = !!token;

    const login = async (newToken: string) => {
        setToken(newToken);
        localStorage.setItem('user_auth_token', newToken);
        // Initialize API instances with the new token
        await api.initialize();
    };

    const logout = () => {
        setToken(null);
        localStorage.removeItem('user_auth_token');
    };

    useEffect(() => {
        const initializeAuth = async () => {
            try {
                // First, check if there's a token in URL parameters
                // Support both 'token' and 'user_auth_token' parameters
                const urlParams = new URLSearchParams(window.location.search);
                const urlToken = urlParams.get('token') || urlParams.get('user_auth_token');

                let finalToken = null;

                if (urlToken) {
                    // Use URL token
                    finalToken = urlToken;
                    localStorage.setItem('user_auth_token', urlToken);

                    // Clean up URL by removing the token parameter (for security and aesthetics)
                    const cleanPath = window.location.pathname;
                    const hash = window.location.hash;
                    const cleanUrl = cleanPath + hash;
                    window.history.replaceState({}, '', cleanUrl);
                } else {
                    // If no URL token, check localStorage
                    const storedToken = localStorage.getItem('user_auth_token');
                    if (storedToken) {
                        finalToken = storedToken;
                    }
                }

                // Validate token (basic validation - you can add more sophisticated checks)
                if (finalToken && finalToken.trim() !== '') {
                    setToken(finalToken);
                    // Initialize API instances with the token
                    await api.initialize();
                }
            } catch (error) {
                console.error('Auth initialization error:', error);
                // Clear potentially corrupted data
                localStorage.removeItem('user_auth_token');
            } finally {
                // Authentication check complete
                setIsLoading(false);
            }
        };

        initializeAuth();
    }, []);

    return (
        <AuthContext.Provider value={{ token, isAuthenticated, isLoading, login, logout }}>
            {children}
        </AuthContext.Provider>
    );
};