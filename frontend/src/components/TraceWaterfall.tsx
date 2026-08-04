import { Box, Chip, Stack, Tooltip, Typography } from '@mui/material';
import { useEffect, useState } from 'react';

// TraceWaterfall renders the span tree of one request from the in-memory
// trace buffer, inline in the AI Logs timeline (the logs page is the only
// entry point for traces — see .design/otel.md §7.4).

export interface TraceSpan {
    trace_id: string;
    span_id: string;
    parent_span_id?: string;
    name: string;
    kind?: string;
    start_time: string;
    end_time: string;
    status_code?: string;
    status_message?: string;
    attributes?: Record<string, string>;
}

export interface TraceDetail {
    trace_id: string;
    spans: TraceSpan[];
    dropped_spans?: number;
}

interface TraceWaterfallProps {
    traceId: string;
    // Returns null when the trace is not (or no longer) in the buffer.
    getTrace: (traceId: string) => Promise<TraceDetail | null>;
}

const durationMs = (s: TraceSpan): number =>
    Math.max(0, new Date(s.end_time).getTime() - new Date(s.start_time).getTime());

const formatDuration = (ms: number): string => (ms >= 1000 ? `${(ms / 1000).toFixed(2)} s` : `${ms} ms`);

// Depth from parent chain so children indent under their parent.
const spanDepth = (span: TraceSpan, byId: Map<string, TraceSpan>): number => {
    let depth = 0;
    let current = span;
    while (current.parent_span_id) {
        const parent = byId.get(current.parent_span_id);
        if (!parent) break;
        depth += 1;
        current = parent;
    }
    return depth;
};

const TraceWaterfall = ({ traceId, getTrace }: TraceWaterfallProps) => {
    const [detail, setDetail] = useState<TraceDetail | null | undefined>(undefined);
    const [error, setError] = useState<string | null>(null);
    const [expandedSpan, setExpandedSpan] = useState<string | null>(null);

    useEffect(() => {
        let cancelled = false;
        getTrace(traceId)
            .then((d) => {
                if (!cancelled) setDetail(d);
            })
            .catch((e) => {
                if (!cancelled) setError(e instanceof Error ? e.message : 'Failed to load trace');
            });
        return () => {
            cancelled = true;
        };
    }, [traceId, getTrace]);

    if (error) {
        return (
            <Typography variant="body2" sx={{ color: 'text.secondary', fontStyle: 'italic' }}>
                Failed to load trace: {error}
            </Typography>
        );
    }
    if (detail === undefined) {
        return (
            <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                Loading trace...
            </Typography>
        );
    }
    if (detail === null || detail.spans.length === 0) {
        return (
            <Typography variant="body2" sx={{ color: 'text.secondary', fontStyle: 'italic' }}>
                Trace no longer buffered (the in-memory ring keeps only recent traces).
            </Typography>
        );
    }

    const spans = detail.spans;
    const byId = new Map(spans.map((s) => [s.span_id, s]));
    const traceStart = Math.min(...spans.map((s) => new Date(s.start_time).getTime()));
    const traceEnd = Math.max(...spans.map((s) => new Date(s.end_time).getTime()));
    const total = Math.max(1, traceEnd - traceStart);

    return (
        <Stack spacing={0.25}>
            {spans.map((span) => {
                const ms = durationMs(span);
                const leftPct = ((new Date(span.start_time).getTime() - traceStart) / total) * 100;
                const widthPct = Math.max(0.5, (ms / total) * 100);
                const isError = span.status_code === 'Error';
                const depth = spanDepth(span, byId);
                const expanded = expandedSpan === span.span_id;
                const attrKeys = span.attributes ? Object.keys(span.attributes).sort() : [];
                return (
                    <Box key={span.span_id}>
                        <Stack
                            direction="row"
                            spacing={1}
                            onClick={() => setExpandedSpan(expanded ? null : span.span_id)}
                            sx={{ alignItems: 'center', cursor: 'pointer', pl: depth * 2, py: 0.25, '&:hover': { backgroundColor: 'action.hover' }, borderRadius: 0.5 }}
                        >
                            <Typography
                                sx={{ fontFamily: 'monospace', fontSize: '0.72rem', minWidth: 180, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis', color: isError ? 'error.main' : 'text.primary' }}
                            >
                                {span.name}
                            </Typography>
                            {isError && (
                                <Chip size="small" label={span.status_message || 'error'} color="error" sx={{ fontSize: '0.6rem', height: 16 }} />
                            )}
                            <Box sx={{ flex: 1, position: 'relative', height: 14, backgroundColor: 'action.hover', borderRadius: 0.5, minWidth: 120 }}>
                                <Tooltip title={`${formatDuration(ms)} · ${span.kind || ''}`} placement="top" arrow>
                                    <Box
                                        sx={{
                                            position: 'absolute',
                                            left: `${leftPct}%`,
                                            width: `${widthPct}%`,
                                            top: 2,
                                            bottom: 2,
                                            borderRadius: 0.5,
                                            backgroundColor: isError ? 'error.main' : 'primary.main',
                                            opacity: 0.85,
                                        }}
                                    />
                                </Tooltip>
                            </Box>
                            <Typography sx={{ fontFamily: 'monospace', fontSize: '0.7rem', color: 'text.secondary', minWidth: 64, textAlign: 'right' }}>
                                {formatDuration(ms)}
                            </Typography>
                        </Stack>
                        {expanded && attrKeys.length > 0 && (
                            <Stack spacing={0.1} sx={{ pl: depth * 2 + 2, pb: 0.5 }}>
                                {attrKeys.map((k) => (
                                    <Typography key={k} sx={{ fontFamily: 'monospace', fontSize: '0.68rem', color: 'text.secondary', wordBreak: 'break-all' }}>
                                        {k}={span.attributes![k]}
                                    </Typography>
                                ))}
                            </Stack>
                        )}
                    </Box>
                );
            })}
            {(detail.dropped_spans ?? 0) > 0 && (
                <Typography variant="caption" sx={{ color: 'text.secondary', fontStyle: 'italic' }}>
                    {detail.dropped_spans} span(s) dropped — trace exceeded the per-trace buffer cap.
                </Typography>
            )}
        </Stack>
    );
};

export default TraceWaterfall;
