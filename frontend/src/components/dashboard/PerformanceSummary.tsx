import { useEffect, useRef, useState } from 'react';
import { Box, CircularProgress, Paper, Typography } from '@mui/material';
import api from '@/services/api';

interface MetricPercentiles {
    sample_count: number;
    p10?: number;
    p50?: number;
    p90?: number;
    p95?: number;
    p99?: number;
}

interface PerformanceSummaryData {
    ttft: MetricPercentiles;
    tps: MetricPercentiles;
    completion: MetricPercentiles;
}

export interface PerformanceQueryParams {
    start_time: string;
    end_time: string;
    provider: string;
    model: string;
    user: string;
}

const fmtLatency = (ms: number) => {
    if (ms >= 1000) return `${(ms / 1000).toFixed(1)}s`;
    return `${ms}ms`;
};

type PercentileKey = 'p99' | 'p95' | 'p90' | 'p50' | 'p10';

const PERCENTILES: Array<{ key: PercentileKey; label: string }> = [
    { key: 'p99', label: 'P99' },
    { key: 'p95', label: 'P95' },
    { key: 'p90', label: 'P90' },
    { key: 'p50', label: 'P50' },
    { key: 'p10', label: 'P10' },
];

function formatMetricValue(metric: MetricPercentiles | undefined, kind: 'latency' | 'tps', percentile: PercentileKey) {
    if (!metric?.sample_count) return '—';
    if (kind === 'tps' && percentile === 'p99') return '—';
    const value = metric[percentile];
    if (typeof value !== 'number' || !Number.isFinite(value)) return '—';
    return kind === 'tps'
        ? value.toFixed(1)
        : fmtLatency(Math.round(value));
}

export default function PerformanceSummary({ queryParams }: { queryParams: PerformanceQueryParams | null }) {
    const [data, setData] = useState<PerformanceSummaryData | null>(null);
    const [loading, setLoading] = useState(false);
    const requestSeq = useRef(0);

    useEffect(() => {
        if (!queryParams) return;
        const seq = ++requestSeq.current;
        setLoading(true);
        setData(null);
        (async () => {
            try {
                const filters: Record<string, string> = {
                    start_time: queryParams.start_time,
                    end_time: queryParams.end_time,
                };
                if (queryParams.provider !== 'all') filters.provider = queryParams.provider;
                if (queryParams.model !== 'all') filters.model = queryParams.model;
                if (queryParams.user !== 'all') filters.user_id = queryParams.user;
                const result = await api.getUsagePerformance(filters);
                if (seq === requestSeq.current && result?.ttft) setData(result);
            } catch (error) {
                console.error('Failed to load performance summary:', error);
            } finally {
                if (seq === requestSeq.current) setLoading(false);
            }
        })();
    }, [queryParams]);

    const metrics = [
        { key: 'ttft', title: 'TTFT', metric: data?.ttft, kind: 'latency' as const },
        { key: 'tps', title: 'TPS', metric: data?.tps, kind: 'tps' as const },
        { key: 'latency', title: 'Latency', metric: data?.completion, kind: 'latency' as const },
    ];

    return (
        <Paper elevation={0} sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, backgroundColor: 'background.paper', boxShadow: 'none', overflow: 'hidden', display: 'flex', flexDirection: 'column', flex: 1, minWidth: 0 }}>
            <Box sx={{ px: 2.25, pt: 2, pb: 0.75, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Typography sx={{ fontWeight: 600, fontSize: '0.875rem' }}>Response Performance</Typography>
                {loading && <CircularProgress size={18} />}
            </Box>
            <Box sx={{ px: 2.25, pb: 1, display: 'grid', gridTemplateColumns: '52px repeat(3, minmax(0, 1fr))', columnGap: 1 }}>
                <Box />
                {metrics.map(({ key, title, metric }) => (
                    <Box key={key} sx={{ minWidth: 0 }}>
                        <Typography sx={{ fontWeight: 600, fontSize: '0.76rem' }}>{title}</Typography>
                        <Typography sx={{ color: 'text.disabled', fontSize: '0.6rem', mt: 0.2 }}>
                            n={metric?.sample_count.toLocaleString() ?? 0}
                        </Typography>
                    </Box>
                ))}
            </Box>
            <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', '& > * + *': { borderTop: '1px solid', borderColor: 'divider' } }}>
                {PERCENTILES.map(({ key, label }) => (
                    <Box key={key} sx={{ px: 2.25, py: 1.25, minWidth: 0, flex: 1, display: 'grid', gridTemplateColumns: '52px repeat(3, minmax(0, 1fr))', alignItems: 'center', columnGap: 1 }}>
                        <Typography sx={{ color: 'text.secondary', fontSize: '0.68rem', fontWeight: 600 }}>{label}</Typography>
                        {metrics.map((metric) => {
                            const value = formatMetricValue(metric.metric, metric.kind, key);
                            return (
                                <Typography key={metric.key} sx={{ color: value === '—' ? 'text.disabled' : 'text.primary', fontWeight: 650, fontSize: '0.86rem', whiteSpace: 'nowrap' }}>
                                    {value}
                                </Typography>
                            );
                        })}
                    </Box>
                ))}
            </Box>
        </Paper>
    );
}
