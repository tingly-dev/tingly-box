import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import {
    AutoAwesome,
    BarChart as BarChartIcon,
    CalendarToday as CalendarIcon,
    ChatBubble,
    Code as CodeIcon,
    DateRange as DateRangeIcon,
    GridOn as GridOnIcon,
    Add as AddIcon,
    ListAlt as LogsIcon,
    Psychology as PromptIcon,
    Lan as RemoteIcon,
    Bolt as SkillIcon,
    Settings as SystemIcon,
    Today as TodayIcon,
    Send as UserPromptIcon,
    Rule,
    History as HistoryIcon,
    VpnKey as VpnKeyIcon,
    Security as AccessControlIcon,
} from '@mui/icons-material';
import LockIcon from '@mui/icons-material/Lock';
import { OpenAI, Anthropic, Claude, OpenCode, Xcode, VSCode, Telegram, Feishu, Lark, DingTalk, Weixin } from '../components/BrandIcons';
import { useFeatureFlags } from '../contexts/FeatureFlagsContext';
import { useProfileContext } from '@/contexts/ProfileContext';
import { isFullEdition } from '@/utils/edition';
import type { ActivityItem, NavDivider, NavItem } from './types';

export function useActivityItems(): ActivityItem[] {
    const { t } = useTranslation();
    const { skillUser, skillIde, enableGuardrails } = useFeatureFlags();
    const { profiles } = useProfileContext();

    const promptMenuItems = useMemo(() => {
        const items: NavItem[] = [];
        if (skillUser) {
            items.push({
                path: '/prompt/user',
                label: 'User Request',
                icon: <UserPromptIcon sx={{ fontSize: 20 }} />,
            });
        }
        if (skillIde) {
            items.push({
                path: '/prompt/skill',
                label: 'Skills',
                icon: <SkillIcon sx={{ fontSize: 20 }} />,
            });
        }
        return items;
    }, [skillUser, skillIde]);

    return useMemo(() => {
        const claudeCodeProfiles = profiles['claude_code'] || [];
        const profileNavItems: NavItem[] = claudeCodeProfiles.map(p => ({
            path: `/use-claude-code/profile/${p.id}`,
            label: 'Claude Code',
            subtitle: `${p.id} - ${p.name}`,
            icon: <Claude size={20} />,
        }));

        const items: ActivityItem[] = [
            {
                key: 'dashboard',
                icon: <BarChartIcon sx={{ fontSize: 22 }} />,
                label: 'Dashboard',
                path: '/dashboard/7d',
                children: [
                    { path: '/overview/90d', label: 'Heatmap', icon: <GridOnIcon sx={{ fontSize: 20 }} /> },
                    { type: 'divider' },
                    { path: '/dashboard/today', label: 'Today', icon: <TodayIcon sx={{ fontSize: 20 }} /> },
                    { path: '/dashboard/yesterday', label: 'Yesterday', icon: <CalendarIcon sx={{ fontSize: 20 }} /> },
                    { path: '/dashboard/3d', label: '3 Days', icon: <DateRangeIcon sx={{ fontSize: 20 }} /> },
                    { path: '/dashboard/7d', label: '7 Days', icon: <DateRangeIcon sx={{ fontSize: 20 }} /> },
                    { path: '/dashboard/30d', label: '30 Days', icon: <DateRangeIcon sx={{ fontSize: 20 }} /> },
                    { path: '/dashboard/90d', label: '90 Days', icon: <DateRangeIcon sx={{ fontSize: 20 }} /> },
                ],
            },
            {
                key: 'scenario',
                icon: <CodeIcon sx={{ fontSize: 22 }} />,
                label: t('layout.nav.home'),
                children: [
                    {
                        path: '/use-claude-code',
                        subtitle: 'default',
                        label: t('layout.nav.useClaudeCode', { defaultValue: 'Claude Code' }),
                        icon: <Claude size={20} />,
                    },
                    ...profileNavItems,
                    { path: '#add-profile', label: 'Add Profile', icon: <AddIcon sx={{ fontSize: 20 }} /> },
                    { type: 'divider' },
                    { path: '/use-codex', label: t('layout.nav.useCodex', { defaultValue: 'Codex' }), icon: <OpenAI size={20} /> },
                    { path: '/use-opencode', label: t('layout.nav.useOpenCode', { defaultValue: 'OpenCode' }), icon: <OpenCode size={20} /> },
                    { path: '/use-xcode', label: t('layout.nav.useXcode', { defaultValue: 'Xcode' }), icon: <Xcode size={20} /> },
                    { path: '/use-vscode', label: t('layout.nav.useVSCode', { defaultValue: 'VS Code' }), icon: <VSCode size={20} /> },
                    { type: 'divider' },
                    { path: '/use-openai', label: t('layout.nav.useOpenAI', { defaultValue: 'OpenAI' }), icon: <OpenAI size={20} /> },
                    { path: '/use-anthropic', label: t('layout.nav.useAnthropic', { defaultValue: 'Anthropic' }), icon: <Anthropic size={20} /> },
                    { type: 'divider' },
                    { path: '/use-agent', label: 'OpenClaw', icon: <AutoAwesome sx={{ fontSize: 20 }} /> },
                ],
            },
            ...(isFullEdition && promptMenuItems.length > 0 ? [{
                key: 'prompt' as const,
                icon: <PromptIcon sx={{ fontSize: 22 }} />,
                label: 'Prompt',
                children: promptMenuItems,
            }] as ActivityItem[] : []),
            ...(isFullEdition ? [{
                key: 'remote-control' as const,
                icon: <RemoteIcon sx={{ fontSize: 22 }} />,
                label: 'Remote',
                children: [
                    { path: '/remote-control', label: 'Overview', icon: <ChatBubble sx={{ fontSize: 20 }} /> },
                    { type: 'divider' } as NavDivider,
                    { path: '/remote-control/weixin', label: 'Weixin', icon: <Weixin size={20} /> },
                    { path: '/remote-control/telegram', label: 'Telegram', icon: <Telegram size={20} /> },
                    { path: '/remote-control/feishu', label: 'Feishu', icon: <Feishu size={20} /> },
                    { path: '/remote-control/lark', label: 'Lark', icon: <Lark size={20} /> },
                    { path: '/remote-control/dingtalk', label: 'DingTalk', icon: <DingTalk size={20} /> },
                ] as NavItem[],
            }] as ActivityItem[] : []),
            ...(enableGuardrails ? [{
                key: 'guardrails',
                icon: <AccessControlIcon sx={{ fontSize: 22 }} />,
                label: 'Guardrails',
                children: [
                    { path: '/guardrails', label: 'Overview', icon: <AccessControlIcon sx={{ fontSize: 20 }} /> },
                    { path: '/guardrails/groups', label: 'Policy Groups', icon: <Rule sx={{ fontSize: 20 }} /> },
                    { path: '/guardrails/rules', label: 'Policies', icon: <Rule sx={{ fontSize: 20 }} /> },
                    { path: '/guardrails/credentials', label: 'Credentials', icon: <VpnKeyIcon sx={{ fontSize: 20 }} /> },
                    { path: '/guardrails/history', label: 'History', icon: <HistoryIcon sx={{ fontSize: 20 }} /> },
                ] as NavItem[],
            }] as ActivityItem[] : []),
            {
                key: 'credential',
                icon: <LockIcon sx={{ fontSize: 22 }} />,
                label: t('layout.nav.credential', { defaultValue: 'Credentials' }),
                children: [
                    { path: '/credentials', label: 'Model Key', icon: <LockIcon sx={{ fontSize: 20 }} /> },
                ],
            },
            {
                key: 'system',
                icon: <SystemIcon sx={{ fontSize: 22 }} />,
                label: 'System',
                children: [
                    { path: '/access-control', label: 'Access Control', icon: <AccessControlIcon sx={{ fontSize: 20 }} /> },
                    { path: '/system', label: 'Status', icon: <SystemIcon sx={{ fontSize: 20 }} /> },
                    { path: '/system/logs', label: 'Logs', icon: <LogsIcon sx={{ fontSize: 20 }} /> },
                ],
            },
        ];

        return items;
    }, [t, promptMenuItems, enableGuardrails, profiles]);
}
