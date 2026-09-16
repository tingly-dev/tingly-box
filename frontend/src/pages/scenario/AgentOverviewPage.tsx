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
    Visibility as IconVisibility,
    VisibilityOff as IconVisibilityOff,
} from '@/components/icons';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { api } from '@/services/api';
import PageLayout from '@/components/PageLayout';
import PageHeader from '@/components/PageHeader';
import { SCENARIOS, useHiddenScenarios } from './scenarioRegistry';

const scenarioIconSize = 32;

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
                <PageHeader
                    title={t('scenarioOverview.title')}
                    subtitle={t('scenarioOverview.subtitle')}
                    icon={<IconAiAgents sx={{ fontSize: 28 }} />}
                    sx={{ mb: 3 }}
                />

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
