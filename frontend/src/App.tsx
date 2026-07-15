import { Events } from '@/bindings';
import { Error as ErrorIcon, Refresh } from '@/components/icons';
import { Box, Button, CircularProgress, Dialog, DialogActions, DialogContent, DialogTitle, IconButton, Paper, Stack, Typography } from '@mui/material';
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
import Onboarding from './pages/Onboarding';
import { api } from './services/api';
import SharingKeysPage from './pages/SharingKeysPage.tsx';
import VirtualModelsPage from './pages/VirtualModelsPage';
import UseOpenAIPage from './pages/scenario/UseOpenAIPage';
import UseAnthropicPage from './pages/scenario/UseAnthropicPage';
import UseCodexPage from './pages/scenario/UseCodexPage';
import UseClaudeCodePage from './pages/scenario/UseClaudeCodePage';
import ClaudeCodeProfilePage from './pages/scenario/ClaudeCodeProfilePage';
import UseClaudeDesktopPage from './pages/scenario/UseClaudeDesktopPage';
import UseAgentPage from './pages/scenario/UseAgentPage';
import UseTeamPage from './pages/scenario/UseTeamPage';
import AgentOverviewPage from './pages/scenario/AgentOverviewPage';
import UseOpenCodePage from './pages/scenario/UseOpenCodePage';
import UseXcodePage from './pages/scenario/UseXcodePage';
import UseVSCodePage from './pages/scenario/UseVSCodePage';
import UseEmbedPage from './pages/scenario/UseEmbedPage';
import UseImageGenPage from './pages/scenario/UseImageGenPage';
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
import ModelTestPage from './pages/ModelTestPage';
import UserPage from './pages/prompt/UserPage';
import SkillPage from './pages/prompt/SkillPage';
import CommandPage from './pages/prompt/CommandPage';
import RemoteCoderPage from './pages/remote-coder/RemoteCoderPage';
import RemoteCoderSessionsPage from './pages/remote-coder/RemoteCoderSessionsPage';
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

// Loading fallback component - kept for potential future use with async data

// Dialogs component that uses the health context
const AppDialogs = () => {
    const { t } = useTranslation();
    const { isHealthy, checking, checkHealth, disconnectDialogOpen, closeDisconnectDialog } = useHealth();

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
                    <Route path="/agent/team" element={<UseTeamPage />} />
                    <Route path="/agent/opencode" element={<UseOpenCodePage />} />
                    <Route path="/agent/xcode" element={<UseXcodePage />} />
                    <Route path="/agent/vscode" element={<UseVSCodePage />} />
                    <Route path="/agent/embed" element={<UseEmbedPage />} />
                    <Route path="/agent/imagegen" element={<UseImageGenPage />} />
                    <Route path="/agent/playground" element={<Navigate to="/agent/imagegen" replace />} />
                    {/* Credential routes - new unified page */}
                    <Route path="/credentials" element={<CredentialPage />} />
                    {/* Provider List page - must come before :tab wildcard */}
                    <Route path="/credentials/providers" element={<ProviderListPage />} />
                    {/* Virtual Models page - peer of Model Key and Sharing */}
                    <Route path="/credentials/virtual-models" element={<VirtualModelsPage />} />
                    {/* Other routes */}
                    <Route path="/system" element={<System />} />
                    <Route path="/access-control" element={<AccessControl />} />
                    <Route path="/tingly-box-token" element={<SharingKeysPage />} />
                    <Route path="/system/logs" element={<LogsPage />} />
                    <Route path="/system/experimental" element={<ExperimentalPage />} />
                    {/* Dashboard routes with time range */}
                    <Route path="/dashboard" element={<Navigate to="/dashboard/7d" replace />} />
                    <Route path="/dashboard/:timeRange" element={<DashboardPage />} />
                    {/* Token Heatmap merged into the Usage Dashboard; keep old
                        /overview links working by redirecting to the dashboard. */}
                    <Route path="/overview" element={<Navigate to="/dashboard/7d" replace />} />
                    <Route path="/overview/:timeRange" element={<Navigate to="/dashboard/7d" replace />} />
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
                    {/* Catch-all redirect for unknown routes (also covers legacy /zen/* links) */}
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
