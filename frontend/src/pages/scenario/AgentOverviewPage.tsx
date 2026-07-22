import {
    Box,
    Card,
    CardActionArea,
    Chip,
    Grid,
    IconButton,
    Skeleton,
    Stack,
    Tooltip,
    Typography,
    alpha,
} from '@mui/material';
import {
    AiAgents as IconAiAgents,
    Photo as IconPhoto,
    Vector as IconVector,
    Users as IconUsers,
    Visibility as IconVisibility,
    VisibilityOff as IconVisibilityOff,
} from '@/components/icons';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { api } from '@/services/api';
import {
    Anthropic,
    Claude,
    ClaudeDesktop,
    Codex,
    OpenAI,
    OpenClaw,
    OpenCode,
    VSCode,
    Xcode,
} from '@/components/BrandIcons';
import PageLayout from '@/components/PageLayout';

export interface ScenarioDescriptor {
    id: string;
    labelKey: string;
    descKey: string;
    path: string;
    icon: (size: number) => React.ReactNode;
    /** Hideable from the sidebar. Claude Code is always shown (anchors profiles). */
    hideable: boolean;
}

const scenarioIconSize = 32;

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

const AgentOverviewPage: React.FC = () => {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const { isHidden, toggleHidden } = useHiddenScenarios();

    const scenarios = useMemo(() => SCENARIOS, []);

    // Per-scenario rule counts drive the card status line ("3 rules" /
    // "Not configured yet"), so this overview answers the user's real question
    // — "which have I set up, which still need attention?" — instead of being a
    // pure launcher (UX principle #1). undefined (after loading) = the fetch
    // for that scenario failed, in which case the card simply omits the
    // status line. `countsLoaded` (distinct from undefined-per-count) gates
    // a skeleton for the true in-flight window, so a fetch failure doesn't
    // read as a permanently-loading card.
    const [ruleCounts, setRuleCounts] = useState<Record<string, number | undefined>>({});
    const [countsLoaded, setCountsLoaded] = useState(false);
    useEffect(() => {
        let cancelled = false;
        (async () => {
            const entries = await Promise.all(
                SCENARIOS.map(async (s) => {
                    try {
                        const res = await api.getRules(s.id);
                        const rules = Array.isArray(res?.data) ? res.data : [];
                        return [s.id, rules.length] as const;
                    } catch {
                        return [s.id, undefined] as const;
                    }
                }),
            );
            if (!cancelled) {
                setRuleCounts(Object.fromEntries(entries));
                setCountsLoaded(true);
            }
        })();
        return () => { cancelled = true; };
    }, []);

    return (
        <PageLayout loading={false}>
            <Box sx={{ maxWidth: 1280, mx: 'auto' }}>
                <Stack
                    direction="row"
                    spacing={1.5}
                    sx={{
                        alignItems: "center",
                        mb: 1
                    }}>
                    <IconAiAgents sx={{ fontSize: 28 }} />
                    <Typography variant="h5" sx={{ fontWeight: 600 }}>
                        {t('scenarioOverview.title')}
                    </Typography>
                </Stack>
                <Typography
                    variant="body2"
                    sx={{
                        color: "text.secondary",
                        mb: 3
                    }}>
                    {t('scenarioOverview.subtitle')}
                </Typography>

                <Grid container spacing={2}>
                    {scenarios.map((s) => {
                        const hidden = s.hideable && isHidden(s.id);
                        const count = ruleCounts[s.id];
                        return (
                            <Grid key={s.id} size={{ xs: 12, sm: 6, md: 4, lg: 3 }}>
                                <Card
                                    variant="outlined"
                                    sx={{
                                        position: 'relative',
                                        opacity: hidden ? 0.55 : 1,
                                        boxShadow: 'none',
                                        transition: 'opacity 0.15s, border-color 0.15s, background-color 0.15s',
                                        // Reveal the visibility toggle on hover/focus so it stays
                                        // available (principle #10) without competing with the
                                        // scenario name for attention (principle #9).
                                        '&:hover .scenario-visibility-toggle, &:focus-within .scenario-visibility-toggle': {
                                            opacity: 1,
                                        },
                                        '&:hover': {
                                            borderColor: 'primary.main',
                                            bgcolor: (theme) => alpha(theme.palette.primary.main, 0.04),
                                        },
                                    }}
                                >
                                    {s.hideable && (
                                        <Tooltip
                                            title={hidden ? t('scenarioOverview.showInSidebar') : t('scenarioOverview.hideFromSidebar', { defaultValue: 'Hide from sidebar' })}
                                            arrow
                                        >
                                            <IconButton
                                                className="scenario-visibility-toggle"
                                                size="small"
                                                onClick={(e) => { e.stopPropagation(); toggleHidden(s.id); }}
                                                sx={{
                                                    position: 'absolute',
                                                    top: 6,
                                                    right: 6,
                                                    zIndex: 1,
                                                    color: 'text.disabled',
                                                    // Keep it visible when hidden (so the state is
                                                    // discoverable), otherwise fade until hover.
                                                    opacity: hidden ? 1 : 0,
                                                }}
                                            >
                                                {hidden ? <IconVisibilityOff fontSize="small" /> : <IconVisibility fontSize="small" />}
                                            </IconButton>
                                        </Tooltip>
                                    )}
                                    <CardActionArea
                                        onClick={() => navigate(s.path)}
                                        sx={{ p: 2 }}
                                    >
                                        <Stack
                                            direction="row"
                                            spacing={1.5}
                                            sx={{
                                                alignItems: "center",
                                                mb: 1
                                            }}>
                                            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', width: 40, height: 40 }}>
                                                {s.icon(scenarioIconSize)}
                                            </Box>
                                            <Box sx={{ flex: 1, minWidth: 0 }}>
                                                <Stack direction="row" spacing={1} sx={{
                                                    alignItems: "center"
                                                }}>
                                                    <Typography variant="subtitle1" sx={{ fontWeight: 600, lineHeight: 1.2 }}>
                                                        {t(s.labelKey)}
                                                    </Typography>
                                                    {hidden && (
                                                        <Chip
                                                            size="small"
                                                            label={t('scenarioOverview.hidden')}
                                                            sx={{ height: 18, fontSize: '0.6875rem' }}
                                                        />
                                                    )}
                                                </Stack>
                                            </Box>
                                        </Stack>
                                        <Typography
                                            variant="body2"
                                            sx={{
                                                color: "text.secondary",
                                                minHeight: 40,
                                                display: '-webkit-box',
                                                WebkitLineClamp: 2,
                                                WebkitBoxOrient: 'vertical',
                                                overflow: 'hidden'
                                            }}>
                                            {t(s.descKey)}
                                        </Typography>
                                        <Box sx={{ mt: 1, minHeight: 20, display: 'flex', alignItems: 'center' }}>
                                            {!countsLoaded ? (
                                                <Skeleton variant="text" width={72} />
                                            ) : count === undefined ? null : count > 0 ? (
                                                <Typography variant="caption" sx={{ color: 'success.main', fontWeight: 500 }}>
                                                    {count === 1
                                                        ? t('scenarioOverview.ruleCountOne', { defaultValue: '1 rule' })
                                                        : t('scenarioOverview.ruleCount', { count, defaultValue: '{{count}} rules' })}
                                                </Typography>
                                            ) : (
                                                <Typography variant="caption" sx={{
                                                    color: "text.disabled"
                                                }}>
                                                    {t('scenarioOverview.notConfigured', { defaultValue: 'Not configured yet' })}
                                                </Typography>
                                            )}
                                        </Box>
                                    </CardActionArea>
                                </Card>
                            </Grid>
                        );
                    })}
                </Grid>
            </Box>
        </PageLayout>
    );
};

export default AgentOverviewPage;
