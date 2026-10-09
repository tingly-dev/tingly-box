import { Box, Button, Chip, IconButton, Stack, Tooltip, Typography } from '@mui/material';
import { Refresh as RefreshIcon } from '@/components/icons';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link as RouterLink } from 'react-router-dom';
import UnifiedCard from '@/components/UnifiedCard';
// Straight from chartStyles, not the dashboard barrel: the barrel pulls the
// charting library into every agent page.
import { formatNumber, getTotalTokens } from '@/components/dashboard/chartStyles';
import { api } from '@/services/api';
import { getLocalMidnight, toLocalISOString } from '@/utils/datetime';
import { fontMono, fontSizes } from '@/theme/fonts';

const RECENT_LIMIT = 5;
const REFRESH_MS = 30_000;

interface TodayTotals {
    requests: number;
    tokens: number;
    errors: number;
}

interface RecentRequest {
    id: number;
    timestamp: string;
    status: string;
    model: string;
    request_model?: string;
    provider_name?: string;
    latency_ms?: number;
    error_code?: string;
}

const fmtLatency = (ms: number) => (ms >= 1000 ? `${(ms / 1000).toFixed(1)}s` : `${ms}ms`);

/**
 * AgentActivityCard — "is this agent getting traffic, and is it healthy?",
 * answered on the agent's own page: today's requests / tokens / errors and its
 * last few requests, with a link into the dashboard filtered to this agent.
 * It is the quick-look half of observing an agent; the dashboard keeps the
 * analysis half.
 */
const AgentActivityCard: React.FC<{ scenario: string }> = ({ scenario }) => {
    const { t, i18n } = useTranslation();
    const [totals, setTotals] = useState<TodayTotals | null>(null);
    const [recent, setRecent] = useState<RecentRequest[] | null>(null);
    const [loading, setLoading] = useState(false);
    const [failed, setFailed] = useState(false);
    const seq = useRef(0);

    const load = useCallback(async () => {
        const mine = ++seq.current;
        setLoading(true);
        try {
            const now = new Date();
            const [stats, records] = await Promise.all([
                api.getUsageStats({
                    scenario,
                    group_by: 'scenario',
                    start_time: toLocalISOString(getLocalMidnight(now)),
                    end_time: toLocalISOString(now),
                    limit: 1,
                }),
                api.getUsageRecords({ scenario, limit: RECENT_LIMIT }),
            ]);
            if (mine !== seq.current) return;
            const row = Array.isArray(stats?.data) ? stats.data[0] : undefined;
            setTotals({
                requests: row?.request_count ?? 0,
                tokens: row ? getTotalTokens(row) : 0,
                errors: row?.error_count ?? 0,
            });
            setRecent(Array.isArray(records?.data) ? records.data : []);
            setFailed(false);
        } catch {
            if (mine === seq.current) setFailed(true);
        } finally {
            if (mine === seq.current) setLoading(false);
        }
    }, [scenario]);

    useEffect(() => {
        void load();
        // Watching an agent means leaving this open: keep it fresh, but not
        // from a background tab.
        const timer = window.setInterval(() => {
            if (document.visibilityState === 'visible') void load();
        }, REFRESH_MS);
        return () => window.clearInterval(timer);
    }, [load]);

    const relative = (iso: string) => {
        const seconds = Math.round((new Date(iso).getTime() - Date.now()) / 1000);
        const rtf = new Intl.RelativeTimeFormat(i18n.language, { numeric: 'auto', style: 'narrow' });
        if (Math.abs(seconds) < 60) return rtf.format(seconds, 'second');
        if (Math.abs(seconds) < 3600) return rtf.format(Math.round(seconds / 60), 'minute');
        if (Math.abs(seconds) < 86400) return rtf.format(Math.round(seconds / 3600), 'hour');
        return rtf.format(Math.round(seconds / 86400), 'day');
    };

    const dashboardHref = `/dashboard/today?scenario=${encodeURIComponent(scenario)}`;
    const empty = recent !== null && recent.length === 0;
    const errorRate = totals && totals.requests > 0 ? (totals.errors / totals.requests) * 100 : 0;

    const stat = (label: string, value: string, hint?: string, danger = false) => (
        <Box sx={{ minWidth: 96 }}>
            <Typography variant="caption" sx={{ color: 'text.secondary', display: 'block' }}>{label}</Typography>
            <Typography sx={{ fontSize: '1.25rem', fontWeight: 600, lineHeight: 1.3, color: danger ? 'error.main' : 'text.primary' }}>
                {value}
            </Typography>
            {hint && <Typography variant="caption" sx={{ color: 'text.secondary' }}>{hint}</Typography>}
        </Box>
    );

    return (
        <UnifiedCard
            id="agent-activity"
            title={t('agentActivity.title')}
            size="full"
            rightAction={
                <Stack direction="row" spacing={0.5} sx={{ alignItems: 'center' }}>
                    <Tooltip title={t('agentActivity.refresh')}>
                        <span>
                            <IconButton size="small" onClick={() => void load()} disabled={loading} aria-label={t('agentActivity.refresh')}>
                                <RefreshIcon fontSize="small" />
                            </IconButton>
                        </span>
                    </Tooltip>
                    <Button component={RouterLink} to={dashboardHref} size="small" variant="text" sx={{ textTransform: 'none' }}>
                        {t('agentActivity.openDashboard')}
                    </Button>
                </Stack>
            }
        >
            {failed && recent === null ? (
                <Typography variant="body2" sx={{ color: 'text.secondary' }}>{t('agentActivity.loadFailed')}</Typography>
            ) : empty && (totals?.requests ?? 0) === 0 ? (
                // Nothing yet is itself the signal: the first request will show up here.
                <Typography variant="body2" sx={{ color: 'text.secondary' }}>{t('agentActivity.empty')}</Typography>
            ) : (
                <Stack spacing={2}>
                    <Stack direction="row" spacing={4} useFlexGap sx={{ flexWrap: 'wrap' }}>
                        {stat(t('agentActivity.requestsToday'), totals ? totals.requests.toLocaleString() : '–')}
                        {stat(t('agentActivity.tokensToday'), totals ? formatNumber(totals.tokens) : '–')}
                        {stat(
                            t('agentActivity.errorsToday'),
                            totals ? totals.errors.toLocaleString() : '–',
                            totals && totals.requests > 0 ? `${errorRate.toFixed(1)}%` : undefined,
                            !!totals && totals.errors > 0,
                        )}
                    </Stack>
                    {(recent?.length ?? 0) > 0 && (
                    <Box>
                        <Typography variant="caption" sx={{ color: 'text.secondary', display: 'block', mb: 0.5 }}>
                            {t('agentActivity.recent')}
                        </Typography>
                        <Stack divider={<Box sx={{ borderTop: '1px solid', borderColor: 'divider' }} />}>
                            {(recent ?? []).map((r) => {
                                const ok = r.status !== 'error';
                                const route = r.request_model && r.request_model !== r.model
                                    ? `${r.request_model} → ${r.model}`
                                    : r.model;
                                return (
                                    <Stack key={r.id} direction="row" spacing={1.5} sx={{ alignItems: 'center', py: 0.75, minWidth: 0 }}>
                                        <Typography variant="caption" sx={{ color: 'text.secondary', width: 56, flexShrink: 0 }}>
                                            {relative(r.timestamp)}
                                        </Typography>
                                        <Chip
                                            size="small"
                                            variant="outlined"
                                            color={ok ? 'success' : 'error'}
                                            label={ok ? t('agentActivity.ok') : (r.error_code || t('agentActivity.error'))}
                                            sx={{ height: 20, fontSize: fontSizes.xs, flexShrink: 0 }}
                                        />
                                        <Typography variant="body2" noWrap sx={{ fontFamily: fontMono, minWidth: 0, flex: 1 }} title={route}>
                                            {route}
                                        </Typography>
                                        {r.provider_name && (
                                            <Typography variant="caption" noWrap sx={{ color: 'text.secondary', maxWidth: 140, display: { xs: 'none', sm: 'block' } }}>
                                                {r.provider_name}
                                            </Typography>
                                        )}
                                        {r.latency_ms ? (
                                            <Typography variant="caption" sx={{ color: 'text.secondary', width: 48, textAlign: 'right', flexShrink: 0 }}>
                                                {fmtLatency(r.latency_ms)}
                                            </Typography>
                                        ) : null}
                                    </Stack>
                                );
                            })}
                        </Stack>
                    </Box>
                    )}
                </Stack>
            )}
        </UnifiedCard>
    );
};

export default AgentActivityCard;
