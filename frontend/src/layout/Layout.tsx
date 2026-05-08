import { Box, Drawer, IconButton, Popover, Typography, Menu, MenuItem } from '@mui/material';
import { IconMenu, IconDots, IconYinYang, IconSun, IconMoon, IconSunHigh } from '@tabler/icons-react';
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
import { Claude, Codex, OpenCode, Xcode, VSCode, OpenAI, Anthropic, OpenClaw } from '../components/BrandIcons';
import { FloatingStatusIndicators } from '../components/FloatingStatusIndicators';
import { IconPlus } from '@tabler/icons-react';

const Layout = ({ children }: LayoutProps) => {
    const { t } = useTranslation();
    const location = useLocation();
    const navigate = useNavigate();
    const { currentVersion } = useAppVersion();
    const { setTheme } = useThemeMode();
    const [mobileOpen, setMobileOpen] = useState(false);
    const [easterEggAnchorEl, setEasterEggAnchorEl] = useState<HTMLElement | null>(null);
    const [moreMenuAnchorEl, setMoreMenuAnchorEl] = useState<HTMLElement | null>(null);
    // When a "More" menu item is selected in zen mode, track which activity's sidebar to show
    const [zenMoreActivity, setZenMoreActivity] = useState<string | null>(null);

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

    // Persist the active activity for cross-session boot. Per-activity path
    // memory is intentionally scoped to the scenario (agent) activity only —
    // every other activity opens at its defaultPath when re-clicked.
    useEffect(() => {
        sessionStorage.setItem('layout.activeActivity', activeActivity);
        localStorage.setItem('layout.activeActivity', activeActivity);
        if (activeActivity === 'scenario') {
            sessionStorage.setItem(`layout.activityPath.${activeActivity}`, location.pathname);
            localStorage.setItem(`layout.activityPath.${activeActivity}`, location.pathname);
        }
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
        setMobileOpen(false);
        setMoreMenuAnchorEl(null); // Close more menu if open

        // If clicking a zen path item, clear the zenMoreActivity to restore zen sidebar
        if (item.path?.startsWith('/zen/')) {
            setZenMoreActivity(null);
            navigate(item.path);
            return;
        }

        sessionStorage.setItem('layout.activeActivity', item.key);

        // Only the scenario activity restores its previously visited sub-page —
        // every other activity always opens at its defaultPath, so users never
        // get a stale "last viewed" view they don't expect.
        const firstNavChild = item.children?.find(c => c.type !== 'divider');
        let targetPath: string | undefined;

        if (item.key === 'scenario') {
            const savedPath = sessionStorage.getItem(`layout.activityPath.${item.key}`) || localStorage.getItem(`layout.activityPath.${item.key}`);
            if (savedPath && item.children?.some(c => c.type !== 'divider' && c.path === savedPath)) {
                targetPath = savedPath;
            }
        }

        if (!targetPath) {
            targetPath = item.defaultPath || item.path || firstNavChild?.path;
        }

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

                    {/* Mobile toggle */}
                    <IconButton
                        color="primary"
                        aria-label="Open navigation menu"
                        onClick={() => setMobileOpen(!mobileOpen)}
                        sx={{
                            display: { xs: 'flex', md: 'none' },
                            position: 'fixed',
                            top: 12,
                            left: 12,
                            zIndex: Z_INDEX.mobileToggle,
                            boxShadow: 3,
                            width: 44,
                            height: 44,
                            '&:hover': { bgcolor: 'action.hover', transform: 'scale(1.05)' },
                            transition: 'all 0.15s',
                        }}
                    >
                        <IconMenu size={24} />
                    </IconButton>

                    {/* Main content */}
                    <Box
                        component="main"
                        sx={{ flexGrow: 1, height: '100vh', display: 'flex', flexDirection: 'column', overflowX: 'hidden', position: 'relative', zIndex: 1 }}
                    >
                        <Box
                            sx={{
                                flex: 1,
                                p: 3,
                                overflowY: 'auto',
                                scrollBehavior: 'smooth',
                                '&::-webkit-scrollbar': { width: 8 },
                                '&::-webkit-scrollbar-track': { backgroundColor: 'grey.100', borderRadius: 1 },
                                '&::-webkit-scrollbar-thumb': {
                                    backgroundColor: 'grey.300',
                                    borderRadius: 1,
                                    '&:hover': { backgroundColor: 'grey.400' },
                                },
                            }}
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
                            <>
                                <MenuItem disabled sx={{ opacity: 0.6 }}>
                                    <Typography variant="caption" sx={{ fontWeight: 600 }}>
                                        {t('layout.themeMenu.theme')}
                                    </Typography>
                                </MenuItem>
                                <MenuItem onClick={() => setTheme('light')} sx={{ gap: 1.5 }}>
                                    <IconSun size={18} />
                                    <Typography>{t('layout.activityBar.light')}</Typography>
                                </MenuItem>
                                <MenuItem onClick={() => setTheme('dark')} sx={{ gap: 1.5 }}>
                                    <IconMoon size={18} />
                                    <Typography>{t('layout.activityBar.dark')}</Typography>
                                </MenuItem>
                                <MenuItem onClick={() => setTheme('sunlit')} sx={{ gap: 1.5 }}>
                                    <IconSunHigh size={18} />
                                    <Typography>{t('layout.activityBar.sunlit')}</Typography>
                                </MenuItem>
                            </>
                        )}
                    </Menu>

                    {/* Easter Egg Popover */}
                    <Popover
                        open={Boolean(easterEggAnchorEl)}
                        anchorEl={easterEggAnchorEl}
                        onClose={() => setEasterEggAnchorEl(null)}
                        anchorOrigin={{ vertical: 'top', horizontal: 'center' }}
                        transformOrigin={{ vertical: 'bottom', horizontal: 'center' }}
                        sx={{ zIndex: Z_INDEX.popover, '& .MuiPopover-paper': { bgcolor: 'primary.main', color: 'white', borderRadius: 2, px: 2, py: 1, fontSize: '0.875rem' } }}
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

                    {/* Mobile toggle */}
                    <IconButton
                        color="primary"
                        aria-label="Open navigation menu"
                        onClick={() => setMobileOpen(!mobileOpen)}
                        sx={{
                            display: { xs: 'flex', md: 'none' },
                            position: 'fixed',
                            top: 12,
                            left: 12,
                            zIndex: Z_INDEX.mobileToggle,
                            boxShadow: 3,
                            width: 44,
                            height: 44,
                            '&:hover': { bgcolor: 'action.hover', transform: 'scale(1.05)' },
                            transition: 'all 0.15s',
                        }}
                    >
                        <IconMenu size={24} />
                    </IconButton>

                    {/* Main content */}
                    <Box
                        component="main"
                        sx={{ flexGrow: 1, height: '100vh', display: 'flex', flexDirection: 'column', overflowX: 'hidden', position: 'relative', zIndex: 1 }}
                    >
                        <Box
                            sx={{
                                flex: 1,
                                p: 3,
                                overflowY: 'auto',
                                scrollBehavior: 'smooth',
                                '&::-webkit-scrollbar': { width: 8 },
                                '&::-webkit-scrollbar-track': { backgroundColor: 'grey.100', borderRadius: 1 },
                                '&::-webkit-scrollbar-thumb': {
                                    backgroundColor: 'grey.300',
                                    borderRadius: 1,
                                    '&:hover': { backgroundColor: 'grey.400' },
                                },
                            }}
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
                        sx={{ zIndex: Z_INDEX.popover, '& .MuiPopover-paper': { bgcolor: 'primary.main', color: 'white', borderRadius: 2, px: 2, py: 1, fontSize: '0.875rem' } }}
                    >
                        {t('layout.easterEgg')} · {currentVersion}
                    </Popover>
                </>
            )}
        </Box>
    );
};

export default Layout;
