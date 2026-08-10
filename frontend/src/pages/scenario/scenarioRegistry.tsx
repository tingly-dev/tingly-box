// Scenario metadata + hidden-scenario state, split out of AgentOverviewPage.tsx
// so that nav-level consumers (layout/useActivityItems.tsx,
// components/dashboard/AgentQuickNav.tsx) don't have to statically import the
// whole overview page — and its PageLayout/PageHeader dependency chain — just
// to read this list. AgentOverviewPage.tsx itself imports from here too.
import { useCallback, useEffect, useState } from 'react';
import {
    Photo as IconPhoto,
    Users as IconUsers,
    Vector as IconVector,
} from '@/components/icons';
import {
    Anthropic,
    Claude,
    ClaudeDesktop,
    Codex,
    OpenAI,
    OpenClaw,
    OpenCode,
    Pi,
    VSCode,
    Xcode,
} from '@/components/BrandIcons';

export interface ScenarioDescriptor {
    id: string;
    labelKey: string;
    descKey: string;
    path: string;
    icon: (size: number) => React.ReactNode;
    /** Hideable from the sidebar. Claude Code is always shown (anchors profiles). */
    hideable: boolean;
}

export const SCENARIOS: ScenarioDescriptor[] = [
    {
        id: 'claude_code',
        labelKey: 'layout.nav.useClaudeCode',
        descKey: 'scenarioOverview.descriptions.claude_code',
        path: '/agent/claude_code',
        icon: (size) => <Claude size={size} />,
        hideable: true,
    },
    {
        id: 'claude_desktop',
        labelKey: 'layout.nav.useClaudeDesktop',
        descKey: 'scenarioOverview.descriptions.claude_desktop',
        path: '/agent/claude_desktop',
        icon: (size) => <ClaudeDesktop size={size} />,
        hideable: true,
    },
    {
        id: 'codex',
        labelKey: 'layout.nav.useCodex',
        descKey: 'scenarioOverview.descriptions.codex',
        path: '/agent/codex',
        icon: (size) => <Codex size={size} />,
        hideable: true,
    },
    {
        id: 'opencode',
        labelKey: 'layout.nav.useOpenCode',
        descKey: 'scenarioOverview.descriptions.opencode',
        path: '/agent/opencode',
        icon: (size) => <OpenCode size={size} />,
        hideable: true,
    },
    {
        id: 'pi',
        labelKey: 'layout.nav.usePi',
        descKey: 'scenarioOverview.descriptions.pi',
        path: '/agent/pi',
        icon: (size) => <Pi size={size} />,
        hideable: true,
    },
    {
        id: 'xcode',
        labelKey: 'layout.nav.useXcode',
        descKey: 'scenarioOverview.descriptions.xcode',
        path: '/agent/xcode',
        icon: (size) => <Xcode size={size} />,
        hideable: true,
    },
    {
        id: 'vscode',
        labelKey: 'layout.nav.useVSCode',
        descKey: 'scenarioOverview.descriptions.vscode',
        path: '/agent/vscode',
        icon: (size) => <VSCode size={size} />,
        hideable: true,
    },
    {
        id: 'openai',
        labelKey: 'layout.nav.useOpenAI',
        descKey: 'scenarioOverview.descriptions.openai',
        path: '/agent/openai',
        icon: (size) => <OpenAI size={size} />,
        hideable: true,
    },
    {
        id: 'anthropic',
        labelKey: 'layout.nav.useAnthropic',
        descKey: 'scenarioOverview.descriptions.anthropic',
        path: '/agent/anthropic',
        icon: (size) => <Anthropic size={size} />,
        hideable: true,
    },
    {
        id: 'embed',
        labelKey: 'layout.nav.useEmbed',
        descKey: 'scenarioOverview.descriptions.embed',
        path: '/agent/embed',
        icon: (size) => <IconVector sx={{ fontSize: size }} />,
        hideable: true,
    },
    {
        id: 'imagegen',
        labelKey: 'layout.nav.useImageGen',
        descKey: 'scenarioOverview.descriptions.imagegen',
        path: '/agent/imagegen',
        icon: (size) => <IconPhoto sx={{ fontSize: size }} />,
        hideable: true,
    },
    {
        id: 'agent',
        labelKey: 'common.openClaw',
        descKey: 'scenarioOverview.descriptions.agent',
        path: '/agent/agent',
        icon: (size) => <OpenClaw size={size} />,
        hideable: true,
    },
    {
        id: 'team',
        labelKey: 'layout.nav.useTeam',
        descKey: 'scenarioOverview.descriptions.team',
        path: '/agent/team',
        icon: (size) => <IconUsers sx={{ fontSize: size }} />,
        hideable: true,
    },
];

const STORAGE_KEY = 'scenario.hiddenScenarios';
const DEFAULTS_VERSION_KEY = 'scenario.hiddenDefaultsVersion';
const VISIBILITY_EVENT = 'scenario-visibility-change';
const DEFAULT_HIDDEN = ['agent', 'team'];
// Bump this whenever DEFAULT_HIDDEN gains new entries, so existing users pick
// up the new defaults without losing their own customisations.
const DEFAULTS_VERSION = 2;

const readHidden = (): string[] => {
    try {
        const raw = localStorage.getItem(STORAGE_KEY);
        if (raw === null) {
            localStorage.setItem(STORAGE_KEY, JSON.stringify(DEFAULT_HIDDEN));
            localStorage.setItem(DEFAULTS_VERSION_KEY, String(DEFAULTS_VERSION));
            return DEFAULT_HIDDEN;
        }
        const parsed = JSON.parse(raw);
        const stored: string[] = Array.isArray(parsed)
            ? parsed.filter((x): x is string => typeof x === 'string')
            : [];

        // Merge any new default-hidden entries that existing users haven't
        // seen yet (i.e. the stored defaults-version is behind the current one).
        const storedVersion = Number(localStorage.getItem(DEFAULTS_VERSION_KEY) ?? 0);
        if (storedVersion < DEFAULTS_VERSION) {
            const merged = Array.from(new Set([...stored, ...DEFAULT_HIDDEN]));
            localStorage.setItem(STORAGE_KEY, JSON.stringify(merged));
            localStorage.setItem(DEFAULTS_VERSION_KEY, String(DEFAULTS_VERSION));
            return merged;
        }

        return stored;
    } catch {
        return [];
    }
};

const writeHidden = (ids: string[]) => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(ids));
    window.dispatchEvent(new Event(VISIBILITY_EVENT));
};

export const getHiddenScenarios = (): Set<string> => new Set(readHidden());

export const useHiddenScenarios = () => {
    const [hidden, setHidden] = useState<Set<string>>(() => new Set(readHidden()));

    useEffect(() => {
        const sync = () => setHidden(new Set(readHidden()));
        window.addEventListener(VISIBILITY_EVENT, sync);
        window.addEventListener('storage', sync);
        return () => {
            window.removeEventListener(VISIBILITY_EVENT, sync);
            window.removeEventListener('storage', sync);
        };
    }, []);

    const isHidden = useCallback((id: string) => hidden.has(id), [hidden]);

    const toggleHidden = useCallback((id: string) => {
        const next = new Set(readHidden());
        if (next.has(id)) next.delete(id);
        else next.add(id);
        writeHidden([...next]);
        setHidden(next);
    }, []);

    return { hidden, isHidden, toggleHidden };
};
