import { Box, Chip, Collapse, Stack, Tooltip, Typography } from '@mui/material';
import { useEffect, useMemo, useState } from 'react';
import { CheckCircle, ErrorOutline, Circle } from '@/components/icons';
import type { ModelRequestEvent } from '@/components/AILogViewer';

// RequestJourney is the single answer to "how did this request go".
//
// It deliberately replaces what used to be two parallel views of the same
// story (a trace waterfall plus a raw event timeline) that the reader had to
// join in their head. Here the trace spans are the narrative spine — one row
// per pipeline stage, in time order — and the log events hang off the stage
// they belong to as annotations. See .design/ux-principles.md §1 (organize
// around the user's question, not the backend's taxonomy).
//
// It degrades on purpose: with tracing off (or the trace evicted from the
// in-memory ring) the stages are derived from each event's `stage` field
// instead, so the page keeps the same shape minus the duration bars.

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

type StageStatus = 'ok' | 'error' | 'unset';

interface JourneyStage {
    key: string;
    label: string;
    detail?: string;
    badge?: string;
    status: StageStatus;
    start: number; // epoch ms
    end: number; // epoch ms
    depth: number;
    measured: boolean; // false for stages derived from events (no real duration)
    attributes?: Record<string, string>;
    annotations: ModelRequestEvent[];
}

interface RequestJourneyProps {
    events: ModelRequestEvent[];
    traceId?: string;
    getTrace?: (traceId: string) => Promise<TraceDetail | null>;
}

// Fields already surfaced by the row header or the stage line itself. Showing
// them again in the annotation dump is pure noise.
const SUPPRESSED_FIELDS = new Set([
    'request_id', 'source', 'stage', 'type', 'time', 'level', 'msg',
    'trace', 'request', 'trace_id',
    'status', 'latency', 'method', 'path', 'body_size', 'client_ip', 'user_agent',
    'scenario', 'request_model', 'routed_model', 'routed_provider',
]);

const formatDuration = (ms: number): string => {
    if (ms >= 1000) return `${(ms / 1000).toFixed(2)} s`;
    return `${Math.round(ms)} ms`;
};

// Log fields carry raw Go values — a logrus duration is nanoseconds, which
// reads as an unexplained 10-digit number unless converted here.
const formatFieldValue = (key: string, value: unknown): string => {
    if (typeof value === 'number') {
        if (key === 'latency' || key.endsWith('_ns')) return formatDuration(value / 1e6);
        if (key.endsWith('_ms')) return formatDuration(value);
    }
    if (typeof value === 'object' && value !== null) return JSON.stringify(value);
    return String(value);
};

const spanStatus = (code?: string): StageStatus =>
    code === 'Error' ? 'error' : code === 'Ok' ? 'ok' : 'unset';

// Presentation for the span names the gateway emits. Anything unrecognized
// falls through to its raw span name rather than being hidden.
const describeSpan = (span: TraceSpan): { label: string; detail?: string; badge?: string } => {
    const attrs = span.attributes || {};
    const status = attrs['http.response.status_code'];
    switch (span.name) {
        case 'routing':
            return {
                label: 'Routing',
                detail: attrs['tingly.lb.service_id'],
                badge: attrs['tingly.lb.tactic'],
            };
        case 'failover.attempt':
            return {
                label: `Attempt ${attrs['tingly.failover.attempt'] || '?'}`,
                detail: attrs['tingly.lb.service_id'],
                badge: status,
            };
        case 'upstream':
            return { label: 'Upstream', detail: attrs['server.address'], badge: status };
        default:
            return { label: span.name };
    }
};

// Nest by time containment rather than parent ids: a stage that runs wholly
// inside another (an upstream call inside a failover attempt) reads as its
// child even when the backend parents both to the request root.
const assignDepth = <T extends { start: number; end: number }>(rows: T[]): (T & { depth: number })[] => {
    const stack: T[] = [];
    return rows.map((row) => {
        while (stack.length > 0) {
            const top = stack[stack.length - 1];
            if (top.start <= row.start && top.end >= row.end) break;
            stack.pop();
        }
        const depth = stack.length;
        stack.push(row);
        return { ...row, depth };
    });
};

const eventTime = (e: ModelRequestEvent): number => new Date(e.time).getTime();

const buildStages = (events: ModelRequestEvent[], spans: TraceSpan[]): JourneyStage[] => {
    // The root span is the request itself — the table row already states its
    // outcome, so it sets the time scale instead of taking a row.
    const children = spans.filter((s) => spans.some((p) => p.span_id === s.parent_span_id));
    const roots = spans.filter((s) => !children.includes(s));
    const rows = (children.length > 0 ? children : roots.length > 1 ? roots : [])
        .map((span) => ({
            span,
            start: new Date(span.start_time).getTime(),
            end: new Date(span.end_time).getTime(),
        }))
        // Containers before contained, so depth assignment sees them first.
        .sort((a, b) => a.start - b.start || b.end - a.end);

    const spanStages: JourneyStage[] = assignDepth(rows).map(({ span, start, end, depth }) => {
        const { label, detail, badge } = describeSpan(span);
        return {
            key: span.span_id,
            label,
            detail,
            badge,
            status: spanStatus(span.status_code),
            start,
            end,
            depth,
            measured: true,
            attributes: span.attributes,
            annotations: [],
        };
    });

    // The access log envelope repeats the row header verbatim (status,
    // latency, path), so it earns no row of its own — unless it is all we
    // have, as when a request dies in auth before any stage runs.
    const staged = events.filter((e) => e.source !== 'http');
    const relevant = staged.length > 0 ? staged : events;

    const leftovers: ModelRequestEvent[] = [];
    for (const event of relevant) {
        const at = eventTime(event);
        // Deepest containing stage wins: an upstream warning belongs to the
        // upstream call, not to the attempt wrapping it.
        let target: JourneyStage | undefined;
        for (const stage of spanStages) {
            if (at >= stage.start && at <= stage.end) {
                if (!target || stage.depth >= target.depth) target = stage;
            }
        }
        if (target) target.annotations.push(event);
        else leftovers.push(event);
    }

    // Events with no measured stage (transform steps, or every event when
    // tracing is off) still get a row, grouped by the stage they name.
    // A leftover whose stage names an existing measured stage joins it
    // rather than opening a duplicate row — event and span clocks can drift
    // by a millisecond at the boundaries.
    const byLabel = new Map(spanStages.map((s) => [s.label.toLowerCase(), s]));
    const derived = new Map<string, JourneyStage>();
    for (const event of leftovers) {
        const key = event.stage || event.source;
        const measured = byLabel.get(key.toLowerCase());
        if (measured) {
            measured.annotations.push(event);
            continue;
        }
        let stage = derived.get(key);
        if (!stage) {
            stage = {
                key: `derived:${key}`,
                label: key.replace(/_/g, ' ').replace(/^\w/, (c) => c.toUpperCase()),
                status: 'unset',
                start: eventTime(event),
                end: eventTime(event),
                depth: 0,
                measured: false,
                annotations: [],
            };
            derived.set(key, stage);
        }
        stage.end = Math.max(stage.end, eventTime(event));
        stage.annotations.push(event);
        if (event.level === 'error' || event.level === 'fatal' || event.level === 'panic') {
            stage.status = 'error';
        }
    }

    return [...spanStages, ...derived.values()].sort((a, b) => a.start - b.start);
};

const StageIcon = ({ status }: { status: StageStatus }) => {
    if (status === 'error') return <ErrorOutline sx={{ fontSize: 15, color: 'error.main' }} />;
    if (status === 'ok') return <CheckCircle sx={{ fontSize: 15, color: 'success.main' }} />;
    return <Circle sx={{ fontSize: 8, color: 'text.disabled' }} />;
};

// The smart-routing evaluation is the one payload worth rendering richly:
// it explains *why* a service was chosen, rule by rule.
const RoutingRules = ({ fields }: { fields?: Record<string, any> }) => {
    const rules: any[] = Array.isArray(fields?.trace) ? fields!.trace : [];
    if (rules.length === 0) return null;
    const matchedIdx = typeof fields?.matched_rule_index === 'number' ? fields!.matched_rule_index : -1;
    return (
        <Stack spacing={0.5} sx={{ mt: 0.5 }}>
            {rules.map((rule: any) => {
                const isWinner = matchedIdx === rule.rule_index;
                return (
                    <Box
                        key={rule.rule_index}
                        sx={{
                            p: 0.5,
                            borderRadius: 0.5,
                            borderLeft: 2,
                            borderColor: isWinner ? 'success.main' : 'divider',
                            backgroundColor: isWinner ? 'rgba(16,185,129,0.05)' : 'transparent',
                        }}
                    >
                        <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                            <Typography sx={{ fontSize: '0.72rem', flex: 1 }}>
                                #{rule.rule_index} {rule.description || '(no description)'}
                            </Typography>
                            <Typography sx={{ fontSize: '0.65rem', color: isWinner ? 'success.main' : 'text.disabled' }}>
                                {rule.matched ? 'MATCH' : 'skip'}
                            </Typography>
                        </Stack>
                        {Array.isArray(rule.ops) &&
                            rule.ops.map((op: any, i: number) => (
                                <Typography
                                    key={i}
                                    sx={{ fontFamily: 'monospace', fontSize: '0.68rem', color: 'text.secondary', pl: 1 }}
                                >
                                    {op.position}.{op.operation} — {op.reason}
                                </Typography>
                            ))}
                    </Box>
                );
            })}
        </Stack>
    );
};

const Annotation = ({ event }: { event: ModelRequestEvent }) => {
    const level = event.level;
    const color = level === 'error' || level === 'fatal' || level === 'panic'
        ? 'error.main'
        : level === 'warning'
            ? 'warning.main'
            : 'text.secondary';
    const fields = event.fields || {};
    const keys = Object.keys(fields).filter((k) => !SUPPRESSED_FIELDS.has(k));
    return (
        <Box sx={{ pl: 1, borderLeft: 2, borderColor: 'divider' }}>
            <Typography sx={{ fontSize: '0.72rem', color }}>{event.message}</Typography>
            {keys.length > 0 && (
                <Typography sx={{ fontFamily: 'monospace', fontSize: '0.68rem', color: 'text.disabled', wordBreak: 'break-all' }}>
                    {keys.map((k) => `${k}=${formatFieldValue(k, fields[k])}`).join('  ')}
                </Typography>
            )}
            {event.source === 'smart_routing' && <RoutingRules fields={event.fields} />}
        </Box>
    );
};

const RequestJourney = ({ events, traceId, getTrace }: RequestJourneyProps) => {
    const [spans, setSpans] = useState<TraceSpan[]>([]);
    const [traceMissing, setTraceMissing] = useState(false);
    const [openStage, setOpenStage] = useState<string | null>(null);

    useEffect(() => {
        if (!traceId || !getTrace) return;
        let cancelled = false;
        getTrace(traceId)
            .then((detail) => {
                if (cancelled) return;
                if (!detail) setTraceMissing(true);
                else setSpans(detail.spans);
            })
            .catch(() => {
                if (!cancelled) setTraceMissing(true);
            });
        return () => {
            cancelled = true;
        };
    }, [traceId, getTrace]);

    const stages = useMemo(() => buildStages(events, spans), [events, spans]);

    if (stages.length === 0) {
        return (
            <Typography variant="body2" sx={{ color: 'text.secondary', fontStyle: 'italic' }}>
                No stages recorded for this request.
            </Typography>
        );
    }

    const first = Math.min(...stages.map((s) => s.start));
    const last = Math.max(...stages.map((s) => s.end));
    const total = Math.max(1, last - first);

    return (
        <Stack spacing={0.25}>
            {stages.map((stage) => {
                const ms = stage.end - stage.start;
                const open = openStage === stage.key;
                const attrKeys = stage.attributes ? Object.keys(stage.attributes).sort() : [];
                const hasDetail = attrKeys.length > 0 || stage.annotations.length > 0;
                return (
                    <Box key={stage.key}>
                        <Stack
                            direction="row"
                            spacing={1}
                            onClick={() => hasDetail && setOpenStage(open ? null : stage.key)}
                            sx={{
                                alignItems: 'center',
                                py: 0.3,
                                pl: stage.depth * 2,
                                borderRadius: 0.5,
                                cursor: hasDetail ? 'pointer' : 'default',
                                '&:hover': hasDetail ? { backgroundColor: 'action.hover' } : undefined,
                            }}
                        >
                            <StageIcon status={stage.status} />
                            <Typography sx={{ fontSize: '0.78rem', fontWeight: 500, minWidth: 92 }}>
                                {stage.label}
                            </Typography>
                            {stage.detail && (
                                <Typography
                                    sx={{ fontFamily: 'monospace', fontSize: '0.72rem', color: 'text.secondary', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis', maxWidth: 280 }}
                                >
                                    {stage.detail}
                                </Typography>
                            )}
                            {stage.badge && (
                                <Chip
                                    size="small"
                                    label={stage.badge}
                                    color={stage.status === 'error' ? 'error' : 'default'}
                                    sx={{ fontSize: '0.6rem', height: 16 }}
                                />
                            )}
                            <Box sx={{ flex: 1, position: 'relative', height: 10, minWidth: 80 }}>
                                {stage.measured && (
                                    <Tooltip title={formatDuration(ms)} placement="top" arrow>
                                        <Box
                                            sx={{
                                                position: 'absolute',
                                                left: `${((stage.start - first) / total) * 100}%`,
                                                width: `${Math.max(0.6, (ms / total) * 100)}%`,
                                                top: 2,
                                                bottom: 2,
                                                borderRadius: 0.5,
                                                backgroundColor: stage.status === 'error' ? 'error.main' : 'primary.main',
                                                opacity: 0.8,
                                            }}
                                        />
                                    </Tooltip>
                                )}
                            </Box>
                            <Typography sx={{ fontFamily: 'monospace', fontSize: '0.7rem', color: 'text.secondary', minWidth: 56, textAlign: 'right' }}>
                                {stage.measured ? formatDuration(ms) : ''}
                            </Typography>
                        </Stack>

                        {stage.annotations.length > 0 && (
                            <Stack spacing={0.4} sx={{ pl: stage.depth * 2 + 3.5, py: 0.25 }}>
                                {stage.annotations.map((event, i) => (
                                    <Annotation key={i} event={event} />
                                ))}
                            </Stack>
                        )}

                        <Collapse in={open} timeout="auto" unmountOnExit>
                            <Stack spacing={0.1} sx={{ pl: stage.depth * 2 + 3.5, pb: 0.5 }}>
                                {attrKeys.map((k) => (
                                    <Typography key={k} sx={{ fontFamily: 'monospace', fontSize: '0.68rem', color: 'text.disabled', wordBreak: 'break-all' }}>
                                        {k}={stage.attributes![k]}
                                    </Typography>
                                ))}
                            </Stack>
                        </Collapse>
                    </Box>
                );
            })}

            {traceMissing && traceId && (
                <Typography variant="caption" sx={{ color: 'text.disabled', fontStyle: 'italic' }}>
                    Span timings unavailable — this trace is no longer in the in-memory buffer.
                </Typography>
            )}
        </Stack>
    );
};

export default RequestJourney;
