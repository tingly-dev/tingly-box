import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import {
    IconChartBar,
    IconGridDots,
    IconCalendarClock,
    IconCalendar,
    IconCalendarEvent,
    IconPlus,
    IconFileText,
    IconBrain,
    IconDeviceRemote,
    IconBolt,
    IconSettings,
    IconSend,
    IconLicense,
    IconHistory,
    IconKey,
    IconShield,
    IconLock,
    IconSparkles,
    IconMessageCircle,
} from '@tabler/icons-react';
import { OpenAI, Anthropic, Claude, OpenCode, Xcode, VSCode, Telegram, Feishu, Lark, DingTalk, Weixin, Codex, OpenClaw } from '../components/BrandIcons';
import { useFeatureFlags } from '../contexts/FeatureFlagsContext';
import { useProfileContext } from '@/contexts/ProfileContext';
import { isFullEdition } from '@/utils/edition';
import type { ActivityItem, NavDivider, NavItem } from './types';
import { IconAiAgents } from '@tabler/icons-react';

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
                icon: <IconSend size={20} />,
            });
        }
        if (skillIde) {
            items.push({
                path: '/prompt/skill',
                label: 'Skills',
                icon: <IconBolt size={20} />,
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
                icon: <IconChartBar size={22} />,
                label: 'Dashboard',
                path: '/dashboard/7d',
                children: [
                    { path: '/overview/90d', label: 'Heatmap', icon: <IconGridDots size={20} /> },
                    { type: 'divider' },
                    { path: '/dashboard/today', label: 'Today', icon: <IconCalendarClock size={20} /> },
                    { path: '/dashboard/yesterday', label: 'Yesterday', icon: <IconCalendar size={20} /> },
                    { path: '/dashboard/3d', label: '3 Days', icon: <IconCalendarEvent size={20} /> },
                    { path: '/dashboard/7d', label: '7 Days', icon: <IconCalendarEvent size={20} /> },
                    { path: '/dashboard/30d', label: '30 Days', icon: <IconCalendarEvent size={20} /> },
                    { path: '/dashboard/90d', label: '90 Days', icon: <IconCalendarEvent size={20} /> },
                ],
            },
            {
                key: 'scenario',
                icon: <IconAiAgents size={22} />,
                label: t('layout.nav.home'),
                children: [
                    {
                        path: '/use-claude-code',
                        subtitle: 'default',
                        label: t('layout.nav.useClaudeCode', { defaultValue: 'Claude Code' }),
                        icon: <Claude size={20} />,
                    },
                    ...profileNavItems,
                    { path: '#add-profile', label: 'Add Profile', icon: <IconPlus size={20} /> },
                    { type: 'divider' },
                    { path: '/use-codex', label: t('layout.nav.useCodex', { defaultValue: 'Codex' }), icon: <Codex size={20} /> },
                    { path: '/use-opencode', label: t('layout.nav.useOpenCode', { defaultValue: 'OpenCode' }), icon: <OpenCode size={20} /> },
                    { path: '/use-xcode', label: t('layout.nav.useXcode', { defaultValue: 'Xcode' }), icon: <Xcode size={20} /> },
                    { path: '/use-vscode', label: t('layout.nav.useVSCode', { defaultValue: 'VS Code' }), icon: <VSCode size={20} /> },
                    { type: 'divider' },
                    { path: '/use-openai', label: t('layout.nav.useOpenAI', { defaultValue: 'OpenAI' }), icon: <OpenAI size={20} /> },
                    { path: '/use-anthropic', label: t('layout.nav.useAnthropic', { defaultValue: 'Anthropic' }), icon: <Anthropic size={20} /> },
                    { type: 'divider' },
                    { path: '/use-agent', label: 'OpenClaw', icon: <OpenClaw size={20} /> },
                ],
            },
            ...(isFullEdition && promptMenuItems.length > 0 ? [{
                key: 'prompt' as const,
                icon: <IconBrain size={22} />,
                label: 'Prompt',
                children: promptMenuItems,
            }] as ActivityItem[] : []),
            ...(isFullEdition ? [{
                key: 'remote-control' as const,
                icon: <IconDeviceRemote size={22} />,
                label: 'Remote',
                children: [
                    { path: '/remote-control', label: 'Overview', icon: <IconMessageCircle size={20} /> },
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
                icon: <IconShield size={22} />,
                label: 'Guardrails',
                children: [
                    { path: '/guardrails', label: 'Overview', icon: <IconShield size={20} /> },
                    { path: '/guardrails/groups', label: 'Policy Groups', icon: <IconLicense size={20} /> },
                    { path: '/guardrails/rules', label: 'Policies', icon: <IconLicense size={20} /> },
                    { path: '/guardrails/credentials', label: 'Credentials', icon: <IconKey size={20} /> },
                    { path: '/guardrails/history', label: 'History', icon: <IconHistory size={20} /> },
                ] as NavItem[],
            }] as ActivityItem[] : []),
            {
                key: 'credential',
                icon: <IconLock size={22} />,
                label: t('layout.nav.credential', { defaultValue: 'Credentials' }),
                children: [
                    { path: '/credentials', label: 'Model Key', icon: <IconLock size={20} /> },
                ],
            },
            {
                key: 'system',
                icon: <IconSettings size={22} />,
                label: 'System',
                children: [
                    { path: '/access-control', label: 'Access Control', icon: <IconShield size={20} /> },
                    { path: '/system', label: 'Status', icon: <IconSettings size={20} /> },
                    { path: '/system/logs', label: 'Logs', icon: <IconFileText size={20} /> },
                ],
            },
        ];

        return items;
    }, [t, promptMenuItems, enableGuardrails, profiles]);
}
