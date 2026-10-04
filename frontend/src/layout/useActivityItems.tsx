import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { SCENARIOS, getHiddenScenarios } from '@/pages/scenario/scenarioRegistry';
import { OpenAI, Anthropic, Claude, Cursor, DeepSeek, OpenCode, Pi, Xcode, VSCode, Codex, ClaudeDesktop } from '../components/BrandIcons';
import {
    SettingsApplications,
    BarChart as IconChartBar,
    EventNote as IconCalendarClock,
    CalendarToday as IconCalendar,
    Event as IconCalendarEvent,
    Add as IconPlus,
    TextSnippet as IconFileText,
    Psychology as IconBrain,
    RemoteControl as IconRemote,
    Robot as IconRobot,
    Terminal as IconTerminal,
    Bell as IconBell,
    Bolt as IconBolt,
    Settings as IconSettings,
    Send as IconSend,
    License as IconLicense,
    History as IconHistory,
    Key as IconKey,
    Shield as IconShield,
    Lock as IconLock,
    Vector as IconVector,
    Photo as IconPhoto,
    Palette as IconPalette,
    Cable as IconPlug,
    Users as IconUsers,
    Science as IconFlask,
    Handyman as IconTools,
    Server as IconServer,
    AiAgents as IconAiAgents,
    Extension as IconExtension,
    Code as IconCode,
    TestPipe as IconTestPipe,
} from '@/components/icons';
import { useFeatureFlags } from '../contexts/FeatureFlagsContext';
import { useProfileContext } from '@/contexts/ProfileContext';
import { useTeamContext } from '@/contexts/TeamContext';
import { orderTeams, teamPath } from '@/utils/team';
import type { ActivityItem, NavItem, NavItemBase } from './types';
import { useBotPlatformSummary } from './useBotPlatformSummary';

// The usage charts' URLs, one per time range (/dashboard/today, /dashboard/7d, …).
const DASHBOARD_RANGE_PATH = /^\/dashboard\/(today|yesterday|3d|7d|30d|90d)$/;

export function useActivityItems(): ActivityItem[] {
    const { t } = useTranslation();
    const { skillUser, skillIde, enableGuardrails, enableBench, enableDesk } = useFeatureFlags();
    const { profiles } = useProfileContext();
    const { teams } = useTeamContext();
    const botSummary = useBotPlatformSummary();

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
    // Aggregate across every platform for the Bots row's count — the
    // per-platform breakdown lives on the Bots page itself, not in the nav
    // (see .design/bot-arch.md §10). A trailing "2/3" on the label's line
    // rather than a second line, so the row matches its one-line siblings.
    const botOverviewCount = useMemo(() => {
        const totals = Object.values(botSummary).reduce(
            (acc, s) => ({ active: acc.active + s.active, total: acc.total + s.total }),
            { active: 0, total: 0 }
        );
        return totals.total > 0 ? {
            value: `${totals.active}/${totals.total}`,
            title: t('layout.botsRunning', { defaultValue: '{{active}} of {{total}} bots running', active: totals.active, total: totals.total }),
        } : undefined;
    }, [botSummary, t]);

    const promptMenuItems = useMemo(() => {
        const items: NavItem[] = [];
        if (skillUser) {
            items.push({
                path: '/prompt/user',
                label: t('layout.userRequest'),
                icon: <IconSend sx={{ fontSize: 20 }} />,
            });
        }
        if (skillIde) {
            items.push({
                path: '/prompt/skill',
                label: t('layout.skills'),
                icon: <IconBolt sx={{ fontSize: 20 }} />,
            });
        }
        return items;
    }, [skillUser, skillIde, t]);

    return useMemo(() => {
        const claudeCodeProfiles = profiles['claude_code'] || [];
        const profileNavItems: NavItem[] = claudeCodeProfiles.map(p => ({
            path: `/agent/claude_code/profile/${p.id}`,
            // The profile is the subject of its row (ux-principles #9): its
            // own name leads; "Claude Code" + id is the caption. Rows used to
            // all read "Claude Code" with the name in grey underneath.
            label: p.name || p.id,
            subtitle: `${t('layout.nav.useClaudeCode', { defaultValue: 'Claude Code' })} · ${p.id}`,
            icon: <Claude size={20} />,
        }));
        const orderedTeams = orderTeams(teams);
        const teamNavItems: NavItem[] = [
            // Before the Team list loads, keep a placeholder row for the default Team.
            ...(orderedTeams[0]?.is_default ? [] : [{
                path: '/agent/team',
                label: t('layout.nav.useTeam', {defaultValue: 'Team'}),
                subtitle: t('layout.default'),
                icon: <IconUsers sx={{fontSize: 20}} />,
            }]),
            ...orderedTeams.map(team => ({
                path: teamPath(team),
                label: team.name || team.slug,
                subtitle: team.slug,
                icon: <IconUsers sx={{fontSize: 20}} />,
            })),
            {path: '#add-team', label: t('layout.addTeam'), icon: <IconPlus sx={{fontSize: 20}} />},
            // Overview of every Team's keys, grouped by Team — last, after the
            // Team list, since it spans all of them.
            {type: 'divider'},
            {
                path: '/agent/team/keys',
                label: t('layout.teamKeys', {defaultValue: 'Team Keys'}),
                icon: <IconKey sx={{fontSize: 20}} />,
                tooltip: t('layout.teamKeysTooltip'),
            },
        ];

        // Hidden agents stay in the list, flagged: the sidebar's edit mode
        // shows them (to un-hide), and drops them otherwise (see
        // withoutHidden in Layout).
        type HideableScenario = { id: string; nav: NavItemBase };
        const hintOf = (id: string) => {
            const s = SCENARIOS.find(x => x.id === id);
            return s ? t(s.descKey) : undefined;
        };
        const flagged = (group: HideableScenario[]): NavItem[] =>
            group.map(s => ({ ...s.nav, hideId: s.id, hidden: hiddenScenarios.has(s.id), hint: hintOf(s.id) }));

        const teamActivityItem: ActivityItem = {
            key: 'team',
            icon: <IconUsers sx={{ fontSize: 22 }} />,
            label: t('layout.team', { defaultValue: 'Team' }),
            defaultPath: '/agent/team',
            children: teamNavItems,
        };

        const codingTools = flagged([
            // Claude Desktop leads so all Claude-branded scenarios stay grouped
            // at the front, right after the Claude Code block.
            { id: 'claude_desktop', nav: { path: '/agent/claude_desktop', label: t('layout.nav.useClaudeDesktop', { defaultValue: 'Claude Desktop' }), icon: <ClaudeDesktop size={20} /> } },
            { id: 'codex', nav: { path: '/agent/codex', label: t('layout.nav.useCodex', { defaultValue: 'Codex' }), icon: <Codex size={20} /> } },
            { id: 'opencode', nav: { path: '/agent/opencode', label: t('layout.nav.useOpenCode', { defaultValue: 'OpenCode' }), icon: <OpenCode size={20} /> } },
            { id: 'pi', nav: { path: '/agent/pi', label: t('layout.nav.usePi', { defaultValue: 'Pi' }), icon: <Pi size={20} /> } },
            { id: 'dsh', nav: { path: '/agent/dsh', label: t('layout.nav.useDsh', { defaultValue: 'DeepSeek' }), icon: <DeepSeek size={20} /> } },
            { id: 'xcode', nav: { path: '/agent/xcode', label: t('layout.nav.useXcode', { defaultValue: 'Xcode' }), icon: <Xcode size={20} /> } },
            { id: 'vscode', nav: { path: '/agent/vscode', label: t('layout.nav.useVSCode', { defaultValue: 'VS Code' }), icon: <VSCode size={20} /> } },
            { id: 'cursor', nav: { path: '/agent/cursor', label: t('layout.nav.useCursor', { defaultValue: 'Cursor' }), icon: <Cursor size={20} /> } },
            // "custom" (bring-your-own-request-model) closes out the coding
            // tools group, right after the named integrations it's a fallback for.
            { id: 'custom', nav: { path: '/agent/custom', label: t('layout.nav.useCustom', { defaultValue: 'Custom' }), icon: <IconExtension sx={{ fontSize: 20 }} /> } },
        ]);
        const sdkTools = flagged([
            { id: 'openai', nav: { path: '/agent/openai', label: t('layout.nav.useOpenAI', { defaultValue: 'OpenAI' }), icon: <OpenAI size={20} /> } },
            { id: 'anthropic', nav: { path: '/agent/anthropic', label: t('layout.nav.useAnthropic', { defaultValue: 'Anthropic' }), icon: <Anthropic size={20} /> } },
            { id: 'embed', nav: { path: '/agent/embed', label: t('layout.nav.useEmbed', { defaultValue: 'Embedding' }), icon: <IconVector sx={{ fontSize: 20 }} /> } },
        ]);

        const scenarioChildren: NavItem[] = [];
        const claudeCodeHidden = hiddenScenarios.has('claude_code');
        scenarioChildren.push({
            path: '/agent/claude_code',
            subtitle: t('layout.default'),
            label: t('layout.nav.useClaudeCode', { defaultValue: 'Claude Code' }),
            icon: <Claude size={20} />,
            hideId: 'claude_code',
            hidden: claudeCodeHidden,
            hint: hintOf('claude_code'),
        });
        // Profiles and "Add Profile" belong to Claude Code: hidden with it.
        if (!claudeCodeHidden) {
            scenarioChildren.push(
                ...profileNavItems,
                { path: '#add-profile', label: t('layout.addProfile'), icon: <IconPlus sx={{ fontSize: 20 }} /> },
            );
        }
        const pushGroup = (group: NavItem[]) => {
            if (group.length === 0) return;
            if (scenarioChildren.length > 0) scenarioChildren.push({ type: 'divider' });
            scenarioChildren.push(...group);
        };
        pushGroup(codingTools);
        pushGroup(sdkTools);

        // Rail order, top to bottom: usage → use → power-ups → verify →
        // configuration. Dashboard leads, but it is not the landing page — OnboardingGate still opens /agent,
        // and Layout falls back to the 'scenario' activity, independent of
        // this order.
        const items: ActivityItem[] = [
            {
                key: 'dashboard',
                icon: <IconChartBar sx={{ fontSize: 22 }} />,
                label: t('layout.dashboard', { defaultValue: 'Dashboard' }),
                defaultPath: '/dashboard/today',
                children: [
                    // One row for the usage charts; the time range is a filter
                    // on that page (every /dashboard/<range> URL still works).
                    { path: '/dashboard/today', label: t('layout.usage', { defaultValue: 'Usage' }), icon: <IconChartBar sx={{ fontSize: 20 }} />, match: (p) => DASHBOARD_RANGE_PATH.test(p) },
                    { path: '/dashboard/users', label: t('layout.userUsage', { defaultValue: 'Team usage' }), icon: <IconUsers sx={{ fontSize: 20 }} /> },
                    { path: '/dashboard/quota-history', label: t('layout.quotaHistory', { defaultValue: 'Quota history' }), icon: <IconHistory sx={{ fontSize: 20 }} /> },
                ],
            },
            // ── Use: agent ⇄ model, team, image ──
            {
                key: 'scenario',
                icon: <IconAiAgents sx={{ fontSize: 22 }} />,
                label: t('layout.nav.home'),
                defaultPath: '/agent',
                children: scenarioChildren,
            },
            // Shown/hidden by its switch in the rail's Power-ups menu (the
            // 'team' entry of the hidden-scenario set), like Image below.
            ...(!hiddenScenarios.has('team') ? [teamActivityItem] : []),
            // Image — the playground outgrew a card on the scenario page
            // (.design/image-layout.md). Shown/hidden together with the
            // imagegen scenario, so there is one switch for "I use images".
            ...(!hiddenScenarios.has('imagegen') ? [{
                key: 'image',
                icon: <IconPhoto sx={{ fontSize: 22 }} />,
                label: t('layout.image', { defaultValue: 'Image' }),
                defaultPath: '/image/playground',
                children: [
                    { path: '/image/playground', label: t('layout.imagePlayground', { defaultValue: 'Playground' }), icon: <IconPalette sx={{ fontSize: 20 }} /> },
                    { path: '/image/api', label: t('layout.nav.useImageGen', { defaultValue: 'Image API' }), icon: <IconPlug sx={{ fontSize: 20 }} /> },
                ],
            }] as ActivityItem[] : []),
            // ── Power-ups: what extends an agent beyond plain model routing ──
            // Remote — one rail icon for the whole domain, named after the
            // product pillar (remote control) rather than the implementation
            // ("Bots"). Bot is the front door (every connected bot, every
            // platform, credentials live there); Remote Control and Notify are
            // the purposes mounted onto those bots. New purposes append here
            // as new rows — the rail icon never grows. Desk (no bot, browser
            // only) closes the list behind its own divider. See bot-arch.md §10.
            // (key stays 'bots' — internal id, not user-visible.)
            // Hidden via the Remote switch in the rail's Power-ups menu (same hidden set as
            // Team/Image) — hides the rail item only, bots keep running.
            ...(!hiddenScenarios.has('remote') ? [{
                key: 'bots' as const,
                icon: <IconRemote sx={{ fontSize: 22 }} />,
                label: t('layout.remote'),
                defaultPath: '/bots/overview',
                children: [
                    { path: '/bots/overview', label: t('layout.bots', { defaultValue: 'Bots' }), icon: <IconRobot sx={{ fontSize: 20 }} />, count: botOverviewCount },
                    { type: 'divider' },
                    { path: '/remote-agent', label: t('layout.remoteControl', { defaultValue: 'Remote Control' }), icon: <IconTerminal sx={{ fontSize: 20 }} />, match: (p) => p.startsWith('/remote-agent') },
                    { path: '/notify', label: t('layout.notify', { defaultValue: 'IM Notify' }), icon: <IconBell sx={{ fontSize: 20 }} /> },
                    // Desk is remote work WITHOUT a bot (Claude Code driven
                    // from the browser), so it sits in its own group after
                    // the bot purposes rather than between them.
                    ...(enableDesk ? [
                        { type: 'divider' as const },
                        { path: '/desk', label: t('layout.desk', { defaultValue: 'Desk' }), icon: <IconCode sx={{ fontSize: 20 }} />, match: (p: string) => p.startsWith('/desk') },
                    ] : []),
                ] as NavItem[],
            }] as ActivityItem[] : []),
            ...(promptMenuItems.length > 0 ? [{
                key: 'prompt' as const,
                icon: <IconBrain sx={{ fontSize: 22 }} />,
                label: t('common.prompt', { defaultValue: 'Prompt' }),
                defaultPath: promptMenuItems.find((item): item is NavItemBase => !('type' in item))?.path,
                children: promptMenuItems,
            }] as ActivityItem[] : []),
            ...[{
                key: 'tools' as const,
                icon: <IconTools sx={{ fontSize: 22 }} />,
                label: t('layout.tools', { defaultValue: 'Tools' }),
                defaultPath: '/mcp',
                children: [
                    { path: '/mcp', label: t('mcp.workspace.overviewNav', { defaultValue: 'MCP' }), icon: <SettingsApplications sx={{ fontSize: 20 }} />, match: (path: string) => ['/mcp', '/mcp/routes', '/mcp/sources'].includes(path) },
                    { path: '/mcp/tools', label: 'Tool', icon: <IconTerminal sx={{ fontSize: 20 }} />, match: (path: string) => ['/mcp/tools', '/mcp/clients', '/mcp/local-mode'].includes(path) },
                    { path: '/mcp/server-tools', label: 'Server Tool', icon: <IconServer sx={{ fontSize: 20 }} />, match: (path: string) => ['/mcp/server-tools', '/tools/servertool'].includes(path) },
                ],
            }] as ActivityItem[],
            ...(enableGuardrails ? [{
                key: 'guardrails',
                icon: <IconShield sx={{ fontSize: 22 }} />,
                label: t('layout.guardrails'),
                defaultPath: '/guardrails',
                children: [
                    { path: '/guardrails', label: t('layout.overview'), icon: <IconShield sx={{ fontSize: 20 }} /> },
                    { path: '/guardrails/groups', label: t('layout.policyGroups'), icon: <IconLicense sx={{ fontSize: 20 }} /> },
                    { path: '/guardrails/rules', label: t('layout.policies'), icon: <IconLicense sx={{ fontSize: 20 }} /> },
                    { path: '/guardrails/credentials', label: t('layout.protectedCredentials', { defaultValue: 'Secrets' }), icon: <IconKey sx={{ fontSize: 20 }} /> },
                    { path: '/guardrails/history', label: t('layout.guardrailsHistory'), icon: <IconHistory sx={{ fontSize: 20 }} /> },
                ] as NavItem[],
            }] as ActivityItem[] : []),
            // Bench — the customizable end-to-end test workbench
            // (.design/bench.md); a single page, so no sidebar children.
            // Experimental, off by default (system.experimental "bench"
            // flag) — same gating as Guardrails/MCP above. Sits after the
            // power-ups it verifies rather than next to Dashboard, so an
            // opt-in lab tool never pushes Agent down the rail.
            ...(enableBench ? [{
                key: 'bench' as const,
                icon: <IconTestPipe sx={{ fontSize: 22 }} />,
                label: t('layout.bench', { defaultValue: 'Bench' }),
                path: '/bench',
                defaultPath: '/bench',
            }] as ActivityItem[] : []),
            // ── Configuration: set once, rarely revisited ──
            {
                key: 'credential',
                icon: <IconLock sx={{ fontSize: 22 }} />,
                label: t('layout.nav.credential', { defaultValue: 'Credentials' }),
                defaultPath: '/credentials',
                children: [
                    { path: '/credentials', label: t('layout.credentials', { defaultValue: 'Credentials' }), icon: <IconLock sx={{ fontSize: 20 }} /> },
                    {
                        path: '/credentials/virtual-models',
                        // Abbreviated here only — the sidebar is the tight spot;
                        // the page itself (VirtualModelsPage) keeps the full
                        // "Virtual Models" title via the shared layout.virtualModels key.
                        label: t('layout.virtualModelsNavLabel', { defaultValue: 'VModel' }),
                        icon: <IconFlask sx={{ fontSize: 20 }} />,
                        tooltip: t('layout.virtualModelsTooltip', {
                            defaultValue: 'Built-in synthetic model providers for onboarding and dry-runs.',
                        }),
                    },
                ],
            },
            {
                key: 'system',
                icon: <IconSettings sx={{ fontSize: 22 }} />,
                label: t('layout.system'),
                defaultPath: '/system',
                children: [
                    { path: '/system', label: t('layout.general'), icon: <IconSettings sx={{ fontSize: 20 }} /> },
                    { path: '/access-control', label: t('layout.accessControl'), icon: <IconShield sx={{ fontSize: 20 }} /> },
                    { path: '/system/experimental', label: t('layout.experimental'), icon: <IconFlask sx={{ fontSize: 20 }} /> },
                    { path: '/system/develop', label: t('layout.develop'), icon: <IconCode sx={{ fontSize: 20 }} /> },
                    { path: '/system/logs', label: t('layout.logs'), icon: <IconFileText sx={{ fontSize: 20 }} /> },
                ],
            },
        ];

        return items;
    }, [t, promptMenuItems, enableGuardrails, enableBench, enableDesk, profiles, teams, botSummary, hiddenScenarios]);
}
