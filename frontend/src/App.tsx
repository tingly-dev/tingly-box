import CssBaseline from '@mui/material/CssBaseline';
import { ThemeProvider } from '@mui/material/styles';
import { CircularProgress, Box, Dialog, DialogTitle, DialogContent, DialogActions, Button, Typography, IconButton, Paper, Stack, Divider } from '@mui/material';
import { BrowserRouter, Route, Routes, useNavigate, Navigate } from 'react-router-dom';
import { lazy, Suspense, useEffect } from 'react';
import ProtectedRoute from './components/ProtectedRoute';
import { AuthProvider } from './contexts/AuthContext';
import { VersionProvider, useVersion } from './contexts/VersionContext';
import { HealthProvider, useHealth } from './contexts/HealthContext';
import { FeatureFlagsProvider } from './contexts/FeatureFlagsContext';
import Layout from './layout/Layout';
import theme from './theme';
import { CloudUpload, Refresh, Error as ErrorIcon, AppRegistration as NPM, GitHub, ContentCopy, UpgradeOutlined, CheckCircle } from '@mui/icons-material';
import { useTranslation } from 'react-i18next';
import { Events } from '@/bindings';

// Lazy load pages for code splitting
const Login = lazy(() => import('./pages/Login'));
const Guiding = lazy(() => import('./pages/./Guiding'));
const UseOpenAIPage = lazy(() => import('./pages/UseOpenAIPage'));
const UseAnthropicPage = lazy(() => import('./pages/UseAnthropicPage'));
const UseClaudeCodePage = lazy(() => import('./pages/UseClaudeCodePage'));
const UseOpenCodePage = lazy(() => import('./pages/UseOpenCodePage'));
const CredentialPage = lazy(() => import('./pages/CredentialPage'));
const System = lazy(() => import('./pages/System'));
const DashboardPage = lazy(() => import('./pages/./DashboardPage'));
const ModelTestPage = lazy(() => import('./pages/ModelTestPage'));

// Prompt pages
const UserPage = lazy(() => import('./pages/prompt/UserPage'));
const SkillPage = lazy(() => import('./pages/prompt/SkillPage'));
const CommandPage = lazy(() => import('./pages/prompt/CommandPage'));

// Remote CC page
const RemoteCCPage = lazy(() => import('./pages/remote-cc/RemoteCCPage'));
const RemoteCCSessionsPage = lazy(() => import('./pages/remote-cc/RemoteCCSessionsPage'));

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
    const { isHealthy, checking, checkHealth, disconnectDialogOpen, closeDisconnectDialog } = useHealth();
    const { openUpdateDialog, currentVersion, latestVersion, releaseURL, closeUpdateDialog } = useVersion();

    return (
        <>
            {/* Disconnect Alert Dialog - now manually controlled */}
            <Dialog
                open={disconnectDialogOpen}
                onClose={closeDisconnectDialog}
                maxWidth="sm"
                fullWidth
                PaperProps={{
                    sx: {
                        borderRadius: 2,
                        boxShadow: '0 8px 32px rgba(0,0,0,0.1)',
                    }
                }}
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
                    <Button onClick={closeDisconnectDialog}>
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
                open={openUpdateDialog}
                onClose={closeUpdateDialog}
                maxWidth="sm"
                fullWidth
                PaperProps={{
                    sx: {
                        borderRadius: 2,
                        overflow: 'hidden',
                        border: '1px solid',
                        borderColor: 'divider',
                    }
                }}
            >
                {/* Header with gradient background - using info color for update notification */}
                <Box
                    sx={{
                        background: 'linear-gradient(135deg, #0891b2 0%, #0e7490 100%)',
                        px: 3,
                        py: 2.5,
                        textAlign: 'center',
                    }}
                >
                    <Box
                        sx={{
                            width: 56,
                            height: 56,
                            borderRadius: '50%',
                            bgcolor: 'rgba(255, 255, 255, 0.2)',
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            mx: 'auto',
                            mb: 1.5,
                        }}
                    >
                        <UpgradeOutlined sx={{ fontSize: 32, color: 'white' }} />
                    </Box>
                    <Typography variant="h5" sx={{ color: 'white', fontWeight: 600, mb: 0.5 }}>
                        {t('update.newVersionAvailable', { defaultValue: 'New Version Available' })}
                    </Typography>
                    <Typography variant="body2" sx={{ color: 'rgba(255, 255, 255, 0.9)' }}>
                        {t('update.versionAvailable', {
                            latest: latestVersion,
                            current: currentVersion,
                            defaultValue: 'Version {{latest}} is available (you have {{current}})'
                        })}
                    </Typography>
                </Box>

                <DialogContent sx={{ p: 0 }}>
                    <Stack spacing={0} divider={<Divider />}>
                        {/* Command Section */}
                        <Box sx={{ p: 2.5 }}>
                            <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1.5, color: 'text.primary' }}>
                                Quick Update with npx
                            </Typography>
                            <Paper
                                variant="outlined"
                                sx={{
                                    p: 2,
                                    bgcolor: 'background.paper',
                                    border: '1px solid',
                                    borderColor: 'divider',
                                    position: 'relative',
                                }}
                            >
                                <Typography
                                    variant="body2"
                                    sx={{
                                        fontFamily: '"Fira Code", "Monaco", "Consolas", monospace',
                                        color: 'text.primary',
                                        fontSize: '0.875rem',
                                        pr: 4,
                                        wordBreak: 'break-all',
                                    }}
                                >
                                    $ npx tingly-box@latest
                                </Typography>
                                <IconButton
                                    size="small"
                                    onClick={() => {
                                        navigator.clipboard.writeText('npx tingly-box@latest');
                                    }}
                                    sx={{
                                        position: 'absolute',
                                        right: 8,
                                        top: '50%',
                                        transform: 'translateY(-50%)',
                                        color: 'text.secondary',
                                        '&:hover': {
                                            color: 'primary.main',
                                            bgcolor: 'action.hover',
                                        },
                                    }}
                                    title="Copy to clipboard"
                                >
                                    <ContentCopy sx={{ fontSize: 18 }} />
                                </IconButton>
                            </Paper>
                        </Box>

                        {/* Links Section */}
                        <Box sx={{ p: 2.5 }}>
                            <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1.5, color: 'text.primary' }}>
                                Or visit release page
                            </Typography>
                            <Stack direction="row" spacing={1.5}>
                                <Button
                                    variant="outlined"
                                    onClick={() => window.open('https://www.npmjs.com/package/tingly-box', '_blank')}
                                    startIcon={<NPM />}
                                    sx={{ flex: 1 }}
                                >
                                    npm
                                </Button>
                                <Button
                                    variant="outlined"
                                    onClick={() => window.open(releaseURL || 'https://github.com/tingly-dev/tingly-box/releases', '_blank')}
                                    startIcon={<GitHub />}
                                    sx={{ flex: 1 }}
                                >
                                    GitHub
                                </Button>
                            </Stack>
                        </Box>
                    </Stack>
                </DialogContent>

                <DialogActions sx={{ px: 3, py: 2, bgcolor: 'action.hover' }}>
                    <Button
                        onClick={closeUpdateDialog}
                        sx={{
                            color: 'text.secondary',
                            '&:hover': {
                                bgcolor: 'action.selected',
                            },
                        }}
                    >
                        {t('update.later', { defaultValue: 'Remind Me Later' })}
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
                                        <Route path="/" element={<DashboardPage/>}/>
                                        {/* Function panel routes */}
                                        <Route path="/use-openai" element={<UseOpenAIPage/>}/>
                                        <Route path="/use-anthropic" element={<UseAnthropicPage/>}/>
                                        <Route path="/use-claude-code" element={<UseClaudeCodePage/>}/>
                                        <Route path="/use-opencode" element={<UseOpenCodePage/>}/>
                                        {/* Credential routes - new unified page */}
                                        <Route path="/credentials" element={<CredentialPage/>}/>
                                        <Route path="/credentials/:tab" element={<CredentialPage/>}/>
                                        {/* Legacy redirects for backward compatibility */}
                                        <Route path="/api-keys" element={<Navigate to="/credentials" replace/>}/>
                                        <Route path="/oauth" element={<Navigate to="/credentials" replace/>}/>
                                        {/* Other routes */}
                                        <Route path="/system" element={<System/>}/>
                                        <Route path="/logs" element={<Navigate to="/system" replace/>}/>
                                        <Route path="/dashboard" element={<DashboardPage/>}/>
                                        <Route path="/model-test/:providerUuid" element={<ModelTestPage/>}/>
                                        {/* Prompt routes */}
                                        <Route path="/prompt/user" element={<UserPage/>}/>
                                        <Route path="/prompt/skill" element={<SkillPage/>}/>
                                        <Route path="/prompt/command" element={<CommandPage/>}/>
                                        {/* Remote CC routes */}
                                        <Route path="/remote-cc" element={<Navigate to="/remote-cc/chat" replace/>}/>
                                        <Route path="/remote-cc/chat" element={<RemoteCCPage/>}/>
                                        <Route path="/remote-cc/sessions" element={<RemoteCCSessionsPage/>}/>
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
    return (
        <ThemeProvider theme={theme}>
            <CssBaseline/>
            <BrowserRouter>
                <HealthProvider>
                    <VersionProvider>
                        <AuthProvider>
                            <FeatureFlagsProvider>
                                <AppContent/>
                                <AppDialogs/>
                            </FeatureFlagsProvider>
                        </AuthProvider>
                    </VersionProvider>
                </HealthProvider>
            </BrowserRouter>
        </ThemeProvider>
    );
}

export default App;
