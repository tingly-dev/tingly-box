import { Events } from '@/bindings';
import { Error as ErrorIcon, Refresh } from '@/components/icons';
import { Box, Button, CircularProgress, Dialog, DialogActions, DialogContent, DialogTitle, IconButton, Paper, Stack, Typography } from '@mui/material';
import CssBaseline from '@mui/material/CssBaseline';
import { ThemeProvider } from '@mui/material/styles';
import { lazy, Suspense, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { resolveLanguage } from '@/i18n';
import { BrowserRouter, Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom';
import ProtectedRoute from './components/ProtectedRoute';
import ExperimentalFeatureGate from './components/ExperimentalFeatureGate';
import { AuthProvider } from './contexts/AuthContext';
import { FeatureFlagsProvider } from './contexts/FeatureFlagsContext';
import { HealthProvider, useHealth } from './contexts/HealthContext';
import { NotificationProvider } from './contexts/NotificationContext';
import { ThemeModeProvider, useThemeMode } from './contexts/ThemeContext';
import { useVersion, VersionProvider } from './contexts/VersionContext';
import { ProfileProvider } from './contexts/ProfileContext';
import { TeamProvider } from './contexts/TeamContext';
import Layout from './layout/Layout';
import createAppTheme from './theme';

// Login is the only pre-auth screen — it's reachable before ProtectedRoute
// can even evaluate, so it stays eager. HelpPage (the onboarding front door
// — OnboardingGate decides at runtime whether a fresh install lands there)
// is a normal post-auth route, so it's lazy-loaded with everything else
// below.
import Login from './pages/Login';
import { api } from './services/api';

// Every route below this point is reached only after auth + navigation, so it
// is lazy-loaded: each becomes its own chunk that downloads on first visit
// instead of being bundled into the initial page load.
const HelpPage = lazy(() => import('./pages/HelpPage'));
const SharingKeysPage = lazy(() => import('./pages/SharingKeysPage.tsx'));
const VirtualModelsPage = lazy(() => import('./pages/VirtualModelsPage'));
const UseOpenAIPage = lazy(() => import('./pages/scenario/UseOpenAIPage'));
const UseAnthropicPage = lazy(() => import('./pages/scenario/UseAnthropicPage'));
const UseCodexPage = lazy(() => import('./pages/scenario/UseCodexPage'));
const UseClaudeCodePage = lazy(() => import('./pages/scenario/UseClaudeCodePage'));
const ClaudeCodeProfilePage = lazy(() => import('./pages/scenario/ClaudeCodeProfilePage'));
const UseClaudeDesktopPage = lazy(() => import('./pages/scenario/UseClaudeDesktopPage'));
const UseCustomPage = lazy(() => import('./pages/scenario/UseCustomPage'));
const UseTeamPage = lazy(() => import('./pages/scenario/UseTeamPage'));
const AgentOverviewPage = lazy(() => import('./pages/scenario/AgentOverviewPage'));
const UseOpenCodePage = lazy(() => import('./pages/scenario/UseOpenCodePage'));
const UsePiPage = lazy(() => import('./pages/scenario/UsePiPage'));
const UseDshPage = lazy(() => import('./pages/scenario/UseDshPage'));
const UseXcodePage = lazy(() => import('./pages/scenario/UseXcodePage'));
const UseVSCodePage = lazy(() => import('./pages/scenario/UseVSCodePage'));
const UseCursorPage = lazy(() => import('./pages/scenario/UseCursorPage'));
const UseEmbedPage = lazy(() => import('./pages/scenario/UseEmbedPage'));
const UseImageGenPage = lazy(() => import('./pages/scenario/UseImageGenPage'));
const CredentialPage = lazy(() => import('./pages/CredentialPage'));
const System = lazy(() => import('./pages/system/System.tsx'));
const AccessControl = lazy(() => import('./pages/system/AccessControl.tsx'));
const LogsPage = lazy(() => import('./pages/system/LogsPage'));
const ExperimentalPage = lazy(() => import('./pages/system/ExperimentalPage'));
const DevelopPage = lazy(() => import('./pages/system/DevelopPage'));
const GuardrailsPage = lazy(() => import('./pages/GuardrailsPage'));
const GuardrailsRulesPage = lazy(() => import('./pages/guardrails/RulesPage'));
const GuardrailsCredentialsPage = lazy(() => import('./pages/guardrails/CredentialsPage'));
const GuardrailsGroupsPage = lazy(() => import('./pages/guardrails/GroupsPage'));
const GuardrailsHistoryPage = lazy(() => import('./pages/guardrails/HistoryPage'));
const DashboardPage = lazy(() => import('./pages/DashboardPage'));
const UserUsagePage = lazy(() => import('./pages/UserUsagePage'));
const ModelTestPage = lazy(() => import('./pages/ModelTestPage'));
const UserPage = lazy(() => import('./pages/prompt/UserPage'));
const SkillPage = lazy(() => import('./pages/prompt/SkillPage'));
const TelegramPage = lazy(() => import('./pages/bots/TelegramPage'));
const FeishuPage = lazy(() => import('./pages/bots/FeishuPage'));
const LarkPage = lazy(() => import('./pages/bots/LarkPage'));
const DingTalkPage = lazy(() => import('./pages/bots/DingTalkPage'));
const WeixinPage = lazy(() => import('./pages/bots/WeixinPage'));
const WeComPage = lazy(() => import('./pages/bots/WeComPage'));
const QQPage = lazy(() => import('./pages/bots/QQPage'));
const DiscordPage = lazy(() => import('./pages/bots/DiscordPage'));
const SlackPage = lazy(() => import('./pages/bots/SlackPage'));
const BotOverviewPage = lazy(() => import('./pages/bots/BotOverviewPage'));
const RemoteAgentPage = lazy(() => import('./pages/remote-agent/RemoteAgentPage'));
const RemoteAgentEntryRedirect = lazy(() => import('./pages/remote-agent/RemoteAgentPage').then(m => ({ default: m.RemoteAgentEntryRedirect })));
const NotifyPage = lazy(() => import('./pages/notify/NotifyPage'));
const MCPLocalMode = lazy(() => import('./pages/mcp/MCPLocalMode'));
const MCPRegisteredServers = lazy(() => import('./pages/mcp/MCPRegisteredServers'));
const ServerToolPage = lazy(() => import('./pages/servertool/ServerToolPage'));
const HubPage = lazy(() => import('./pages/HubPage'));

// Route-switch fallback: Layout/nav chrome is already on screen (it renders
// outside this Suspense boundary), so this only covers the content area
// while a page chunk downloads — a brief spinner, not a full-page blank.
const RouteFallback = () => (
    <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '40vh' }}>
        <CircularProgress />
    </Box>
);

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
                slotProps={{
                    paper: {
                        sx: {
                            borderRadius: 2,
                            boxShadow: '0 8px 32px rgba(0,0,0,0.1)',
                        }
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
// installs (no provider configured) get sent to /help — the lightbulb Help
// page, whose ProvidersCard is the browsable "add your first provider"
// experience (the old standalone Onboarding page's content, now a card
// there instead of a page of its own); everyone else lands on the agent
// overview at /agent. We hit /api/v2/providers once on mount; while in
// flight we render nothing to avoid a flash of the default agent page.
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
                    setTarget('/help');
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

// LegacyBotSectionRedirect keeps old /remote-control/* bookmarks working. The
// combined section was split into Bots (resource) + Remote Agent (purpose)
// with identical per-platform pagination; old links land on the purpose side,
// which links onward to Bots.
const LegacyBotSectionRedirect = () => {
    const location = useLocation();
    const to = location.pathname.replace(/^\/remote-control/, '/remote-agent') + location.search;
    return <Navigate to={to} replace />;
};

// The original Remote Coder surface has been replaced by Remote Control.
// Keep bookmarks useful without exposing the retired pages and their dead API calls.
const LegacyRemoteCoderRedirect = () => {
    const location = useLocation();
    return <Navigate to={`/remote-agent${location.search}`} replace />;
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
        <Suspense fallback={<RouteFallback />}>
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
                    {/* Default landing: send first-time users (no providers) to Help,
                        everyone else to their last-active activity. */}
                    <Route index element={<OnboardingGate />} />
                    <Route path="/help" element={<HelpPage />} />
                    {/* Back-compat: the old standalone Onboarding page was folded into
                        Help as ProvidersCard — keep old bookmarks/links working. */}
                    <Route path="/onboarding" element={<Navigate to="/help" replace />} />
                    {/* Tray hub: compact landing page shown by the tray-mode window */}
                    <Route path="/hub" element={<HubPage />} />
                    {/* Function panel routes */}
                    <Route path="/agent" element={<AgentOverviewPage />} />
                    <Route path="/agent/openai" element={<UseOpenAIPage />} />
                    <Route path="/agent/anthropic" element={<UseAnthropicPage />} />
                    <Route path="/agent/codex" element={<UseCodexPage />} />
                    <Route path="/agent/claude_code" element={<UseClaudeCodePage />} />
                    <Route path="/agent/claude_code/profile/:profileId" element={<ClaudeCodeProfilePage />} />
                    <Route path="/agent/claude_desktop" element={<UseClaudeDesktopPage />} />
                    <Route path="/agent/custom" element={<UseCustomPage />} />
                    {/* "agent" was renamed to "custom" (formerly OpenClaw); keep the old
                        bookmarked path working. */}
                    <Route path="/agent/agent" element={<Navigate to="/agent/custom" replace />} />
                    <Route path="/agent/team" element={<UseTeamPage />} />
                    <Route path="/agent/team/:teamSlug" element={<UseTeamPage />} />
                    <Route path="/agent/opencode" element={<UseOpenCodePage />} />
                    <Route path="/agent/pi" element={<UsePiPage />} />
                    <Route path="/agent/dsh" element={<UseDshPage />} />
                    <Route path="/agent/xcode" element={<UseXcodePage />} />
                    <Route path="/agent/vscode" element={<UseVSCodePage />} />
                    <Route path="/agent/cursor" element={<UseCursorPage />} />
                    <Route path="/agent/embed" element={<UseEmbedPage />} />
                    <Route path="/agent/image" element={<UseImageGenPage />} />
                    <Route path="/agent/playground" element={<Navigate to="/agent/image" replace />} />
                    <Route path="/agent/imagegen" element={<Navigate to="/agent/image" replace />} />
                    {/* Credential routes - new unified page */}
                    <Route path="/credentials" element={<CredentialPage />} />
                    {/* Virtual Models page - peer of Model Key and Sharing */}
                    <Route path="/credentials/virtual-models" element={<VirtualModelsPage />} />
                    {/* Other routes */}
                    <Route path="/system" element={<System />} />
                    <Route path="/access-control" element={<AccessControl />} />
                    <Route path="/tingly-box-token" element={<SharingKeysPage />} />
                    <Route path="/system/develop" element={<DevelopPage />} />
                    <Route path="/system/logs" element={<LogsPage />} />
                    <Route path="/system/experimental" element={<ExperimentalPage />} />
                    {/* Dashboard routes with time range */}
                    <Route path="/dashboard" element={<Navigate to="/dashboard/7d" replace />} />
                    <Route path="/dashboard/users" element={<UserUsagePage />} />
                    <Route path="/dashboard/:timeRange" element={<DashboardPage />} />
                    {/* Token Heatmap merged into the Usage Dashboard; keep old
                        /overview links working by redirecting to the dashboard. */}
                    <Route path="/overview" element={<Navigate to="/dashboard/7d" replace />} />
                    <Route path="/overview/:timeRange" element={<Navigate to="/dashboard/7d" replace />} />
                    <Route path="/model-test/:providerUuid" element={<ModelTestPage />} />
                    {/* Prompt routes */}
                    <Route path="/prompt/user" element={<ExperimentalFeatureGate feature="skill_user"><UserPage /></ExperimentalFeatureGate>} />
                    <Route path="/prompt/skill" element={<ExperimentalFeatureGate feature="skill_ide"><SkillPage /></ExperimentalFeatureGate>} />
                    <Route path="/prompt/command" element={<Navigate to="/prompt/skill" replace />} />
                    {/* Retired Remote Coder bookmarks now enter Remote Control. */}
                    <Route path="/remote-coder" element={<LegacyRemoteCoderRedirect />} />
                    <Route path="/remote-coder/*" element={<LegacyRemoteCoderRedirect />} />
                    {/* Bots — Overview is the front door: every connected bot, across every
                        platform, one list. The old per-platform pages stay live (not
                        removed) as deep-link targets — /bots/:platform?add=1 links and any
                        bookmarks into them keep working — but Overview is what's in the nav. */}
                    <Route path="/bots" element={<Navigate to="/bots/overview" replace />} />
                    <Route path="/bots/overview" element={<BotOverviewPage />} />
                    <Route path="/bots/telegram" element={<TelegramPage />} />
                    <Route path="/bots/feishu" element={<FeishuPage />} />
                    <Route path="/bots/lark" element={<LarkPage />} />
                    <Route path="/bots/dingtalk" element={<DingTalkPage />} />
                    <Route path="/bots/weixin" element={<WeixinPage />} />
                    <Route path="/bots/wecom" element={<WeComPage />} />
                    <Route path="/bots/qq" element={<QQPage />} />
                    <Route path="/bots/discord" element={<DiscordPage />} />
                    <Route path="/bots/slack" element={<SlackPage />} />
                    {/* IM Notify — the other purpose mounted on a bot's channel. */}
                    <Route path="/notify" element={<NotifyPage />} />
                    {/* Remote Control — the purpose pages. One nav row (see useActivityItems);
                        platform selection is an in-page picker (RemoteAgentPage) instead
                        of a route per platform in the sidebar. The routes themselves are
                        unchanged, so deep links and the bot table purpose chip still work. */}
                    <Route path="/remote-agent" element={<RemoteAgentEntryRedirect />} />
                    <Route path="/remote-agent/:platform" element={<RemoteAgentPage />} />
                    {/* Back-compat: old /remote-control/* (the pre-split combined pages) → /remote-agent/* */}
                    <Route path="/remote-control" element={<Navigate to="/remote-agent" replace />} />
                    <Route path="/remote-control/*" element={<LegacyBotSectionRedirect />} />
                    {/* Guardrails */}
                    <Route path="/guardrails" element={<ExperimentalFeatureGate feature="guardrails"><GuardrailsPage /></ExperimentalFeatureGate>} />
                    <Route path="/guardrails/groups" element={<ExperimentalFeatureGate feature="guardrails"><GuardrailsGroupsPage /></ExperimentalFeatureGate>} />
                    <Route path="/guardrails/rules" element={<ExperimentalFeatureGate feature="guardrails"><GuardrailsRulesPage /></ExperimentalFeatureGate>} />
                    <Route path="/guardrails/credentials" element={<ExperimentalFeatureGate feature="guardrails"><GuardrailsCredentialsPage /></ExperimentalFeatureGate>} />
                    <Route path="/guardrails/history" element={<ExperimentalFeatureGate feature="guardrails"><GuardrailsHistoryPage /></ExperimentalFeatureGate>} />
                    {/* MCP Settings */}
                    <Route path="/mcp/sources" element={<ExperimentalFeatureGate feature="mcp"><MCPRegisteredServers /></ExperimentalFeatureGate>} />
                    <Route path="/mcp/local-mode" element={<ExperimentalFeatureGate feature="mcp"><MCPLocalMode /></ExperimentalFeatureGate>} />
                    <Route path="/mcp" element={<ExperimentalFeatureGate feature="mcp"><Navigate to="/mcp/sources" replace /></ExperimentalFeatureGate>} />
                    {/* Tools */}
                    <Route path="/tools/servertool" element={<ExperimentalFeatureGate feature="mcp"><ServerToolPage /></ExperimentalFeatureGate>} />
                    {/* Catch-all redirect for unknown routes (also covers legacy /zen/* links) */}
                    <Route path="*" element={<Navigate to="/agent" replace />} />
                </Route>
            </Routes>
        </Suspense>
    )
}

// Inner component that uses theme context
function AppWithTheme() {
    const { effectiveMode } = useThemeMode();
    const { i18n } = useTranslation();
    const language = resolveLanguage(i18n.language);
    const theme = useMemo(() => createAppTheme(effectiveMode, language), [effectiveMode, language]);

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
                                        <TeamProvider>
                                            <AppContent />
                                            <AppDialogs />
                                        </TeamProvider>
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
