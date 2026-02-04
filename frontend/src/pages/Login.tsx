import React, { useState } from 'react';
import { Alert, Box, Button, Container, Paper, Snackbar, TextField, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useAuth } from '../contexts/AuthContext';

const Login: React.FC = () => {
    const { t } = useTranslation();
    const [token, setToken] = useState('');
    const [error, setError] = useState('');
    const [loading, setLoading] = useState(false);
    const [showSuccess, setShowSuccess] = useState(false);
    const { login } = useAuth();

    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();
        if (!token.trim()) {
            setError(t('login.errors.enterValidToken'));
            return;
        }

        setLoading(true);
        setError('');

        try {
            // Validate the token by making a test API call
            const response = await fetch('/api/status', {
                headers: {
                    'Authorization': `Bearer ${token}`,
                    'Content-Type': 'application/json',
                },
            });

            if (response.ok) {
                await login(token);
                setShowSuccess(true);
                // Refresh page after successful login to ensure all state and API instances are properly initialized
                setTimeout(() => {
                    window.location.href = '/';
                }, 500);
            } else {
                setError(t('login.errors.invalidToken'));
            }
        } catch (err) {
            setError(t('login.errors.validationFailed'));
        } finally {
            setLoading(false);
        }
    };

    const handleGenerateToken = () => {
        const clientId = prompt('Enter client ID (web):', 'web');
        if (clientId) {
            window.open(`/api/token?client_id=${encodeURIComponent(clientId)}`, '_blank');
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

                        <Button
                            fullWidth
                            variant="outlined"
                            onClick={handleGenerateToken}
                            sx={{ mb: 2 }}
                        >
                            {t('login.generateTokenButton')}
                        </Button>
                    </Box>
                </Paper>
            </Box>

            <Snackbar
                open={showSuccess}
                autoHideDuration={2000}
                onClose={() => setShowSuccess(false)}
            >
                <Alert onClose={() => setShowSuccess(false)} severity="success">
                    {t('login.success.loginSuccess')}
                </Alert>
            </Snackbar>
        </Container>
    );
};

export default Login;