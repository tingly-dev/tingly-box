import React, { useState, useEffect, useCallback } from 'react';
import { Alert, Box, Button, Container, Paper, TextField, Typography } from '@mui/material';
import { useNavigate, useLocation, useParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useAuth } from '../contexts/AuthContext';
import { getControlApiClient } from '../services/openapi';

const Login: React.FC = () => {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const location = useLocation();
    const { token: tokenParam } = useParams<{ token: string }>();
    const [token, setToken] = useState('');
    const [error, setError] = useState('');
    const [loading, setLoading] = useState(false);
    const { login } = useAuth();

    // Get the redirect path from router state or sessionStorage, default to '/'
    // Avoid redirect loops by checking if the target is a login page
    const fromPath = (location.state as any)?.from?.pathname || sessionStorage.getItem('redirectAfterLogin') || '/';
    const from = (fromPath.startsWith('/login') ? '/' : fromPath);

    const handleAutoLogin = useCallback(async (urlToken: string) => {
        setLoading(true);
        setError('');

        try {
            const client = await getControlApiClient();
            const result = await client.GET('/api/v1/auth/validate', {
                headers: {
                    'Authorization': `Bearer ${urlToken}`,
                },
            });

            if (result.response.ok) {
                await login(urlToken);
                sessionStorage.removeItem('redirectAfterLogin');
                // Use window.location.href to trigger a full page reload
                // This ensures all contexts and states are properly initialized
                window.location.href = from;
            } else {
                setError(t('login.errors.invalidToken'));
                setLoading(false);
            }
        } catch (err) {
            setError(t('login.errors.validationFailed'));
            setLoading(false);
        }
    }, [login, navigate, from, t]);

    // Auto-login if token is in URL path
    useEffect(() => {
        if (tokenParam) {
            handleAutoLogin(tokenParam);
        }
    }, [tokenParam, handleAutoLogin]);

    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();
        if (!token.trim()) {
            setError(t('login.errors.enterValidToken'));
            return;
        }

        setLoading(true);
        setError('');

        try {
            // Validate the token by making a test API call to the validate endpoint
            const client = await getControlApiClient();
            const result = await client.GET('/api/v1/auth/validate', {
                headers: {
                    'Authorization': `Bearer ${token}`,
                },
            });

            if (result.response.ok) {
                await login(token);
                // Clear the saved redirect path
                sessionStorage.removeItem('redirectAfterLogin');
                // Use window.location.href to trigger a full page reload
                // This ensures all contexts and states are properly initialized
                window.location.href = from;
            } else {
                setError(t('login.errors.invalidToken'));
            }
        } catch (err) {
            setError(t('login.errors.validationFailed'));
        } finally {
            setLoading(false);
        }
    };

    return (
        <Container component="main" maxWidth="sm">
            <Box
                sx={{
                    marginTop: 8,
                    display: 'flex',
                    flexDirection: 'column',
                    alignItems: 'center',
                }}
            >
                <Paper elevation={3} sx={{ padding: 4, width: '100%' }}>
                    <Typography component="h1" variant="h4" align="center" gutterBottom>
                        {t('login.title')}
                    </Typography>
                    <Typography component="h2" variant="h6" align="center" color="text.secondary" gutterBottom>
                        {t('login.subtitle')}
                    </Typography>

                    <Box component="form" onSubmit={handleSubmit} sx={{ mt: 3 }}>
                        <TextField
                            margin="normal"
                            required
                            fullWidth
                            name="token"
                            label={t('login.tokenLabel')}
                            type="password"
                            id="token"
                            autoComplete="current-token"
                            value={token}
                            onChange={(e) => setToken(e.target.value)}
                            disabled={loading}
                            helperText={t('login.tokenHelper')}
                        />

                        {error && (
                            <Alert severity="error" sx={{ mt: 2 }}>
                                {error}
                            </Alert>
                        )}

                        <Button
                            type="submit"
                            fullWidth
                            variant="contained"
                            sx={{ mt: 3, mb: 2 }}
                            disabled={loading}
                        >
                            {loading ? t('login.validating') : t('login.loginButton')}
                        </Button>
                    </Box>
                </Paper>
            </Box>
        </Container>
    );
};

export default Login;
