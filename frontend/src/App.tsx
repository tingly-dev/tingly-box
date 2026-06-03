import { Events } from '@/bindings';
import { fontMono } from '@/theme/fonts';
import { ContentCopy, Error as ErrorIcon, GitHub, AppRegistration as NPM, Refresh, UpgradeOutlined } from '@/components/icons';
import { Box, Button, CircularProgress, Dialog, DialogActions, DialogContent, DialogTitle, Divider, IconButton, Paper, Stack, Typography } from '@mui/material';
import CssBaseline from '@mui/material/CssBaseline';
import { ThemeProvider } from '@mui/material/styles';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { BrowserRouter, Navigate, Route, Routes, useNavigate } from 'react-router-dom';
import ProtectedRoute from './components/ProtectedRoute';
import { AuthProvider } from './contexts/AuthContext';
import { FeatureFlagsProvider } from './contexts/FeatureFlagsContext';
import { HealthProvider, useHealth } from './contexts/HealthContext';
import { NotificationProvider } from './contexts/NotificationContext';
import { ThemeModeProvider, useThemeMode } from './contexts/ThemeContext';
import { useVersion, VersionProvider } from './contexts/VersionContext';
import { ProfileProvider } from './contexts/ProfileContext';
import Layout from './layout/Layout';
import createAppTheme from './theme';

import Login from './pages/Login';
import Guiding from './pages/Guiding';
import Onboarding from './pages/Onboarding';
import { api } from './services/api';
import APITokensPage from './pages/APITokensPage';
import VirtualModelsPage from './pages/VirtualModelsPage';
import UseOpenAIPage from './pages/scenario/UseOpenAIPage';
import UseAnthropicPage from './pages/scenario/UseAnthropicPage';
import UseCodexPage from './pages/scenario/UseCodexPage';
import UseClaudeCodePage from './pages/scenario/UseClaudeCodePage';
import ClaudeCodeProfilePage from './pages/scenario/ClaudeCodeProfilePage';
import UseClaudeDesktopPage from './pages/scenario/UseClaudeDesktopPage';
import UseAgentPage from './pages/scenario/UseAgentPage';
import AgentOverviewPage from './pages/scenario/AgentOverviewPage';
import UseOpenCodePage from './pages/scenario/UseOpenCodePage';
import UseXcodePage from './pages/scenario/UseXcodePage';
import UseVSCodePage from './pages/scenario/UseVSCodePage';
import UseEmbedPage from './pages/scenario/UseEmbedPage';
import UseImageGenPage from './pages/scenario/UseImageGenPage';
import PlaygroundPage from './pages/scenario/PlaygroundPage';
import CredentialPage from './pages/CredentialPage';
import ProviderListPage from './pages/ProviderListPage';
import System from './pages/system/System.tsx';
import AccessControl from './pages/system/AccessControl.tsx';
import LogsPage from './pages/system/LogsPage';
import ExperimentalPage from './pages/system/ExperimentalPage';
import GuardrailsPage from './pages/GuardrailsPage';
import GuardrailsRulesPage from './pages/guardrails/RulesPage';
import GuardrailsCredentialsPage from './pages/guardrails/CredentialsPage';
import GuardrailsGroupsPage from './pages/guardrails/GroupsPage';
import GuardrailsHistoryPage from './pages/guardrails/HistoryPage';
import DashboardPage from './pages/DashboardPage';
import OverviewPage from './pages/OverviewPage.tsx';
import ModelTestPage from './pages/ModelTestPage';
import UserPage from './pages/prompt/UserPage';
import SkillPage from './pages/prompt/SkillPage';
import CommandPage from './pages/prompt/CommandPage';
import RemoteCoderPage from './pages/remote-coder/RemoteCoderPage';
import RemoteCoderSessionsPage from './pages/remote-coder/RemoteCoderSessionsPage';
import AgentPage from './pages/remote-control/AgentPage';
import TelegramPage from './pages/remote-control/TelegramPage';
import FeishuPage from './pages/remote-control/FeishuPage';
import LarkPage from './pages/remote-control/LarkPage';
import DingTalkPage from './pages/remote-control/DingTalkPage';
import WeixinPage from './pages/remote-control/WeixinPage';
import WeComPage from './pages/remote-control/WeComPage';
import QQPage from './pages/remote-control/QQPage';
import DiscordPage from './pages/remote-control/DiscordPage';
import SlackPage from './pages/remote-control/SlackPage';
import MCPLocalMode from './pages/mcp/MCPLocalMode';
import MCPRegisteredServers from './pages/mcp/MCPRegisteredServers';
import ServerToolPage from './pages/servertool/ServerToolPage';
import {
    ZenClaudeCodePage,
    ZenClaudeCodeProfilePage,
    ZenCodexPage,
    ZenOpenCodePage,
    ZenXcodePage,
    ZenVSCodePage,
    ZenOpenAIPage,
    ZenAnthropicPage,
    ZenAgentPage,
} from './pages/zen';

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
                                        fontFamily: fontMono,
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

// OnboardingGate decides where a freshly-authenticated user lands. Brand-new
// installs (no provider configured) get sent to /onboarding; everyone else
// lands on the agent overview at /agent. We hit /api/v2/providers once on
// mount; while in flight we render nothing to avoid a flash of the default
// agent page.
const OnboardingGate: React.FC = () => {
    const [target, setTarget] = useState<string | null>(null);

    useEffect(() => {
        let cancelled = false;
        (async () => {
            try {
                const result = await api.getProviders();
                if (cancelled) return;
                const providers = Array.isArray(result?.data) ? result.data : [];
                if (providers.length === 0) {
                    setTarget('/onboarding');
                    localStorage.removeItem('layout.activeActivity');
                    sessionStorage.removeItem('layout.activeActivity');
                    return;
                }
            } catch {
                // Swallow the error and fall through to the default agent —
                // failing the gate should never lock the user out of the app.
            }
            // Clear stale activity state and navigate to agent overview
            localStorage.removeItem('layout.activeActivity');
            sessionStorage.removeItem('layout.activeActivity');
            if (!cancelled) setTarget('/agent');
        })();
        return () => { cancelled = true; };
    }, []);

    if (target === null) return null;
    return <Navigate to={target} replace />;
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
                    {/* Default landing: send first-time users (no providers) to onboarding,
                        everyone else to their last-active activity. */}
                    <Route index element={<OnboardingGate />} />
                    {/* Onboarding for new installs */}
                    <Route path="/onboarding" element={<Onboarding />} />
                    {/* Function panel routes */}
                    <Route path="/agent" element={<AgentOverviewPage />} />
                    <Route path="/agent/openai" element={<UseOpenAIPage />} />
                    <Route path="/agent/anthropic" element={<UseAnthropicPage />} />
                    <Route path="/agent/codex" element={<UseCodexPage />} />
                    <Route path="/agent/claude_code" element={<UseClaudeCodePage />} />
                    <Route path="/agent/claude_code/profile/:profileId" element={<ClaudeCodeProfilePage />} />
                    <Route path="/agent/claude_desktop" element={<UseClaudeDesktopPage />} />
                    <Route path="/agent/agent" element={<UseAgentPage />} />
                    <Route path="/agent/opencode" element={<UseOpenCodePage />} />
                    <Route path="/agent/xcode" element={<UseXcodePage />} />
                    <Route path="/agent/vscode" element={<UseVSCodePage />} />
                    <Route path="/agent/embed" element={<UseEmbedPage />} />
                    <Route path="/agent/imagegen" element={<UseImageGenPage />} />
                    <Route path="/agent/playground" element={<PlaygroundPage />} />
                    {/* Credential routes - new unified page */}
                    <Route path="/credentials" element={<CredentialPage />} />
                    {/* Provider List page - must come before :tab wildcard */}
                    <Route path="/credentials/providers" element={<ProviderListPage />} />
                    {/* Virtual Models page - peer of Model Key and Sharing */}
                    <Route path="/credentials/virtual-models" element={<VirtualModelsPage />} />
                    {/* Other routes */}
                    <Route path="/system" element={<System />} />
                    <Route path="/access-control" element={<AccessControl />} />
                    <Route path="/tingly-box-token" element={<APITokensPage />} />
                    <Route path="/system/logs" element={<LogsPage />} />
                    <Route path="/system/experimental" element={<ExperimentalPage />} />
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
                    <Route path="/remote-control" element={<Navigate to="/remote-control/weixin" replace />} />
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
                    {/* MCP Settings */}
                    <Route path="/mcp/sources" element={<MCPRegisteredServers />} />
                    <Route path="/mcp/local-mode" element={<MCPLocalMode />} />
                    <Route path="/mcp" element={<Navigate to="/mcp/sources" replace />} />
                    {/* Tools */}
                    <Route path="/tools/servertool" element={<ServerToolPage />} />
                    {/* Zen Mode Routes - Use zen layout when in zen mode */}
                    <Route path="/zen/claude_code" element={<ZenClaudeCodePage />} />
                    <Route path="/zen/claude_code/profile/:profileId" element={<ZenClaudeCodeProfilePage />} />
                    <Route path="/zen/codex" element={<ZenCodexPage />} />
                    <Route path="/zen/opencode" element={<ZenOpenCodePage />} />
                    <Route path="/zen/xcode" element={<ZenXcodePage />} />
                    <Route path="/zen/vscode" element={<ZenVSCodePage />} />
                    <Route path="/zen/openai" element={<ZenOpenAIPage />} />
                    <Route path="/zen/anthropic" element={<ZenAnthropicPage />} />
                    <Route path="/zen/agent" element={<ZenAgentPage />} />
                    <Route path="/zen" element={<Navigate to="/zen/claude_code" replace />} />
                    {/* Catch-all redirect for unknown routes */}
                    <Route path="*" element={<Navigate to="/agent" replace />} />
                </Route>
            </Routes>
    )
}

// Inner component that uses theme context
function AppWithTheme() {
    const { effectiveMode } = useThemeMode();
    const theme = useMemo(() => createAppTheme(effectiveMode), [effectiveMode]);

    return (
        <ThemeProvider theme={theme}>
            <CssBaseline />
            <NotificationProvider>
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
            </NotificationProvider>
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
