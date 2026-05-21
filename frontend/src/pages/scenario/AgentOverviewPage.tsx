import {
    Box,
    Card,
    CardActionArea,
    Chip,
    Grid,
    Stack,
    Switch,
    Typography,
    alpha,
} from '@mui/material';
import {
    IconAiAgents,
    IconPhoto,
    IconVector,
} from '@tabler/icons-react';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
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
        icon: (size) => <IconVector size={size} />,
        hideable: true,
    },
    {
        id: 'imagegen',
        labelKey: 'layout.nav.useImageGen',
        descKey: 'scenarioOverview.descriptions.imagegen',
        path: '/agent/imagegen',
        icon: (size) => <IconPhoto size={size} />,
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
];

const STORAGE_KEY = 'scenario.hiddenScenarios';
const VISIBILITY_EVENT = 'scenario-visibility-change';
const DEFAULT_HIDDEN = ['agent'];

const readHidden = (): string[] => {
    try {
        const raw = localStorage.getItem(STORAGE_KEY);
        if (raw === null) {
            localStorage.setItem(STORAGE_KEY, JSON.stringify(DEFAULT_HIDDEN));
            return DEFAULT_HIDDEN;
        }
        const parsed = JSON.parse(raw);
        return Array.isArray(parsed) ? parsed.filter((x): x is string => typeof x === 'string') : [];
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

    return (
        <PageLayout loading={false}>
            <Box sx={{ maxWidth: 1280, mx: 'auto' }}>
                <Stack direction="row" spacing={1.5} alignItems="center" sx={{ mb: 1 }}>
                    <IconAiAgents size={28} />
                    <Typography variant="h5" sx={{ fontWeight: 600 }}>
                        {t('scenarioOverview.title')}
                    </Typography>
                </Stack>
                <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
                    {t('scenarioOverview.subtitle')}
                </Typography>

                <Grid container spacing={2}>
                    {scenarios.map((s) => {
                        const hidden = s.hideable && isHidden(s.id);
                        return (
                            <Grid key={s.id} size={{ xs: 12, sm: 6, md: 4, lg: 3 }}>
                                <Card
                                    variant="outlined"
                                    sx={{
                                        opacity: hidden ? 0.55 : 1,
                                        boxShadow: 'none',
                                        transition: 'opacity 0.15s, border-color 0.15s, background-color 0.15s',
                                        '&:hover': {
                                            borderColor: 'primary.main',
                                            bgcolor: (theme) => alpha(theme.palette.primary.main, 0.04),
                                        },
                                    }}
                                >
                                    <CardActionArea
                                        onClick={() => navigate(s.path)}
                                        sx={{ p: 2 }}
                                    >
                                        <Stack direction="row" spacing={1.5} alignItems="center" sx={{ mb: 1 }}>
                                            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', width: 40, height: 40 }}>
                                                {s.icon(scenarioIconSize)}
                                            </Box>
                                            <Box sx={{ flex: 1, minWidth: 0 }}>
                                                <Stack direction="row" spacing={1} alignItems="center">
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
                                            color="text.secondary"
                                            sx={{
                                                minHeight: 40,
                                                display: '-webkit-box',
                                                WebkitLineClamp: 2,
                                                WebkitBoxOrient: 'vertical',
                                                overflow: 'hidden',
                                            }}
                                        >
                                            {t(s.descKey)}
                                        </Typography>
                                    </CardActionArea>
                                    <Box
                                        sx={{
                                            px: 2,
                                            py: 1,
                                            borderTop: '1px solid',
                                            borderColor: 'divider',
                                            display: 'flex',
                                            alignItems: 'center',
                                            justifyContent: 'space-between',
                                            visibility: s.hideable ? 'visible' : 'hidden',
                                        }}
                                    >
                                        <Typography variant="caption" color="text.secondary">
                                            {t('scenarioOverview.showInSidebar')}
                                        </Typography>
                                        <Switch
                                            size="small"
                                            checked={!hidden}
                                            disabled={!s.hideable}
                                            onChange={() => toggleHidden(s.id)}
                                            onClick={(e) => e.stopPropagation()}
                                        />
                                    </Box>
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
