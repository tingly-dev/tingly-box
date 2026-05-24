import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { getHiddenScenarios } from '@/pages/scenario/AgentOverviewPage';
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
    IconVector,
    IconPhoto,
    IconFlask,
    IconPlayerPlay,
    IconTools,
    IconServer,
} from '@tabler/icons-react';
import { OpenAI, Anthropic, Claude, OpenCode, Xcode, VSCode, Telegram, Feishu, Lark, DingTalk, Weixin, WeCom, Codex, OpenClaw, ClaudeDesktop } from '../components/BrandIcons';
import { SettingsApplications } from '@/components/icons';
import { useFeatureFlags } from '../contexts/FeatureFlagsContext';
import { useProfileContext } from '@/contexts/ProfileContext';
import { isFullEdition } from '@/utils/edition';
import type { ActivityItem, NavItem } from './types';
import { IconAiAgents } from '@tabler/icons-react';
import { useBotPlatformSummary } from './useBotPlatformSummary';

export function useActivityItems(): ActivityItem[] {
    const { t } = useTranslation();
    const { skillUser, skillIde, enableGuardrails, enableMCP } = useFeatureFlags();
    const { profiles } = useProfileContext();
    const botSummary = useBotPlatformSummary(isFullEdition);

    const [hiddenScenarios, setHiddenScenarios] = useState<Set<string>>(() => getHiddenScenarios());
    useEffect(() => {
        const sync = () => setHiddenScenarios(getHiddenScenarios());
        window.addEventListener('scenario-visibility-change', sync);
        window.addEventListener('storage', sync);
        return () => {
            window.removeEventListener('scenario-visibility-change', sync);
            window.removeEventListener('storage', sync);
        };
    }, []);
    const platformSubtitle = (id: string): string | undefined => {
        const s = botSummary[id];
        return s && s.total > 0 ? `active ${s.active} / ${s.total}` : undefined;
    };

    const promptMenuItems = useMemo(() => {
        const items: NavItem[] = [];
        if (skillUser) {
            items.push({
                path: '/prompt/user',
                label: t('layout.userRequest'),
                icon: <IconSend size={20} />,
            });
        }
        if (skillIde) {
            items.push({
                path: '/prompt/skill',
                label: t('layout.skills'),
                icon: <IconBolt size={20} />,
            });
        }
        return items;
    }, [skillUser, skillIde, t]);

    return useMemo(() => {
        const claudeCodeProfiles = profiles['claude_code'] || [];
        const profileNavItems: NavItem[] = claudeCodeProfiles.map(p => ({
            path: `/agent/claude_code/profile/${p.id}`,
            label: t('layout.nav.useClaudeCode', { defaultValue: 'Claude Code' }),
            subtitle: `${p.id} - ${p.name}`,
            icon: <Claude size={20} />,
        }));

        type HideableScenario = { id: string; nav: NavItem };
        const visible = (group: HideableScenario[]): NavItem[] =>
            group.filter(s => !hiddenScenarios.has(s.id)).map(s => s.nav);

        const playgroundTool: NavItem[] = [
            { path: '/agent/playground', label: t('layout.nav.playground', { defaultValue: 'Playground' }), icon: <IconPlayerPlay size={20} /> },
        ];
        const codingTools = visible([
            { id: 'codex', nav: { path: '/agent/codex', label: t('layout.nav.useCodex', { defaultValue: 'Codex' }), icon: <Codex size={20} /> } },
            { id: 'opencode', nav: { path: '/agent/opencode', label: t('layout.nav.useOpenCode', { defaultValue: 'OpenCode' }), icon: <OpenCode size={20} /> } },
            { id: 'xcode', nav: { path: '/agent/xcode', label: t('layout.nav.useXcode', { defaultValue: 'Xcode' }), icon: <Xcode size={20} /> } },
            { id: 'vscode', nav: { path: '/agent/vscode', label: t('layout.nav.useVSCode', { defaultValue: 'VS Code' }), icon: <VSCode size={20} /> } },
            { id: 'claude_desktop', nav: { path: '/agent/claude_desktop', label: t('layout.nav.useClaudeDesktop', { defaultValue: 'Claude Desktop' }), icon: <ClaudeDesktop size={20} /> } },
        ]);
        const sdkTools = visible([
            { id: 'openai', nav: { path: '/agent/openai', label: t('layout.nav.useOpenAI', { defaultValue: 'OpenAI' }), icon: <OpenAI size={20} /> } },
            { id: 'anthropic', nav: { path: '/agent/anthropic', label: t('layout.nav.useAnthropic', { defaultValue: 'Anthropic' }), icon: <Anthropic size={20} /> } },
            { id: 'embed', nav: { path: '/agent/embed', label: t('layout.nav.useEmbed', { defaultValue: 'Embedding' }), icon: <IconVector size={20} /> } },
            { id: 'imagegen', nav: { path: '/agent/imagegen', label: t('layout.nav.useImageGen', { defaultValue: 'Image Gen' }), icon: <IconPhoto size={20} /> } },
        ]);
        const agentTools = visible([
            { id: 'agent', nav: { path: '/agent/agent', label: t('common.openClaw', { defaultValue: 'OpenClaw' }), icon: <OpenClaw size={20} /> } },
        ]);

        const scenarioChildren: NavItem[] = [];
        if (!hiddenScenarios.has('claude_code')) {
            scenarioChildren.push(
                {
                    path: '/agent/claude_code',
                    subtitle: t('layout.default'),
                    label: t('layout.nav.useClaudeCode', { defaultValue: 'Claude Code' }),
                    icon: <Claude size={20} />,
                },
                ...profileNavItems,
                { path: '#add-profile', label: t('layout.addProfile'), icon: <IconPlus size={20} /> },
            );
        }
        const pushGroup = (group: NavItem[]) => {
            if (group.length === 0) return;
            if (scenarioChildren.length > 0) scenarioChildren.push({ type: 'divider' });
            scenarioChildren.push(...group);
        };
        pushGroup(codingTools);
        pushGroup(sdkTools);
        pushGroup(agentTools);
        pushGroup(playgroundTool);

        const items: ActivityItem[] = [
            {
                key: 'scenario',
                icon: <IconAiAgents size={22} />,
                label: t('layout.nav.home'),
                defaultPath: '/agent',
                children: scenarioChildren,
            },
            {
                key: 'dashboard',
                icon: <IconChartBar size={22} />,
                label: t('layout.usage', { defaultValue: 'Usage' }),
                path: '/dashboard/7d',
                defaultPath: '/dashboard/7d',
                children: [
                    { path: '/overview/90d', label: t('layout.heatmap'), icon: <IconGridDots size={20} /> },
                    { type: 'divider' },
                    { path: '/dashboard/today', label: t('layout.today'), icon: <IconCalendarClock size={20} /> },
                    { path: '/dashboard/yesterday', label: t('layout.yesterday'), icon: <IconCalendar size={20} /> },
                    { path: '/dashboard/3d', label: `3 ${t('layout.days')}`, icon: <IconCalendarEvent size={20} /> },
                    { path: '/dashboard/7d', label: `7 ${t('layout.days')}`, icon: <IconCalendarEvent size={20} /> },
                    { path: '/dashboard/30d', label: `30 ${t('layout.days')}`, icon: <IconCalendarEvent size={20} /> },
                    { path: '/dashboard/90d', label: `90 ${t('layout.days')}`, icon: <IconCalendarEvent size={20} /> },
                ],
            },
            ...(isFullEdition && promptMenuItems.length > 0 ? [{
                key: 'prompt' as const,
                icon: <IconBrain size={22} />,
                label: t('common.prompt', { defaultValue: 'Prompt' }),
                defaultPath: promptMenuItems[0]?.path,
                children: promptMenuItems,
            }] as ActivityItem[] : []),
            ...(isFullEdition ? [{
                key: 'remote-control' as const,
                icon: <IconDeviceRemote size={22} />,
                label: t('layout.remote'),
                defaultPath: '/remote-control/weixin',
                children: [
                    { path: '/remote-control/weixin', label: t('layout.platforms.weixin'), icon: <Weixin size={20} />, subtitle: platformSubtitle('weixin') },
                    { path: '/remote-control/wecom', label: t('layout.platforms.wecom'), icon: <WeCom size={20} />, subtitle: platformSubtitle('wecom') },
                    { path: '/remote-control/telegram', label: t('layout.platforms.telegram'), icon: <Telegram size={20} />, subtitle: platformSubtitle('telegram') },
                    { path: '/remote-control/feishu', label: t('layout.platforms.feishu'), icon: <Feishu size={20} />, subtitle: platformSubtitle('feishu') },
                    { path: '/remote-control/lark', label: t('layout.platforms.lark'), icon: <Lark size={20} />, subtitle: platformSubtitle('lark') },
                    { path: '/remote-control/dingtalk', label: t('layout.platforms.dingtalk'), icon: <DingTalk size={20} />, subtitle: platformSubtitle('dingtalk') },
                ] as NavItem[],
            }] as ActivityItem[] : []),
            ...(enableGuardrails ? [{
                key: 'guardrails',
                icon: <IconShield size={22} />,
                label: t('layout.guardrails'),
                defaultPath: '/guardrails',
                children: [
                    { path: '/guardrails', label: t('layout.overview'), icon: <IconShield size={20} /> },
                    { path: '/guardrails/groups', label: t('layout.policyGroups'), icon: <IconLicense size={20} /> },
                    { path: '/guardrails/rules', label: t('layout.policies'), icon: <IconLicense size={20} /> },
                    { path: '/guardrails/credentials', label: t('layout.nav.credential', { defaultValue: 'Credential' }), icon: <IconKey size={20} /> },
                    { path: '/guardrails/history', label: t('layout.guardrailsHistory'), icon: <IconHistory size={20} /> },
                ] as NavItem[],
            }] as ActivityItem[] : []),
            ...(enableMCP ? [{
                key: 'tools' as const,
                icon: <IconTools size={22} />,
                label: 'Tools',
                defaultPath: '/mcp/sources',
                children: [
                    { path: '/mcp/sources', label: 'MCP', icon: <SettingsApplications sx={{ fontSize: 20 }} /> },
                    { path: '/tools/servertool', label: 'Servertool', icon: <IconServer size={20} /> },
                ],
            }] as ActivityItem[] : []),
            {
                key: 'credential',
                icon: <IconLock size={22} />,
                label: t('layout.nav.credential', { defaultValue: 'Credentials' }),
                defaultPath: '/credentials',
                children: [
                    { path: '/credentials', label: t('layout.modelKey'), icon: <IconLock size={20} /> },
                    {
                        path: '/tingly-box-token',
                        label: t('layout.tinglyBox'),
                        icon: <IconKey size={20} />,
                        tooltip: t('layout.tinglyBoxTooltip'),
                    },
                    {
                        path: '/credentials/virtual-models',
                        label: t('layout.virtualModels', { defaultValue: 'Virtual Models' }),
                        icon: <IconFlask size={20} />,
                        tooltip: t('layout.virtualModelsTooltip', {
                            defaultValue: 'Built-in synthetic model providers for onboarding and dry-runs.',
                        }),
                    },
                ],
            },
            {
                key: 'system',
                icon: <IconSettings size={22} />,
                label: t('layout.system'),
                defaultPath: '/system',
                children: [
                    { path: '/system', label: t('layout.general'), icon: <IconSettings size={20} /> },
                    { path: '/access-control', label: t('layout.accessControl'), icon: <IconShield size={20} /> },
                    { path: '/system/experimental', label: t('layout.experimental'), icon: <IconFlask size={20} /> },
                    { path: '/system/logs', label: t('layout.logs'), icon: <IconFileText size={20} /> },
                ],
            },
        ];

        return items;
    }, [t, promptMenuItems, enableGuardrails, enableMCP, profiles, botSummary, hiddenScenarios]);
}
