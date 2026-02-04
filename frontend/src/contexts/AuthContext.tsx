import React, { createContext, useContext, useEffect, useState, type ReactNode } from 'react';
import { api } from '../services/api';
import { authState } from '../services/authState';
import {
    Dialog,
    DialogTitle,
    DialogContent,
    DialogActions,
    Button,
    Box,
    Typography,
    IconButton,
} from '@mui/material';
import ErrorOutlineIcon from '@mui/icons-material/ErrorOutline';

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

// Auth prompt dialog component
const AuthPromptDialog: React.FC<{
    open: boolean;
    onGoToLogin: () => void;
}> = ({ open, onGoToLogin }) => {
    return (
        <Dialog
            open={open}
            onClose={() => {}}
            maxWidth="sm"
            fullWidth
            PaperProps={{
                sx: {
                    borderRadius: 2,
                    boxShadow: '0 8px 32px rgba(0,0,0,0.1)',
                }
            }}
        >
            <DialogTitle sx={{
                display: 'flex',
                alignItems: 'center',
                gap: 1,
                pb: 1,
            }}>
                <ErrorOutlineIcon color="warning" sx={{ fontSize: 28 }} />
                <Typography variant="h6" component="span">
                    Session Expired
                </Typography>
            </DialogTitle>
            <DialogContent>
                <Typography variant="body1" color="text.secondary">
                    Your authentication token has expired or is invalid. Please log in again to continue.
                </Typography>
            </DialogContent>
            <DialogActions sx={{ px: 3, pb: 2 }}>
                <Button
                    variant="contained"
                    onClick={onGoToLogin}
                    sx={{ minWidth: 120 }}
                >
                    Go to Login
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export const AuthProvider: React.FC<AuthProviderProps> = ({ children }) => {
    const [token, setToken] = useState<string | null>(null);
    const [isLoading, setIsLoading] = useState(true);
    const [authPromptOpen, setAuthPromptOpen] = useState(false);

    const isAuthenticated = !!token;

    const login = async (newToken: string) => {
        setToken(newToken);
        localStorage.setItem('user_auth_token', newToken);
        // Reset auth state to authenticated
        authState.setAuthenticated();
        setAuthPromptOpen(false);
        // Initialize API instances with the new token
        await api.initialize();
    };

    const logout = () => {
        setToken(null);
        localStorage.removeItem('user_auth_token');
    };

    const handleGoToLogin = () => {
        setAuthPromptOpen(false);
        // Navigate to login using React Router
        window.location.href = '/login';
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

    // Listen for auth state changes
    useEffect(() => {
        // Subscribe to auth state changes from API layer (401 handling)
        const unsubscribe = authState.onChange((state) => {
            if (state === 'unauthorized') {
                // Clear token state
                setToken(null);
                // Only show prompt if not already on login page
                if (window.location.pathname !== '/login') {
                    setAuthPromptOpen(true);
                }
            }
        });

        // Storage event for cross-tab sync
        const handleStorageChange = (e: StorageEvent) => {
            if (e.key === 'user_auth_token') {
                if (e.newValue === null) {
                    setToken(null);
                } else if (e.newValue && e.newValue.trim() !== '') {
                    setToken(e.newValue);
                }
            }
        };

        // Custom event for additional cross-tab compatibility
        const handleAuthStateChange = (e: CustomEvent<{ type: 'logout' | 'login'; token?: string }>) => {
            if (e.detail.type === 'logout') {
                setToken(null);
            } else if (e.detail.type === 'login' && e.detail.token) {
                setToken(e.detail.token);
            }
        };

        window.addEventListener('storage', handleStorageChange);
        window.addEventListener('auth-state-change', handleAuthStateChange as EventListener);

        return () => {
            unsubscribe();
            window.removeEventListener('storage', handleStorageChange);
            window.removeEventListener('auth-state-change', handleAuthStateChange as EventListener);
        };
    }, []);

    return (
        <AuthContext.Provider value={{ token, isAuthenticated, isLoading, login, logout }}>
            {children}
            <AuthPromptDialog open={authPromptOpen} onGoToLogin={handleGoToLogin} />
        </AuthContext.Provider>
    );
};
