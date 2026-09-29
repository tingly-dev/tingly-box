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
    Visibility as IconVisibility,
    VisibilityOff as IconVisibilityOff,
} from '@/components/icons';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { api } from '@/services/api';
import PageLayout from '@/components/PageLayout';
import { SCENARIOS, useHiddenScenarios, type ScenarioDescriptor } from './scenarioRegistry';
import PowerUpsSection from './PowerUpsSection';
import UnifiedCard from '@/components/UnifiedCard';

const scenarioIconSize = 28;

// Cap the page body so the card grid keeps a readable column width on wide
// monitors instead of stretching to the window. Centered, so the page still
// reads as one column of content rather than a left-anchored strip.
const pageContentMaxWidth = 1280;

interface AgentCardProps {
    scenario: ScenarioDescriptor;
    count: number | undefined;
    countsLoaded: boolean;
    hidden: boolean;
    onOpen: () => void;
    onToggleHidden: () => void;
}

// One card component for both states: hiding an agent marks its card, it does
// not reshape it. A grid of half-height rows below a grid of full cards read as
// two different things, when the only difference is visibility.
const AgentCard: React.FC<AgentCardProps> = ({
    scenario,
    count,
    countsLoaded,
    hidden,
    onOpen,
    onToggleHidden,
}) => {
    const { t } = useTranslation();

    return (
        <Card
            variant="outlined"
            sx={{
                position: 'relative',
                boxShadow: 'none',
                opacity: hidden ? 0.55 : 1,
                transition: 'opacity 0.15s, border-color 0.15s, background-color 0.15s',
                // Reveal the visibility toggle on hover/focus so it stays
                // available (principle #10) without competing with the
                // scenario name for attention (principle #9). A hidden card
                // keeps it visible instead — there the toggle is the card's
                // state, and the state has to be readable without hovering.
                '&:hover .scenario-visibility-toggle, &:focus-within .scenario-visibility-toggle': {
                    opacity: 1,
                },
                '&:hover': {
                    borderColor: 'primary.main',
                    bgcolor: (theme) => alpha(theme.palette.primary.main, 0.04),
                },
            }}
        >
            {scenario.hideable && (
                <Tooltip
                    title={hidden
                        ? t('scenarioOverview.showInSidebar')
                        : t('scenarioOverview.hideFromSidebar', { defaultValue: 'Hide from sidebar' })}
                    arrow
                >
                    <IconButton
                        className="scenario-visibility-toggle"
                        size="small"
                        onClick={(e) => { e.stopPropagation(); onToggleHidden(); }}
                        sx={{
                            position: 'absolute',
                            top: 6,
                            right: 6,
                            zIndex: 1,
                            color: 'text.disabled',
                            opacity: hidden ? 1 : 0,
                        }}
                    >
                        {hidden
                            ? <IconVisibilityOff fontSize="small" />
                            : <IconVisibility fontSize="small" />}
                    </IconButton>
                </Tooltip>
            )}
            <CardActionArea
                onClick={onOpen}
                sx={{ p: 1.5 }}
            >
                {/* Rule count sits under the name, beside the icon,
                    instead of on its own footer row — one row less
                    per card. The right padding on a hidden card keeps the
                    Hidden chip clear of the always-visible toggle. */}
                <Stack
                    direction="row"
                    spacing={1.25}
                    sx={{
                        alignItems: "center",
                        mb: 0.75,
                        pr: hidden ? 4 : 0,
                    }}>
                    <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', width: 36, height: 36, flexShrink: 0 }}>
                        {scenario.icon(scenarioIconSize)}
                    </Box>
                    <Box sx={{ flex: 1, minWidth: 0 }}>
                        <Stack direction="row" spacing={1} sx={{ alignItems: 'center', minWidth: 0 }}>
                            <Typography variant="subtitle1" noWrap sx={{ fontWeight: 600, lineHeight: 1.2 }}>
                                {t(scenario.labelKey)}
                            </Typography>
                            {hidden && (
                                <Chip
                                    size="small"
                                    label={t('scenarioOverview.hidden')}
                                    sx={{ height: 18, fontSize: '0.6875rem', flexShrink: 0 }}
                                />
                            )}
                        </Stack>
                        <Box sx={{ minHeight: 18, display: 'flex', alignItems: 'center' }}>
                            {!countsLoaded ? (
                                <Skeleton variant="text" width={56} />
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
                    {t(scenario.descKey)}
                </Typography>
            </CardActionArea>
        </Card>
    );
};

const AgentOverviewPage: React.FC = () => {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const { isHidden, toggleHidden } = useHiddenScenarios();

    // Hidden agents drop to a labelled group under the main grid, but keep the
    // same card — the grid answers "which do I use?", and the group keeps the
    // hidden ones reachable without mixing them into that answer.
    const visibleScenarios = SCENARIOS.filter(s => !(s.hideable && isHidden(s.id)));
    const hiddenScenarios = SCENARIOS.filter(s => s.hideable && isHidden(s.id));

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
            <Box sx={{ maxWidth: pageContentMaxWidth, mx: 'auto' }}>
                <Stack spacing={3}>
                    <UnifiedCard
                        size="full"
                        titleHeadingLevel={1}
                        title={t('scenarioOverview.title')}
                        subtitle={t('scenarioOverview.subtitle')}
                    >
                        <Grid container spacing={2}>
                            {visibleScenarios.map((s) => (
                                <Grid key={s.id} size={{ xs: 12, sm: 6, md: 4, lg: 3 }}>
                                    <AgentCard
                                        scenario={s}
                                        count={ruleCounts[s.id]}
                                        countsLoaded={countsLoaded}
                                        hidden={false}
                                        onOpen={() => navigate(s.path)}
                                        onToggleHidden={() => toggleHidden(s.id)}
                                    />
                                </Grid>
                            ))}
                        </Grid>

                        {hiddenScenarios.length > 0 && (
                            <Box sx={{ mt: 3 }}>
                                <Typography variant="caption" sx={{ color: 'text.secondary', fontWeight: 600 }}>
                                    {t('scenarioOverview.hidden')} · {hiddenScenarios.length}
                                </Typography>
                                {/* Same grid, same card, same size — dimmed and
                                    chipped, so hidden agents stay one glance away
                                    instead of becoming a second, smaller species. */}
                                <Grid container spacing={2} sx={{ mt: 0.5 }}>
                                    {hiddenScenarios.map((s) => (
                                        <Grid key={s.id} size={{ xs: 12, sm: 6, md: 4, lg: 3 }}>
                                            <AgentCard
                                                scenario={s}
                                                count={ruleCounts[s.id]}
                                                countsLoaded={countsLoaded}
                                                hidden
                                                onOpen={() => navigate(s.path)}
                                                onToggleHidden={() => toggleHidden(s.id)}
                                            />
                                        </Grid>
                                    ))}
                                </Grid>
                            </Box>
                        )}
                    </UnifiedCard>

                    <PowerUpsSection />
                </Stack>
            </Box>
        </PageLayout>
    );
};

export default AgentOverviewPage;
