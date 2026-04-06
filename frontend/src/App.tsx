import { Events } from '@/bindings';
import { ContentCopy, Error as ErrorIcon, GitHub, AppRegistration as NPM, Refresh, UpgradeOutlined } from '@mui/icons-material';
import { Box, Button, CircularProgress, Dialog, DialogActions, DialogContent, DialogTitle, Divider, IconButton, Paper, Stack, Typography } from '@mui/material';
import CssBaseline from '@mui/material/CssBaseline';
import { ThemeProvider } from '@mui/material/styles';
import { useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { BrowserRouter, Navigate, Route, Routes, useNavigate } from 'react-router-dom';
import ProtectedRoute from './components/ProtectedRoute';
import { SunlitBackground } from './components/SunlitBackground';
import { AuthProvider } from './contexts/AuthContext';
import { FeatureFlagsProvider } from './contexts/FeatureFlagsContext';
import { HealthProvider, useHealth } from './contexts/HealthContext';
import { ThemeModeProvider, useThemeMode } from './contexts/ThemeContext';
import { useVersion, VersionProvider } from './contexts/VersionContext';
import { ProfileProvider } from './contexts/ProfileContext';
import Layout from './layout/Layout';
import createAppTheme from './theme';

import Login from './pages/Login';
import Guiding from './pages/Guiding';
import UseOpenAIPage from './pages/scenario/UseOpenAIPage';
import UseAnthropicPage from './pages/scenario/UseAnthropicPage';
import UseCodexPage from './pages/scenario/UseCodexPage';
import UseClaudeCodePage from './pages/scenario/UseClaudeCodePage';
import ClaudeCodeProfilePage from './pages/scenario/ClaudeCodeProfilePage';
import UseAgentPage from './pages/scenario/UseAgentPage';
import UseOpenCodePage from './pages/scenario/UseOpenCodePage';
import UseXcodePage from './pages/scenario/UseXcodePage';
import UseVSCodePage from './pages/scenario/UseVSCodePage';
import CredentialPage from './pages/CredentialPage';
import System from './pages/System';
import AccessControl from './pages/AccessControl';
import LogsPage from './pages/system/LogsPage';
import GuardrailsPage from './pages/GuardrailsPage';
import GuardrailsRulesPage from './pages/guardrails/RulesPage';
import GuardrailsCredentialsPage from './pages/guardrails/CredentialsPage';
import GuardrailsGroupsPage from './pages/guardrails/GroupsPage';
import GuardrailsHistoryPage from './pages/guardrails/HistoryPage';
import DashboardPage from './pages/DashboardPage';
import OverviewPage from './pages/overview/OverviewPage';
import ModelTestPage from './pages/ModelTestPage';
import UserPage from './pages/prompt/UserPage';
import SkillPage from './pages/prompt/SkillPage';
import CommandPage from './pages/prompt/CommandPage';
import RemoteCoderPage from './pages/remote-coder/RemoteCoderPage';
import RemoteCoderSessionsPage from './pages/remote-coder/RemoteCoderSessionsPage';
import AgentPage from './pages/remote-control/AgentPage';
import RemoteControlOverviewPage from './pages/remote-control/OverviewPage';
import TelegramPage from './pages/remote-control/TelegramPage';
import FeishuPage from './pages/remote-control/FeishuPage';
import LarkPage from './pages/remote-control/LarkPage';
import DingTalkPage from './pages/remote-control/DingTalkPage';
import WeixinPage from './pages/remote-control/WeixinPage';
import WeComPage from './pages/remote-control/WeComPage';
import QQPage from './pages/remote-control/QQPage';
import DiscordPage from './pages/remote-control/DiscordPage';
import SlackPage from './pages/remote-control/SlackPage';

// Loading fallback component - kept for potential future use with async data

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
            <Routes>
                <Route path="/login" element={<Login />} />
                <Route path="/login/:token" element={<Login />} />
                {/* Protected routes with Layout */}
                <Route
                    element={
                        <ProtectedRoute>
                            <Layout />
                        </ProtectedRoute>
                    }
                >
                    {/* Default redirect */}
                    <Route index element={<Navigate to="/dashboard/7d" replace />} />
                    {/* Function panel routes */}
                    <Route path="/use-openai" element={<UseOpenAIPage />} />
                    <Route path="/use-anthropic" element={<UseAnthropicPage />} />
                    <Route path="/use-codex" element={<UseCodexPage />} />
                    <Route path="/use-claude-code" element={<UseClaudeCodePage />} />
                    <Route path="/use-claude-code/profile/:profileId" element={<ClaudeCodeProfilePage />} />
                    <Route path="/use-agent" element={<UseAgentPage />} />
                    <Route path="/use-opencode" element={<UseOpenCodePage />} />
                    <Route path="/use-xcode" element={<UseXcodePage />} />
                    <Route path="/use-vscode" element={<UseVSCodePage />} />
                    {/* Credential routes - new unified page */}
                    <Route path="/credentials" element={<CredentialPage />} />
                    <Route path="/credentials/:tab" element={<CredentialPage />} />
                    {/* Legacy redirects for backward compatibility */}
                    <Route path="/api-keys" element={<Navigate to="/credentials" replace />} />
                    <Route path="/oauth" element={<Navigate to="/credentials" replace />} />
                    {/* Other routes */}
                    <Route path="/system" element={<System />} />
                    <Route path="/access-control" element={<AccessControl />} />
                    <Route path="/system/logs" element={<LogsPage />} />
                    {/* Legacy redirects for backward compatibility */}
                    <Route path="/system/http-logs" element={<Navigate to="/system/logs" replace />} />
                    <Route path="/system/system-logs" element={<Navigate to="/system/logs" replace />} />
                    <Route path="/logs" element={<Navigate to="/system/logs" replace />} />
                    {/* Dashboard routes with time range */}
                    <Route path="/dashboard" element={<Navigate to="/dashboard/7d" replace />} />
                    <Route path="/dashboard/:timeRange" element={<DashboardPage />} />
                    {/* Overview / Token Heatmap routes */}
                    <Route path="/overview" element={<Navigate to="/overview/90d" replace />} />
                    <Route path="/overview/:timeRange" element={<OverviewPage />} />
                    <Route path="/model-test/:providerUuid" element={<ModelTestPage />} />
                    {/* Prompt routes */}
                    <Route path="/prompt/user" element={<UserPage />} />
                    <Route path="/prompt/skill" element={<SkillPage />} />
                    <Route path="/prompt/command" element={<CommandPage />} />
                    {/* Remote Control routes */}
                    <Route path="/remote-coder" element={<Navigate to="/remote-coder/chat" replace />} />
                    <Route path="/remote-coder/chat" element={<RemoteCoderPage />} />
                    <Route path="/remote-coder/sessions" element={<RemoteCoderSessionsPage />} />
                    {/* Remote Control routes */}
                    <Route path="/remote-control" element={<RemoteControlOverviewPage />} />
                    <Route path="/remote-control/agent" element={<AgentPage />} />
                    {/* Platform-specific bot pages */}
                    <Route path="/remote-control/telegram" element={<TelegramPage />} />
                    <Route path="/remote-control/feishu" element={<FeishuPage />} />
                    <Route path="/remote-control/lark" element={<LarkPage />} />
                    <Route path="/remote-control/dingtalk" element={<DingTalkPage />} />
                    <Route path="/remote-control/weixin" element={<WeixinPage />} />
                    <Route path="/remote-control/wecom" element={<WeComPage />} />
                    <Route path="/remote-control/qq" element={<QQPage />} />
                    <Route path="/remote-control/discord" element={<DiscordPage />} />
                    <Route path="/remote-control/slack" element={<SlackPage />} />
                    {/* Guardrails */}
                    <Route path="/guardrails" element={<GuardrailsPage />} />
                    <Route path="/guardrails/groups" element={<GuardrailsGroupsPage />} />
                    <Route path="/guardrails/rules" element={<GuardrailsRulesPage />} />
                    <Route path="/guardrails/credentials" element={<GuardrailsCredentialsPage />} />
                    <Route path="/guardrails/history" element={<GuardrailsHistoryPage />} />
                    {/* Catch-all redirect for unknown routes */}
                    <Route path="*" element={<Navigate to="/dashboard/7d" replace />} />
                </Route>
            </Routes>
    )
}

// Inner component that uses theme context
function AppWithTheme() {
    const { mode } = useThemeMode();
    const theme = useMemo(() => createAppTheme(mode), [mode]);

    return (
        <ThemeProvider theme={theme}>
            <CssBaseline />
            {/* Sunlit background effect */}
            {mode === 'sunlit' && <SunlitBackground />}
            <BrowserRouter>
                <HealthProvider>
                    <VersionProvider>
                        <AuthProvider>
                            <FeatureFlagsProvider>
                                <ProfileProvider>
                                    <AppContent />
                                    <AppDialogs />
                                </ProfileProvider>
                            </FeatureFlagsProvider>
                        </AuthProvider>
                    </VersionProvider>
                </HealthProvider>
            </BrowserRouter>
        </ThemeProvider>
    );
}

function App() {
    return (
        <ThemeModeProvider>
            <AppWithTheme />
        </ThemeModeProvider>
    );
}

export default App;
