import { displayVersion } from '@/utils/version';
import {
    Box,
    Chip,
    IconButton,
    Paper,
    Stack,
    Tooltip,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { host } from '@/host';
import { AiAgents, BarChart, Lock, Settings, TextSnippet } from '@/components/icons';
import { useHealth } from '@/contexts/HealthContext';
import { useVersion } from '@/contexts/VersionContext';
import { ProviderQuotaPanel } from '@/components/ProviderQuotaPanel';
import { fetchUIAPI } from '@/services/api';
import { SHELL_ROUTES } from '@/routes/shellRoutes';

// HubPage is the tray's compact panel — a dedicated small window (see
// gui/wails3/systray.go's useSystray) separate from the main app window. It
// never navigates itself; its jumps open the main window instead, so this
// page only ever renders /hub. Content is scoped to what's
// not already obvious the moment you open the full app: server health,
// update availability, and provider quota.
export default function HubPage() {
    const { t } = useTranslation();
    const { isHealthy } = useHealth();
    const { currentVersion, hasUpdate } = useVersion();
    // Opens the separate main app window at the given path. HTTP-first: the
    // same-origin /api/v1/gui/open route reaches the exact same Go handler
    // and — unlike the wails bound call (host.openMainWindow), which has silently failed in the
    // panel webview before — rides the plain fetch path that every other
    // API call on this page already uses successfully. The bound method
    // stays as fallback in case the HTTP route is unreachable.
    const openMainWindow = async (path: string) => {
        try {
            await fetchUIAPI(`/gui/open?path=${encodeURIComponent(path)}`, { method: 'POST' });
        } catch {
            await host.openMainWindow(path);
        }
    };

    const versionKnown = Boolean(currentVersion) && currentVersion !== 'Unknown';

    // The main window's rail, in its order and with its icons and labels, so a
    // jump from the panel lands where the same icon would have taken you.
    const navItems = [
        { path: SHELL_ROUTES.agent, label: t('layout.nav.home'), icon: <AiAgents sx={{ fontSize: 20 }} /> },
        { path: SHELL_ROUTES.dashboard, label: t('layout.dashboard'), icon: <BarChart sx={{ fontSize: 20 }} /> },
        { path: SHELL_ROUTES.credentials, label: t('layout.nav.credential'), icon: <Lock sx={{ fontSize: 20 }} /> },
        { path: SHELL_ROUTES.logs, label: t('layout.logs'), icon: <TextSnippet sx={{ fontSize: 20 }} /> },
        { path: SHELL_ROUTES.system, label: t('layout.system'), icon: <Settings sx={{ fontSize: 20 }} /> },
    ];

    return (
        <Box
            sx={{
                height: '100vh',
                display: 'flex',
                flexDirection: 'column',
                bgcolor: 'background.default',
                p: 1.25,
                gap: 1.25,
            }}
        >
            {/* Header: identity + status, like a menu-bar app's masthead */}
            <Stack direction="row" spacing={1} sx={{ alignItems: 'center', px: 0.5, pt: 0.25 }}>
                {/* Same brand mark the app's activity bar uses */}
                <Box component="img" src="/assets/icon.svg" alt="" sx={{ width: 28, height: 28, borderRadius: 1 }} />
                <Box sx={{ flex: 1, minWidth: 0 }}>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, lineHeight: 1.3 }} noWrap>
                        {t('hub.title')}
                    </Typography>
                    <Stack direction="row" spacing={0.5} sx={{ alignItems: 'center' }}>
                        <Box
                            sx={{
                                width: 6,
                                height: 6,
                                borderRadius: '50%',
                                bgcolor: isHealthy ? 'success.main' : 'error.main',
                                flexShrink: 0,
                            }}
                        />
                        <Typography variant="caption" color="text.secondary" noWrap>
                            {isHealthy ? t('hub.status.healthy') : t('hub.status.unhealthy')}
                            {versionKnown && ` · ${displayVersion(currentVersion)}`}
                        </Typography>
                    </Stack>
                </Box>
                {/* Only claim an update when we actually know what we're on —
                    dev builds report an unknown version and would otherwise
                    always flag one. */}
                {hasUpdate && versionKnown && (
                    <Chip
                        size="small"
                        label={t('hub.status.updateAvailable')}
                        color="info"
                        variant="outlined"
                        onClick={() => openMainWindow(SHELL_ROUTES.system)}
                    />
                )}
            </Stack>

            {/* Jumps into the main window, mirroring its rail */}
            <Paper elevation={0} sx={{ borderRadius: 2, bgcolor: 'background.paper', display: 'flex', justifyContent: 'space-around', py: 0.5 }}>
                {navItems.map(({ path, label, icon }) => (
                    <Tooltip key={path} title={label} arrow>
                        <IconButton aria-label={label} onClick={() => openMainWindow(path)} sx={{ color: 'text.secondary', '&:hover': { color: 'primary.main' } }}>
                            {icon}
                        </IconButton>
                    </Tooltip>
                ))}
            </Paper>

            {/* Provider quota card */}
            <ProviderQuotaPanel onNavigate={openMainWindow} />
        </Box>
    );
}
