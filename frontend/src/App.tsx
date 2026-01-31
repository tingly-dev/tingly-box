import CssBaseline from '@mui/material/CssBaseline';
import { ThemeProvider } from '@mui/material/styles';
import { CircularProgress, Box, Dialog, DialogTitle, DialogContent, DialogActions, Button, Typography, IconButton } from '@mui/material';
import { BrowserRouter, Route, Routes, useNavigate } from 'react-router-dom';
import { lazy, Suspense, useEffect, useRef, useState } from 'react';
import ProtectedRoute from './components/ProtectedRoute';
import { AuthProvider } from './contexts/AuthContext';
import { VersionProvider, useVersion } from './contexts/VersionContext';
import { HealthProvider, useHealth } from './contexts/HealthContext';
import Layout from './layout/Layout';
import theme from './theme';
import { CloudUpload, Refresh, Error as ErrorIcon, AppRegistration as NPM, GitHub } from '@mui/icons-material';
import { useTranslation } from 'react-i18next';
import Logs from "@/pages/Logs.tsx";
import { Events } from '@/bindings';

// Lazy load pages for code splitting
const Login = lazy(() => import('./pages/Login'));
const Dashboard = lazy(() => import('./pages/Dashboard'));
const UseOpenAIPage = lazy(() => import('./pages/UseOpenAIPage'));
const UseAnthropicPage = lazy(() => import('./pages/UseAnthropicPage'));
const UseClaudeCodePage = lazy(() => import('./pages/UseClaudeCodePage'));
const UseOpenCodePage = lazy(() => import('./pages/UseOpenCodePage'));
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
    const { showNotification, updateTrigger, currentVersion, latestVersion } = useVersion();
    const [showDisconnectAlert, setShowDisconnectAlert] = useState(false);
    const [showUpdateAlert, setShowUpdateAlert] = useState(false);
    const disconnectAlertShown = useRef(false);
    const lastUpdateTrigger = useRef(0);

    // Show disconnect alert when health status changes to unhealthy
    useEffect(() => {
        if (!checking && !isHealthy && !disconnectAlertShown.current) {
            setShowDisconnectAlert(true);
            disconnectAlertShown.current = true;
        } else if (isHealthy && showDisconnectAlert) {
            setShowDisconnectAlert(false);
            disconnectAlertShown.current = false;
        }
    }, [isHealthy, checking, showDisconnectAlert]);

    // Show update alert when showNotification changes from false to true
    // OR when updateTrigger changes (manual refresh)
    useEffect(() => {
        // If this is a manual trigger (updateTrigger increased)
        if (updateTrigger > lastUpdateTrigger.current) {
            setShowUpdateAlert(true);
            lastUpdateTrigger.current = updateTrigger;
        } else if (showNotification && lastUpdateTrigger.current === 0) {
            // First time showing notification (on mount)
            setShowUpdateAlert(true);
            lastUpdateTrigger.current = updateTrigger;
        }
    }, [showNotification, updateTrigger, currentVersion, latestVersion]);

    // Listen for test events
    useEffect(() => {
        const handleTestUpdate = () => {
            setShowUpdateAlert(true);
        };
        const handleTestDisconnect = () => {
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
                    <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                        Run the following command to update:
                    </Typography>
                    <Typography
                        variant="body2"
                        component="div"
                        sx={{
                            fontFamily: 'monospace',
                            bgcolor: 'grey.100',
                            p: 1,
                            borderRadius: 1,
                            mb: 2
                        }}
                    >
                        npx tingly-box@latest
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        Or visit the release page for more information.
                    </Typography>
                </DialogContent>
                <DialogActions>
                    <Button
                        variant="contained"
                        onClick={() => window.open('https://www.npmjs.com/package/tingly-box', '_blank')}
                        startIcon={<NPM />}
                    >
                        npm
                    </Button>
                    <Button
                        variant="contained"
                        onClick={() => window.open('https://github.com/tingly-dev/tingly-box/releases', '_blank')}
                        startIcon={<GitHub />}
                    >
                        GitHub
                    </Button>
                    <Button onClick={() => setShowUpdateAlert(false)}>
                        {t('update.later', { defaultValue: 'Later' })}
                    </Button>
                </DialogActions>
            </Dialog>
        </>
    );
};

function AppContent() {
    const navigate = useNavigate();

    // Listen for systray navigation events
    useEffect(() => {
        const off = Events.On('systray-navigate', (event: any) => {
            const path = event.data || event;
            console.log('[Systray] Navigate to:', path);
            navigate(path);
        });

        return () => {
            off?.();
        };
    }, [navigate]);

    return (
        <Suspense fallback={<PageLoader/>}>
            <Routes>
                <Route path="/login" element={<Login/>}/>
                <Route
                    path="/*"
                    element={
                        <ProtectedRoute>
                            <Layout>
                                <Suspense fallback={<PageLoader/>}>
                                    <Routes>
                                        <Route path="/" element={<Dashboard/>}/>
                                        {/* Function panel routes */}
                                        <Route path="/use-openai" element={<UseOpenAIPage/>}/>
                                        <Route path="/use-anthropic" element={<UseAnthropicPage/>}/>
                                        <Route path="/use-claude-code" element={<UseClaudeCodePage/>}/>
                                        <Route path="/use-opencode" element={<UseOpenCodePage/>}/>
                                        {/* Other routes */}
                                        <Route path="/api-keys" element={<ApiKeyPage/>}/>
                                        <Route path="/oauth" element={<OAuthPage/>}/>
                                        <Route path="/system" element={<System/>}/>
                                        <Route path="/logs" element={<Logs/>}/>
                                        <Route path="/dashboard" element={<UsageDashboardPage/>}/>
                                        <Route path="/model-test/:providerUuid" element={<ModelTestPage/>}/>
                                    </Routes>
                                </Suspense>
                            </Layout>
                        </ProtectedRoute>
                    }
                />
            </Routes>
        </Suspense>
    )
}

function App() {
    // Expose test functions to window for debugging
    useEffect(() => {
        (window as any).testShowUpdateDialog = () => {
            const event = new CustomEvent('test-show-update');
            window.dispatchEvent(event);
        };
        (window as any).testShowDisconnectDialog = () => {
            const event = new CustomEvent('test-show-disconnect');
            window.dispatchEvent(event);
        };
    }, []);

    return (
        <ThemeProvider theme={theme}>
            <CssBaseline/>
            <BrowserRouter>
                <HealthProvider>
                    <VersionProvider>
                        <AuthProvider>
                            <AppContent/>
                            <AppDialogs/>
                        </AuthProvider>
                    </VersionProvider>
                </HealthProvider>
            </BrowserRouter>
        </ThemeProvider>
    );
}

export default App;
