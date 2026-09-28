import {
    Box,
    Card,
    CardActionArea,
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
import { SCENARIOS, useHiddenScenarios } from './scenarioRegistry';
import PowerUpsSection from './PowerUpsSection';
import UnifiedCard from '@/components/UnifiedCard';

const scenarioIconSize = 28;

const AgentOverviewPage: React.FC = () => {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const { isHidden, toggleHidden } = useHiddenScenarios();

    // Hidden agents leave the card grid for a compact one-line row below it — a
    // different shape, not just a dimmer card, so "in use" vs "hidden" reads
    // at a glance. Hiding/showing moves the agent between the two on purpose.
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
            <Stack spacing={3}>
                <UnifiedCard
                    size="full"
                    titleHeadingLevel={1}
                    title={t('scenarioOverview.title')}
                    subtitle={t('scenarioOverview.subtitle')}
                >
                    <Grid container spacing={2}>
                        {visibleScenarios.map((s) => {
                            const count = ruleCounts[s.id];
                            return (
                                <Grid key={s.id} size={{ xs: 12, sm: 6, md: 4, lg: 3 }}>
                                    <Card
                                        variant="outlined"
                                        sx={{
                                            position: 'relative',
                                            boxShadow: 'none',
                                            transition: 'border-color 0.15s, background-color 0.15s',
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
                                                title={t('scenarioOverview.hideFromSidebar', { defaultValue: 'Hide from sidebar' })}
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
                                                        opacity: 0,
                                                    }}
                                                >
                                                    <IconVisibility fontSize="small" />
                                                </IconButton>
                                            </Tooltip>
                                        )}
                                        <CardActionArea
                                            onClick={() => navigate(s.path)}
                                            sx={{ p: 1.5 }}
                                        >
                                            {/* Rule count sits under the name, beside the icon,
                                                instead of on its own footer row — one row less
                                                per card. */}
                                            <Stack
                                                direction="row"
                                                spacing={1.25}
                                                sx={{
                                                    alignItems: "center",
                                                    mb: 0.75
                                                }}>
                                                <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', width: 36, height: 36, flexShrink: 0 }}>
                                                    {s.icon(scenarioIconSize)}
                                                </Box>
                                                <Box sx={{ flex: 1, minWidth: 0 }}>
                                                    <Typography variant="subtitle1" noWrap sx={{ fontWeight: 600, lineHeight: 1.2 }}>
                                                        {t(s.labelKey)}
                                                    </Typography>
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
                                                {t(s.descKey)}
                                            </Typography>
                                        </CardActionArea>
                                    </Card>
                                </Grid>
                            );
                        })}
                    </Grid>

                    {hiddenScenarios.length > 0 && (
                        <Box sx={{ mt: 3 }}>
                            <Typography variant="caption" sx={{ color: 'text.secondary', fontWeight: 600 }}>
                                {t('scenarioOverview.hidden')} · {hiddenScenarios.length}
                            </Typography>
                            {/* Same column grid as the cards above so the hidden row
                                lines up with them — just one line tall instead of a card. */}
                            <Grid container spacing={2} sx={{ mt: 0.5 }}>
                                {hiddenScenarios.map((s) => (
                                    <Grid key={s.id} size={{ xs: 12, sm: 6, md: 4, lg: 3 }}>
                                        <Card variant="outlined" sx={{ display: 'flex', alignItems: 'center', boxShadow: 'none' }}>
                                            <CardActionArea
                                                onClick={() => navigate(s.path)}
                                                sx={{ display: 'flex', justifyContent: 'flex-start', gap: 1.25, px: 1.5, py: 0.75, minWidth: 0 }}
                                            >
                                                <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', width: 20, flexShrink: 0 }}>
                                                    {s.icon(18)}
                                                </Box>
                                                <Typography variant="body2" noWrap sx={{ color: 'text.secondary' }}>
                                                    {t(s.labelKey)}
                                                </Typography>
                                            </CardActionArea>
                                            <Tooltip title={t('scenarioOverview.showInSidebar')} arrow>
                                                <IconButton
                                                    size="small"
                                                    onClick={() => toggleHidden(s.id)}
                                                    sx={{ color: 'text.disabled', mr: 0.5 }}
                                                >
                                                    <IconVisibilityOff fontSize="small" />
                                                </IconButton>
                                            </Tooltip>
                                        </Card>
                                    </Grid>
                                ))}
                            </Grid>
                        </Box>
                    )}
                </UnifiedCard>

                <PowerUpsSection />
            </Stack>
        </PageLayout>
    );
};

export default AgentOverviewPage;
