import { Box } from '@mui/material';
import React from 'react';
import { Outlet, useParams } from 'react-router-dom';
import { ZenActivityBar } from '../../layout/ZenActivityBar';
import { ZenSidebar } from '../../layout/ZenSidebar';
import { useActivityItems } from '../../layout/useActivityItems';
import { useProfileContext } from '../../contexts/ProfileContext';
import { Claude, Codex, OpenCode, Xcode, VSCode, OpenAI, Anthropic, OpenClaw } from '../../components/BrandIcons';
import type { ActivityItem } from '@/layout/types.ts';

/**
 * Zen Layout Page
 *
 * Provides a simplified, focused layout for zen mode.
 * Replaces the standard dual-column ActivityBar + Sidebar layout.
 */
const ZenLayoutPage: React.FC = () => {
    const { agent } = useParams<{ agent: string }>();
    const { profiles } = useProfileContext();
    const activityItems = useActivityItems();

    // Get agent info
    const getAgentInfo = (agentKey: string) => {
        const info: Record<string, { icon: any; label: string; path: string; scenario: string; zenPath: string }> = {
            'claude_code': { icon: <Claude size={22} />, label: 'Claude', path: '/agent/claude_code', scenario: 'claude_code', zenPath: '/zen/claude_code' },
            'codex':       { icon: <Codex size={22} />,   label: 'Codex',     path: '/agent/codex',       scenario: 'codex',       zenPath: '/zen/codex' },
            'opencode':    { icon: <OpenCode size={22} />, label: 'OpenCode',  path: '/agent/opencode',    scenario: 'opencode',    zenPath: '/zen/opencode' },
            'xcode':       { icon: <Xcode size={22} />,   label: 'Xcode',     path: '/agent/xcode',       scenario: 'xcode',       zenPath: '/zen/xcode' },
            'vscode':      { icon: <VSCode size={22} />,  label: 'VS Code',   path: '/agent/vscode',      scenario: 'vscode',      zenPath: '/zen/vscode' },
            'openai':      { icon: <OpenAI size={22} />,  label: 'OpenAI',    path: '/agent/openai',      scenario: 'openai',      zenPath: '/zen/openai' },
            'anthropic':   { icon: <Anthropic size={22} />, label: 'Anthropic', path: '/agent/anthropic', scenario: 'anthropic',   zenPath: '/zen/anthropic' },
            'agent':       { icon: <OpenClaw size={22} />, label: 'OpenClaw', path: '/agent/agent',       scenario: 'agent',       zenPath: '/zen/agent' },
        };
        return info[agentKey || 'claude_code'] || info['claude_code'];
    };

    const agentInfo = getAgentInfo(agent || 'claude_code');

    // Build sidebar items: agent page + profiles + divider + add profile
    const sidebarItems: ActivityItem['children'] = [
        { path: agentInfo.path, label: agentInfo.label, icon: agentInfo.icon, subtitle: 'default' },
    ];

    // Add profiles for agents that support them
    const agentProfiles = profiles[agentInfo.scenario] || [];
    agentProfiles.forEach(profile => {
        sidebarItems.push({
            path: `${agentInfo.path}/profile/${profile.id}`,
            label: profile.name,
            icon: agentInfo.icon,
            subtitle: profile.id,
        });
    });

    // Add divider and add profile button
    sidebarItems.push({ type: 'divider' });
    sidebarItems.push({
        path: '#add-profile',
        label: 'Add Profile',
        icon: <span>+</span>,
    });

    return (
        <Box sx={{ display: 'flex', height: '100vh', overflow: 'hidden' }}>
            {/* Zen Activity Bar */}
            <ZenActivityBar
                agent={agent || 'claude-code'}
                activityItems={activityItems}
            />

            {/* Zen Sidebar */}
            <ZenSidebar
                sidebarItems={sidebarItems}
                activeActivityLabel={agentInfo.label}
            />

            {/* Main Content */}
            <Box
                component="main"
                sx={{
                    flexGrow: 1,
                    height: '100vh',
                    display: 'flex',
                    flexDirection: 'column',
                    overflowX: 'hidden',
                }}
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
                    <Outlet />
                </Box>
            </Box>
        </Box>
    );
};

export default ZenLayoutPage;
