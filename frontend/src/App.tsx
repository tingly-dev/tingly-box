import CssBaseline from '@mui/material/CssBaseline';
import { ThemeProvider } from '@mui/material/styles';
import { CircularProgress, Box, Dialog, DialogTitle, DialogContent, DialogActions, Button, Typography, IconButton } from '@mui/material';
import { BrowserRouter, Route, Routes } from 'react-router-dom';
import { lazy, Suspense, useEffect, useRef, useState } from 'react';
import ProtectedRoute from './components/ProtectedRoute';
import { AuthProvider } from './contexts/AuthContext';
import { VersionProvider, useVersion } from './contexts/VersionContext';
import { HealthProvider, useHealth } from './contexts/HealthContext';
import { UpdateNotification } from './components/UpdateNotification';
import Layout from './layout/Layout';
import theme from './theme';
import { CloudUpload, Refresh, Error as ErrorIcon, Close } from '@mui/icons-material';
import { useTranslation } from 'react-i18next';

// Lazy load pages for code splitting
const Login = lazy(() => import('./pages/Login'));
const Dashboard = lazy(() => import('./pages/Dashboard'));
const UseOpenAIPage = lazy(() => import('./pages/UseOpenAIPage'));
const UseAnthropicPage = lazy(() => import('./pages/UseAnthropicPage'));
const UseClaudeCodePage = lazy(() => import('./pages/UseClaudeCodePage'));
const ApiKeyPage = lazy(() => import('./pages/ApiKeyPage'));
const OAuthPage = lazy(() => import('./pages/OAuthPage'));
const System = lazy(() => import('./pages/System'));
const UsageDashboardPage = lazy(() => import('./pages/UsageDashboardPage'));
const ModelTestPage = lazy(() => import('./pages/ModelTestPage'));

// Loading fallback component
const PageLoader = () => (
    <Box
        sx={{
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
            height: '100vh',
        }}
    >
        <CircularProgress />
    </Box>
);

// Dialogs component that uses the health and version contexts
const AppDialogs = () => {
    const { t } = useTranslation();
    const { isHealthy, checking, checkHealth } = useHealth();
    const { showNotification, updateTrigger, checking: checkingVersion, checkForUpdates, currentVersion, latestVersion } = useVersion();
    const [showDisconnectAlert, setShowDisconnectAlert] = useState(false);
    const [showUpdateAlert, setShowUpdateAlert] = useState(false);
    const disconnectAlertShown = useRef(false);
    const lastUpdateTrigger = useRef(0);

    // Show disconnect alert when health status changes to unhealthy
    useEffect(() => {
        console.log('[AppDialogs] Health status changed:', { isHealthy, checking, disconnectAlertShown: disconnectAlertShown.current });
        if (!checking && !isHealthy && !disconnectAlertShown.current) {
            console.log('[AppDialogs] Showing disconnect alert');
            setShowDisconnectAlert(true);
            disconnectAlertShown.current = true;
        } else if (isHealthy) {
            disconnectAlertShown.current = false;
        }
    }, [isHealthy, checking]);

    // Show update alert when showNotification changes from false to true
    // OR when updateTrigger changes (manual refresh)
    useEffect(() => {
        console.log('[AppDialogs] showNotification/updateTrigger changed:', {
            showNotification,
            updateTrigger,
            currentVersion,
            latestVersion,
            lastUpdateTrigger: lastUpdateTrigger.current
        });

        // If this is a manual trigger (updateTrigger increased)
        if (updateTrigger > lastUpdateTrigger.current) {
            console.log('[AppDialogs] Manual update trigger detected, showing update alert');
            setShowUpdateAlert(true);
            lastUpdateTrigger.current = updateTrigger;
        } else if (showNotification && lastUpdateTrigger.current === 0) {
            // First time showing notification (on mount)
            console.log('[AppDialogs] Showing update alert (initial notification)');
            setShowUpdateAlert(true);
            lastUpdateTrigger.current = updateTrigger;
        }
    }, [showNotification, updateTrigger]);

    // Listen for test events
    useEffect(() => {
        const handleTestUpdate = () => {
            console.log('[AppDialogs] Test: Showing update dialog');
            setShowUpdateAlert(true);
        };
        const handleTestDisconnect = () => {
            console.log('[AppDialogs] Test: Showing disconnect dialog');
            setShowDisconnectAlert(true);
        };

        window.addEventListener('test-show-update', handleTestUpdate);
        window.addEventListener('test-show-disconnect', handleTestDisconnect);

        // Also add keyboard shortcuts for testing
        const handleKeyDown = (e: KeyboardEvent) => {
            if (e.ctrlKey && e.shiftKey && e.key === 'U') {
                e.preventDefault();
                handleTestUpdate();
            }
            if (e.ctrlKey && e.shiftKey && e.key === 'D') {
                e.preventDefault();
                handleTestDisconnect();
            }
        };
        window.addEventListener('keydown', handleKeyDown);

        return () => {
            window.removeEventListener('test-show-update', handleTestUpdate);
            window.removeEventListener('test-show-disconnect', handleTestDisconnect);
            window.removeEventListener('keydown', handleKeyDown);
        };
    }, []);

    console.log('[AppDialogs] Render:', { showDisconnectAlert, showUpdateAlert, isHealthy, showNotification });

    return (
        <>
            {/* Disconnect Alert Dialog */}
            <Dialog
                open={showDisconnectAlert}
                onClose={() => setShowDisconnectAlert(false)}
                maxWidth="sm"
                fullWidth
            >
                <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <ErrorIcon color="error" />
                    {t('health.disconnectTitle', { defaultValue: 'Connection Lost' })}
                </DialogTitle>
                <DialogContent>
                    <Typography variant="body1">
                        {t('health.disconnectMessage', { defaultValue: 'Connection to server lost. Please check if the server is running.' })}
                    </Typography>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setShowDisconnectAlert(false)}>
                        {t('common.close', { defaultValue: 'Close' })}
                    </Button>
                    <Button
                        variant="contained"
                        onClick={checkHealth}
                        disabled={checking}
                        startIcon={checking ? <CircularProgress size={16} /> : <Refresh />}
                    >
                        {t('health.retry', { defaultValue: 'Retry' })}
                    </Button>
                </DialogActions>
            </Dialog>

            {/* Update Available Dialog */}
            <Dialog
                open={showUpdateAlert}
                onClose={() => setShowUpdateAlert(false)}
                maxWidth="sm"
                fullWidth
            >
                <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <CloudUpload color="info" />
                    {t('update.newVersionAvailable')}
                </DialogTitle>
                <DialogContent>
                    <Typography variant="body1" sx={{ mb: 1 }}>
                        {t('update.versionAvailable', { latest: latestVersion, current: currentVersion })}
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        {t('update.message', { defaultValue: 'A new version is available on GitHub. Would you like to download it now?' })}
                    </Typography>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setShowUpdateAlert(false)}>
                        {t('update.later', { defaultValue: 'Later' })}
                    </Button>
                    <Button
                        variant="contained"
                        onClick={() => window.open('https://github.com/tingly-dev/tingly-box/releases', '_blank')}
                        startIcon={<CloudUpload />}
                    >
                        {t('update.download')}
                    </Button>
                </DialogActions>
            </Dialog>
        </>
    );
};

function App() {
    // Expose test functions to window for debugging
    useEffect(() => {
        (window as any).testShowUpdateDialog = () => {
            console.log('[Test] Triggering update dialog manually');
            const event = new CustomEvent('test-show-update');
            window.dispatchEvent(event);
        };
        (window as any).testShowDisconnectDialog = () => {
            console.log('[Test] Triggering disconnect dialog manually');
            const event = new CustomEvent('test-show-disconnect');
            window.dispatchEvent(event);
        };
        console.log('[App] Test functions available: window.testShowUpdateDialog(), window.testShowDisconnectDialog()');
    }, []);

    return (
        <ThemeProvider theme={theme}>
            <CssBaseline />
            <BrowserRouter>
                <HealthProvider>
                    <VersionProvider>
                        <AuthProvider>
                            <Suspense fallback={<PageLoader />}>
                                <UpdateNotification />
                                <Routes>
                                    <Route path="/login" element={<Login />} />
                                    <Route
                                        path="/*"
                                        element={
                                            <ProtectedRoute>
                                                <Layout>
                                                    <Suspense fallback={<PageLoader />}>
                                                        <Routes>
                                                            <Route path="/" element={<Dashboard />} />
                                                            {/* Function panel routes */}
                                                            <Route path="/use-openai" element={<UseOpenAIPage />} />
                                                            <Route path="/use-anthropic" element={<UseAnthropicPage />} />
                                                            <Route path="/use-claude-code" element={<UseClaudeCodePage />} />
                                                            {/* Other routes */}
                                                            <Route path="/api-keys" element={<ApiKeyPage />} />
                                                            <Route path="/oauth" element={<OAuthPage />} />
                                                            <Route path="/system" element={<System />} />
                                                            <Route path="/dashboard" element={<UsageDashboardPage />} />
                                                            <Route path="/model-test/:providerUuid" element={<ModelTestPage />} />
                                                        </Routes>
                                                    </Suspense>
                                                </Layout>
                                            </ProtectedRoute>
                                        }
                                    />
                                </Routes>
                            </Suspense>
                            <AppDialogs />
                        </AuthProvider>
                    </VersionProvider>
                </HealthProvider>
            </BrowserRouter>
        </ThemeProvider>
    );
}

export default App;
