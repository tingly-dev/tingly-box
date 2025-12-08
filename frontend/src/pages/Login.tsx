import React, { useState } from 'react';
import { Alert, Box, Button, Container, Paper, Snackbar, TextField, Typography } from '@mui/material';
import { useNavigate } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';

const Login: React.FC = () => {
    const [token, setToken] = useState('');
    const [error, setError] = useState('');
    const [loading, setLoading] = useState(false);
    const [showSuccess, setShowSuccess] = useState(false);
    const { login } = useAuth();
    const navigate = useNavigate();

    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();
        if (!token.trim()) {
            setError('Please enter a valid token');
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
                login(token);
                setShowSuccess(true);
                setTimeout(() => {
                    navigate('/dashboard');
                }, 1000);
            } else {
                setError('Invalid token. Please check your token and try again.');
            }
        } catch (err) {
            setError('Failed to validate token. Please check your connection and try again.');
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
                        Tingly Box
                    </Typography>
                    <Typography component="h2" variant="h6" align="center" color="text.secondary" gutterBottom>
                        Authentication Required
                    </Typography>

                    <Box component="form" onSubmit={handleSubmit} sx={{ mt: 3 }}>
                        <TextField
                            margin="normal"
                            required
                            fullWidth
                            name="token"
                            label="Authentication Token"
                            type="password"
                            id="token"
                            autoComplete="current-token"
                            value={token}
                            onChange={(e) => setToken(e.target.value)}
                            disabled={loading}
                            helperText="Enter your user authentication token for UI and management access"
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
                            {loading ? 'Validating...' : 'Login'}
                        </Button>

                        <Button
                            fullWidth
                            variant="outlined"
                            onClick={handleGenerateToken}
                            sx={{ mb: 2 }}
                        >
                            Generate New Token
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
                    Login successful! Redirecting...
                </Alert>
            </Snackbar>
        </Container>
    );
};

export default Login;