import { Box, Drawer, IconButton, Popover, Tooltip, Typography, Menu, MenuItem, Stack } from '@mui/material';
import { IconMenu, IconDots, IconYinYang, IconPencil } from '@tabler/icons-react';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Outlet, useLocation, useNavigate } from 'react-router-dom';
import { useVersion as useAppVersion } from '../contexts/VersionContext';
import { useThemeMode } from '../contexts/ThemeContext';
import { Z_INDEX } from '../constants/zIndex';
import { activityBarWidth, sidebarWidth } from './constants';
import { ZenActivityBar } from './ZenActivityBar.tsx';
import { Sidebar } from './Sidebar';
import { useActivityItems } from './useActivityItems.tsx';
import { useZenMode } from '../hooks/useZenMode';
import { useProfileContext } from '../contexts/ProfileContext';
import type { ActivityItem, LayoutProps } from './types';
import { getThemeOptions } from '@/theme/options';
import { Claude, Codex, OpenCode, Xcode, VSCode, OpenAI, Anthropic, OpenClaw } from '../components/BrandIcons';
import { FloatingStatusIndicators } from '../components/FloatingStatusIndicators';
import { IconPlus } from '@tabler/icons-react';

const mobileContentSx = {
    flex: 1,
    px: { xs: 2, md: 3 },
    pt: { xs: 9, md: 3 },
    pb: 3,
    overflowY: 'auto',
    scrollBehavior: 'smooth',
    '&::-webkit-scrollbar': { width: 8 },
    '&::-webkit-scrollbar-track': { backgroundColor: 'grey.100', borderRadius: 1 },
    '&::-webkit-scrollbar-thumb': {
        backgroundColor: 'grey.300',
        borderRadius: 1,
        '&:hover': { backgroundColor: 'grey.400' },
    },
} as const;

const MobileNavigationBar = ({ onMenuClick }: { onMenuClick: () => void }) => (
    <Box
        sx={{
            display: { xs: 'flex', md: 'none' },
            position: 'fixed',
            top: 0,
            left: 0,
            right: 0,
            height: 56,
            zIndex: Z_INDEX.mobileToggle,
            alignItems: 'center',
            px: 1,
            bgcolor: 'background.paper',
            borderBottom: '1px solid',
            borderColor: 'divider',
        }}
    >
        <IconButton
            color="primary"
            aria-label="Open navigation menu"
            onClick={onMenuClick}
            sx={{
                width: 44,
                height: 44,
                '&:hover': { bgcolor: 'action.hover' },
            }}
        >
            <IconMenu size={24} />
        </IconButton>
    </Box>
);

const Layout = ({ children }: LayoutProps) => {
    const { t } = useTranslation();
    const location = useLocation();
    const navigate = useNavigate();
    const { currentVersion } = useAppVersion();
    const { setTheme } = useThemeMode();
    const themeMenuOptions = useMemo(() => getThemeOptions(t), [t]);
    const [mobileOpen, setMobileOpen] = useState(false);
    const [easterEggAnchorEl, setEasterEggAnchorEl] = useState<HTMLElement | null>(null);
    const [moreMenuAnchorEl, setMoreMenuAnchorEl] = useState<HTMLElement | null>(null);
    const [zenMenuAnchorEl, setZenMenuAnchorEl] = useState<HTMLElement | null>(null);
    // When a "More" menu item is selected in zen mode, track which activity's sidebar to show
    const [zenMoreActivity, setZenMoreActivity] = useState<string | null>(null);

    const handleZenAgentSelect = (zenPath: string) => {
        const agent = zenPath.replace('/zen/', '').replace('-', '_');
        localStorage.setItem('mock-flag-_global-zen', agent);
        window.location.href = zenPath;
    };

    // Zen mode state
    const { enabled: zenEnabled, agent: zenAgentFromHook, loading: zenLoading, setZenMode } = useZenMode();

    // Determine if we're in zen mode (either from flag or from URL path)
    const isInZenPath = /^\/zen\//.test(location.pathname);
    const effectiveZenEnabled = zenEnabled || isInZenPath;

    // Extract current zen agent from URL path if in zen mode
    const getCurrentZenAgent = (): string => {
        if (isInZenPath) {
            const match = location.pathname.match(/^\/zen\/([^/]+)$/);
            if (match) return match[1];
        }
        return zenAgentFromHook || '';
    };

    const zenAgent = getCurrentZenAgent();
    const { profiles } = useProfileContext();

    const activityItems = useActivityItems();

    const isActive = (path: string) => location.pathname === path;
    const isChildActive = (children?: ActivityItem['children']) =>
        children?.some(item => item.type !== 'divider' && isActive(item.path)) ?? false;

    // Determine active activity from current path, falling back to localStorage
    const activeActivity = useMemo(() => {
        if (location.pathname === '/onboarding') return 'onboarding';
        for (const item of activityItems) {
            if (item.path && isActive(item.path)) return item.key;
            if (item.children && isChildActive(item.children)) return item.key;
        }
        // Check if saved activity is still valid
        const saved = sessionStorage.getItem('layout.activeActivity') || localStorage.getItem('layout.activeActivity');
        if (saved && activityItems.some(item => item.key === saved)) return saved;
        // Fallback to 'scenario' (which is valid - it's the agent activity)
        return 'scenario';
    }, [activityItems, location.pathname]);

    // In zen mode, active activity is based on current zen path
    const zenActiveActivity = useMemo(() => {
        if (effectiveZenEnabled) {
            const zenPathMatch = location.pathname.match(/^\/zen\/([^/]+)$/);
            if (zenPathMatch) return `zen-${zenPathMatch[1]}`;
            return 'zen-claude_code';
        }
        return activeActivity;
    }, [effectiveZenEnabled, location.pathname, activeActivity]);

    // Persist the active activity for cross-session boot. Each activity
    // re-opens at its defaultPath (the scenario activity opens its overview).
    useEffect(() => {
        sessionStorage.setItem('layout.activeActivity', activeActivity);
        localStorage.setItem('layout.activeActivity', activeActivity);
    }, [activeActivity, location.pathname]);

    // When navigating to a zen path, clear zenMoreActivity
    useEffect(() => {
        if (isInZenPath) {
            setZenMoreActivity(null);
        }
    }, [isInZenPath]);

    // Sidebar items - normal mode or zen mode showing other activities
    const sidebarItems = useMemo(() => {
        // Zen mode with a "More" activity selected - show that activity's children
        if (effectiveZenEnabled && zenMoreActivity) {
            const activity = activityItems.find(item => item.key === zenMoreActivity);
            return activity?.children || [];
        }

        // Zen mode - get agent + profiles
        if (effectiveZenEnabled && zenAgent) {
            const getAgentInfo = (agent: string) => {
                // Default to claude_code if agent is empty
                const normalizedAgent = agent || 'claude_code';
                const info: Record<string, { icon: any; label: string; path: string; scenario: string; hasProfiles: boolean }> = {
                    claude_code: { icon: <Claude size={20} />, label: 'Claude Code', path: '/agent/claude_code', scenario: 'claude_code', hasProfiles: true },
                    codex: { icon: <Codex size={20} />, label: 'Codex', path: '/agent/codex', scenario: 'codex', hasProfiles: false },
                    opencode: { icon: <OpenCode size={20} />, label: 'OpenCode', path: '/agent/opencode', scenario: 'opencode', hasProfiles: false },
                    xcode: { icon: <Xcode size={20} />, label: 'Xcode', path: '/agent/xcode', scenario: 'xcode', hasProfiles: false },
                    vscode: { icon: <VSCode size={20} />, label: 'VS Code', path: '/agent/vscode', scenario: 'vscode', hasProfiles: false },
                    openai: { icon: <OpenAI size={20} />, label: 'OpenAI', path: '/agent/openai', scenario: 'openai', hasProfiles: false },
                    anthropic: { icon: <Anthropic size={20} />, label: 'Anthropic', path: '/agent/anthropic', scenario: 'anthropic', hasProfiles: false },
                    agent: { icon: <OpenClaw size={20} />, label: 'OpenClaw', path: '/agent/agent', scenario: 'agent', hasProfiles: false },
                };
                return info[normalizedAgent] || info.claude_code;
            };

            const agentInfo = getAgentInfo(zenAgent);
            const zenAgentPath = `/zen/${zenAgent}`;
            const zenSidebarItems: ActivityItem['children'] = [
                { path: zenAgentPath, label: agentInfo.label, icon: agentInfo.icon, subtitle: 'default' },
            ];

            // Only add profiles for Claude Code
            if (agentInfo.hasProfiles) {
                const agentProfiles = profiles[agentInfo.scenario] || [];
                agentProfiles.forEach(profile => {
                    zenSidebarItems.push({
                        path: `/zen/${zenAgent}/profile/${profile.id}`,
                        label: agentInfo.label,
                        icon: agentInfo.icon,
                        subtitle: `${profile.id} - ${profile.name}`,
                    });
                });

                // Add profile divider and add profile button
                zenSidebarItems.push({ type: 'divider' });
                zenSidebarItems.push({
                    path: '#add-profile',
                    label: 'Add Profile',
                    icon: <IconPlus size={20} />,
                });
            }

            return zenSidebarItems;
        }

        // Normal mode
        const activity = activityItems.find(item => item.key === activeActivity);
        return activity?.children || [];
    }, [effectiveZenEnabled, zenAgent, zenMoreActivity, activityItems, activeActivity, profiles]);

    const activeActivityLabel = useMemo(() => {
        if (effectiveZenEnabled && zenMoreActivity) {
            const activity = activityItems.find(item => item.key === zenMoreActivity);
            return activity?.label || '';
        }
        if (effectiveZenEnabled && zenAgent && zenActiveActivity === 'zen-agent') {
            const labelMap: Record<string, string> = {
                claude_code: 'Claude Code',
                codex: 'Codex',
                opencode: 'OpenCode',
                xcode: 'Xcode',
                vscode: 'VS Code',
                openai: 'OpenAI',
                anthropic: 'Anthropic',
                agent: 'OpenClaw',
            };
            return labelMap[zenAgent] || zenAgent;
        }
        const activity = activityItems.find(item => item.key === activeActivity);
        return activity?.label || '';
    }, [effectiveZenEnabled, zenAgent, zenMoreActivity, zenActiveActivity, activityItems, activeActivity]);

    const handleActivityClick = (item: ActivityItem) => {
        const hasSidebarItems = item.children?.some(child => child.type !== 'divider') ?? false;
        if (!hasSidebarItems) {
            setMobileOpen(false);
        }
        setMoreMenuAnchorEl(null); // Close more menu if open

        // If clicking a zen path item, clear the zenMoreActivity to restore zen sidebar
        if (item.path?.startsWith('/zen/')) {
            setZenMoreActivity(null);
            navigate(item.path);
            return;
        }

        sessionStorage.setItem('layout.activeActivity', item.key);

        // Every activity opens at its defaultPath when clicked. The scenario
        // activity points at /agent (the overview) so users always land there
        // before drilling into a specific scenario.
        const firstNavChild = item.children?.find(c => c.type !== 'divider');
        let targetPath = item.defaultPath || item.path || firstNavChild?.path;

        // Ultimate fallback to prevent navigation to invalid paths
        if (!targetPath && firstNavChild) {
            targetPath = firstNavChild.path;
        }

        if (targetPath) navigate(targetPath);
    };

    // Get zen mode activity items
    const getZenActivityItems = (): ActivityItem[] => {
        const zenPathMatch = location.pathname.match(/^\/zen\/([^/]+)$/);
        const currentAgent = zenPathMatch ? zenPathMatch[1] : 'claude_code';

        const getAgentInfo = (agent: string) => {
            const info: Record<string, { key: string; icon: any; label: string; path: string }> = {
                'claude_code': { key: 'zen-claude_code', icon: <Claude size={22} />, label: 'Claude', path: '/zen/claude_code' },
                'codex':       { key: 'zen-codex',       icon: <Codex size={22} />,   label: 'Codex',       path: '/zen/codex' },
                'opencode':    { key: 'zen-opencode',    icon: <OpenCode size={22} />, label: 'OpenCode',    path: '/zen/opencode' },
                'xcode':       { key: 'zen-xcode',       icon: <Xcode size={22} />,   label: 'Xcode',       path: '/zen/xcode' },
                'vscode':      { key: 'zen-vscode',      icon: <VSCode size={22} />,  label: 'VS Code',     path: '/zen/vscode' },
                'openai':      { key: 'zen-openai',      icon: <OpenAI size={22} />,  label: 'OpenAI',      path: '/zen/openai' },
                'anthropic':   { key: 'zen-anthropic',   icon: <Anthropic size={22} />, label: 'Anthropic', path: '/zen/anthropic' },
                'agent':       { key: 'zen-agent',       icon: <OpenClaw size={22} />, label: 'OpenClaw',   path: '/zen/agent' },
            };
            return info[agent] || info['claude_code'];
        };

        const agentInfo = getAgentInfo(currentAgent);
        return [{ key: agentInfo.key, icon: agentInfo.icon, label: agentInfo.label, path: agentInfo.path }];
    };

    const zenActivityItems = getZenActivityItems();

    const sidebarHeaderAction = activeActivity === 'scenario' ? (
        <Stack direction="row" spacing={0.5} alignItems="center">
            <Tooltip title={t('layout.activityBar.zenMode')} arrow placement="bottom">
                <IconButton
                    size="small"
                    onClick={(e) => setZenMenuAnchorEl(e.currentTarget)}
                    sx={{
                        color: 'text.secondary',
                        '&:hover': { color: 'primary.main' },
                    }}
                >
                    <IconYinYang size={16} />
                </IconButton>
            </Tooltip>
            <Tooltip title={t('scenarioOverview.editTooltip', { defaultValue: 'Manage visible agents' })} arrow placement="right">
                <IconButton
                    size="small"
                    onClick={() => navigate('/agent')}
                    sx={{
                        color: 'text.secondary',
                        '&:hover': { color: 'primary.main' },
                    }}
                >
                    <IconPencil size={16} />
                </IconButton>
            </Tooltip>
        </Stack>
    ) : undefined;

    const navigationContent = (
        <Box sx={{ display: 'flex', height: '100%' }}>
            <ZenActivityBar
                activityItems={activityItems}
                activeActivity={activeActivity}
                onActivityClick={handleActivityClick}
                onUserClick={(e) => setEasterEggAnchorEl(e.currentTarget)}
                onZenToggle={() => {
                    // Enter zen mode
                    localStorage.setItem('mock-flag-_global-zen', 'claude_code');
                    window.location.reload();
                }}
                zenEnabled={effectiveZenEnabled}
                onStandaloneNavigate={() => setMobileOpen(false)}
            />
            {sidebarItems.length > 0 && (
                <Sidebar
                    sidebarItems={sidebarItems}
                    activeActivityLabel={activeActivityLabel}
                    onClose={() => setMobileOpen(false)}
                    headerAction={sidebarHeaderAction}
                />
            )}
        </Box>
    );

    const zenNavigationContent = (
        <Box sx={{ display: 'flex', height: '100%' }}>
            <ZenActivityBar
                activityItems={zenActivityItems}
                activeActivity={zenActiveActivity}
                onActivityClick={handleActivityClick}
                onUserClick={(e) => setEasterEggAnchorEl(e.currentTarget)}
                onZenToggle={() => {
                    // Exit zen mode - clear flag and redirect to dashboard
                    localStorage.removeItem('mock-flag-_global-zen');
                    window.location.href = '/dashboard/7d';
                }}
                zenEnabled={effectiveZenEnabled}
                onMoreClick={(e) => setMoreMenuAnchorEl(e.currentTarget)}
                onStandaloneNavigate={() => setMobileOpen(false)}
            />
            {sidebarItems.length > 0 && (
                <Sidebar
                    sidebarItems={sidebarItems}
                    activeActivityLabel={activeActivityLabel}
                    onClose={() => setMobileOpen(false)}
                />
            )}
        </Box>
    );

    return (
        <Box sx={{ display: 'flex', height: '100vh', overflow: 'hidden', position: 'relative', zIndex: Z_INDEX.main }}>
            <FloatingStatusIndicators />
            {/* Zen Mode Layout */}
            {effectiveZenEnabled && !zenLoading && zenAgent ? (
                <>
                    {/* Desktop nav - Zen mode */}
                    <Box component="nav" sx={{ display: { xs: 'none', md: 'flex' }, height: '100%', position: 'relative', zIndex: Z_INDEX.drawer + 1 }}>
                        {zenNavigationContent}
                    </Box>

                    {/* Mobile Drawer - Zen mode */}
                    <Drawer
                        variant="temporary"
                        open={mobileOpen}
                        onClose={() => setMobileOpen(false)}
                        ModalProps={{ keepMounted: true }}
                        sx={{
                            display: { xs: 'block', md: 'none' },
                            '& .MuiDrawer-paper': {
                                boxSizing: 'border-box',
                                width: activityBarWidth + sidebarWidth,
                                zIndex: Z_INDEX.drawer,
                            },
                        }}
                    >
                        {zenNavigationContent}
                    </Drawer>

                    <MobileNavigationBar onMenuClick={() => setMobileOpen(!mobileOpen)} />

                    {/* Main content */}
                    <Box
                        component="main"
                        sx={{ flexGrow: 1, height: '100vh', display: 'flex', flexDirection: 'column', overflowX: 'hidden', position: 'relative', zIndex: 1 }}
                    >
                        <Box
                            sx={mobileContentSx}
                        >
                            {children ?? <Outlet />}
                        </Box>
                    </Box>

                    {/* More Menu */}
                    <Menu
                        anchorEl={moreMenuAnchorEl}
                        open={Boolean(moreMenuAnchorEl)}
                        onClose={() => setMoreMenuAnchorEl(null)}
                        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
                        transformOrigin={{ vertical: 'top', horizontal: 'right' }}
                        slotProps={{
                            paper: {
                                sx: { minWidth: 200, maxHeight: 400 },
                            },
                        }}
                    >
                        <MenuItem disabled sx={{ opacity: 0.6 }}>
                            <Typography variant="caption" sx={{ fontWeight: 600 }}>
                                {t('layout.themeMenu.switchTo')}
                            </Typography>
                        </MenuItem>
                        {activityItems.map((activity) => (
                            <MenuItem
                                key={activity.key}
                                onClick={() => {
                                    if (activity.children && activity.children.length > 0) {
                                        // Show the secondary sidebar for this activity
                                        setZenMoreActivity(activity.key);
                                        sessionStorage.setItem('layout.activeActivity', activity.key);
                                        // Navigate to first child
                                        const firstNavChild = activity.children.find(c => c.type !== 'divider');
                                        const targetPath = activity.path || firstNavChild?.path;
                                        if (targetPath) navigate(targetPath);
                                    } else {
                                        sessionStorage.setItem('layout.activeActivity', activity.key);
                                        const targetPath = activity.path;
                                        if (targetPath) navigate(targetPath);
                                    }
                                    setMoreMenuAnchorEl(null);
                                }}
                                selected={zenActiveActivity === activity.key || zenMoreActivity === activity.key}
                            >
                                {activity.icon}
                                <Typography sx={{ ml: 1 }}>{activity.label}</Typography>
                            </MenuItem>
                        ))}

                        {/* Theme options - only in zen mode */}
                        {effectiveZenEnabled && (
                            <MenuItem disabled sx={{ opacity: 0.6 }}>
                                <Typography variant="caption" sx={{ fontWeight: 600 }}>
                                    {t('layout.themeMenu.theme')}
                                </Typography>
                            </MenuItem>
                        )}
                        {effectiveZenEnabled && themeMenuOptions.map(({ value, label, renderIcon }) => (
                            <MenuItem key={value} onClick={() => setTheme(value)} sx={{ gap: 1.5 }}>
                                {renderIcon({ size: 18 })}
                                <Typography>{label}</Typography>
                            </MenuItem>
                        ))}
                    </Menu>

                    {/* Easter Egg Popover */}
                    <Popover
                        open={Boolean(easterEggAnchorEl)}
                        anchorEl={easterEggAnchorEl}
                        onClose={() => setEasterEggAnchorEl(null)}
                        anchorOrigin={{ vertical: 'top', horizontal: 'center' }}
                        transformOrigin={{ vertical: 'bottom', horizontal: 'center' }}
                        sx={{ zIndex: Z_INDEX.popover, '& .MuiPopover-paper': { bgcolor: 'primary.main', color: 'white', borderRadius: 2, px: 2, py: 1 } }}
                    >
                        {t('layout.easterEgg')} · {currentVersion}
                    </Popover>
                </>
            ) : (
                <>
                    {/* Normal Mode Layout */}
                    {/* Desktop nav */}
                    <Box component="nav" sx={{ display: { xs: 'none', md: 'flex' }, height: '100%', position: 'relative', zIndex: Z_INDEX.drawer + 1 }}>
                        {navigationContent}
                    </Box>

                    {/* Mobile Drawer */}
                    <Drawer
                        variant="temporary"
                        open={mobileOpen}
                        onClose={() => setMobileOpen(false)}
                        ModalProps={{ keepMounted: true }}
                        sx={{
                            display: { xs: 'block', md: 'none' },
                            '& .MuiDrawer-paper': {
                                boxSizing: 'border-box',
                                width: sidebarItems.length > 0 ? activityBarWidth + sidebarWidth : activityBarWidth,
                                zIndex: Z_INDEX.drawer,
                            },
                        }}
                    >
                        {navigationContent}
                    </Drawer>

                    <MobileNavigationBar onMenuClick={() => setMobileOpen(!mobileOpen)} />

                    {/* Main content */}
                    <Box
                        component="main"
                        sx={{ flexGrow: 1, height: '100vh', display: 'flex', flexDirection: 'column', overflowX: 'hidden', position: 'relative', zIndex: 1 }}
                    >
                        <Box
                            sx={mobileContentSx}
                        >
                            {children ?? <Outlet />}
                        </Box>
                    </Box>

                    {/* Easter Egg Popover */}
                    <Popover
                        open={Boolean(easterEggAnchorEl)}
                        anchorEl={easterEggAnchorEl}
                        onClose={() => setEasterEggAnchorEl(null)}
                        anchorOrigin={{ vertical: 'top', horizontal: 'center' }}
                        transformOrigin={{ vertical: 'bottom', horizontal: 'center' }}
                        sx={{ zIndex: Z_INDEX.popover, '& .MuiPopover-paper': { bgcolor: 'primary.main', color: 'white', borderRadius: 2, px: 2, py: 1 } }}
                    >
                        {t('layout.easterEgg')} · {currentVersion}
                    </Popover>

                    {/* Zen Agent Selection Menu */}
                    <Menu
                        anchorEl={zenMenuAnchorEl}
                        open={Boolean(zenMenuAnchorEl)}
                        onClose={() => setZenMenuAnchorEl(null)}
                        anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
                        transformOrigin={{ vertical: 'top', horizontal: 'left' }}
                        slotProps={{ paper: { sx: { minWidth: 180, mt: 0.5 } } }}
                    >
                        <MenuItem disabled sx={{ opacity: 0.6 }}>
                            <Typography variant="caption" sx={{ fontWeight: 600 }}>
                                {t('layout.activityBar.enterZenMode')}
                            </Typography>
                        </MenuItem>
                        <MenuItem onClick={() => handleZenAgentSelect('/zen/claude_code')} sx={{ gap: 1.5 }}>
                            <Claude size={18} />
                            <Typography>Claude Code</Typography>
                        </MenuItem>
                        <MenuItem onClick={() => handleZenAgentSelect('/zen/codex')} sx={{ gap: 1.5 }}>
                            <Codex size={18} />
                            <Typography>Codex</Typography>
                        </MenuItem>
                        <MenuItem onClick={() => handleZenAgentSelect('/zen/opencode')} sx={{ gap: 1.5 }}>
                            <OpenCode size={18} />
                            <Typography>OpenCode</Typography>
                        </MenuItem>
                    </Menu>
                </>
            )}
        </Box>
    );
};

export default Layout;
