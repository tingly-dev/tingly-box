import { Box, Chip, Collapse, Stack, Typography } from '@mui/material';
import { useEffect, useMemo, useState } from 'react';
import { CheckCircle, ErrorOutline, Circle } from '@/components/icons';
import type { ModelRequestEvent } from '@/components/AILogViewer';

// RequestJourney is the single answer to "how did this request go".
//
// It replaces what used to be two parallel accounts of the same story (a
// trace waterfall plus a raw event timeline) that the reader had to join in
// their head — see .design/ux-principles.md §1. The trace spans are the
// spine: one line per pipeline stage, top to bottom in time order, with the
// log events that belong to a stage hanging under it.
//
// It is a list, not a chart. Duration bars only earn their space when the
// reader needs to see overlap or concurrency; these stages are strictly
// sequential, so an aligned column of durations compares them better than
// bars — which spent 600px to render a 12ms stage as an invisible dot.
//
// It degrades on purpose: with tracing off (or the trace evicted from the
// in-memory ring) the stages come from each event's `stage` field instead,
// so the page keeps its shape minus the timings.

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
    facts: string[];
    badge?: string;
    status: StageStatus;
    start: number; // epoch ms
    end: number; // epoch ms
    measured: boolean; // false for stages derived from events (no real duration)
    attributes?: Record<string, string>;
    annotations: ModelRequestEvent[];
}

interface RequestJourneyProps {
    events: ModelRequestEvent[];
    traceId?: string;
    getTrace?: (traceId: string) => Promise<TraceDetail | null>;
}

// Fields already stated by the row header, the stage line, or the rule
// breakdown. Repeating them in the annotation dump is pure noise.
const SUPPRESSED_FIELDS = new Set([
    'request_id', 'source', 'stage', 'type', 'time', 'level', 'msg',
    'trace', 'request', 'trace_id',
    'status', 'latency', 'method', 'path', 'body_size', 'client_ip', 'user_agent',
    'scenario', 'request_model', 'routed_model', 'routed_provider',
    'outcome', 'matched_rule_index', 'selected_provider', 'selected_model',
]);

const formatDuration = (ms: number): string =>
    ms >= 1000 ? `${(ms / 1000).toFixed(2)} s` : `${Math.round(ms)} ms`;

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

const eventTime = (e: ModelRequestEvent): number => new Date(e.time).getTime();

const isErrorLevel = (level: string) => level === 'error' || level === 'fatal' || level === 'panic';

const titleCase = (s: string) => s.replace(/_/g, ' ').replace(/^\w/, (c) => c.toUpperCase());

const contains = (outer: TraceSpan, inner: TraceSpan): boolean =>
    new Date(outer.start_time).getTime() <= new Date(inner.start_time).getTime() &&
    new Date(outer.end_time).getTime() >= new Date(inner.end_time).getTime();

// A failover attempt and the single upstream call inside it are the same
// event described twice — same status, near-identical duration, one named by
// service id and the other by host. Fold that pair so the journey states it
// once, keeping both sets of facts.
//
// An attempt that made *several* upstream calls keeps them as their own
// lines: an MCP tool loop turns one attempt into a call per iteration, and
// folding would silently drop all but one of them.
const foldUpstreamIntoAttempts = (spans: TraceSpan[]): (TraceSpan & { upstreamHost?: string })[] => {
    const attempts = spans.filter((s) => s.name === 'failover.attempt');
    if (attempts.length === 0) return spans;

    const insideAttempt = new Map<string, TraceSpan[]>();
    for (const span of spans) {
        if (span.name !== 'upstream') continue;
        const parent = attempts.find((a) => contains(a, span));
        if (!parent) continue;
        insideAttempt.set(parent.span_id, [...(insideAttempt.get(parent.span_id) || []), span]);
    }

    const foldedInto = new Map<string, TraceSpan>();
    for (const [attemptID, upstreams] of insideAttempt) {
        if (upstreams.length === 1) foldedInto.set(attemptID, upstreams[0]);
    }
    const absorbed = new Set([...foldedInto.values()].map((s) => s.span_id));

    return spans
        .filter((s) => !absorbed.has(s.span_id))
        .map((s) => {
            const upstream = foldedInto.get(s.span_id);
            if (!upstream) return s;
            return {
                ...s,
                attributes: { ...(s.attributes || {}), ...(upstream.attributes || {}) },
                // Carried separately so describeSpan can show it as a fact.
                upstreamHost: upstream.attributes?.['server.address'],
            };
        });
};

// Presentation for the span names the gateway emits. Anything unrecognized
// falls through to its raw span name rather than being hidden.
const describeSpan = (span: TraceSpan & { upstreamHost?: string }): { label: string; facts: string[]; badge?: string } => {
    const attrs = span.attributes || {};
    const status = attrs['http.response.status_code'];
    const service = attrs['tingly.lb.service_id'];
    switch (span.name) {
        case 'routing':
            return {
                label: 'Routing',
                facts: [service, attrs['tingly.lb.tactic']].filter(Boolean) as string[],
            };
        case 'failover.attempt':
            return {
                label: `Attempt ${attrs['tingly.failover.attempt'] || '?'}`,
                facts: [service, span.upstreamHost].filter(Boolean) as string[],
                badge: status,
            };
        case 'upstream':
            return {
                label: 'Upstream',
                facts: [attrs['server.address']].filter(Boolean) as string[],
                badge: status,
            };
        default:
            return { label: span.name, facts: [] };
    }
};

const buildStages = (events: ModelRequestEvent[], spans: TraceSpan[]): JourneyStage[] => {
    // The root span is the request itself — the table row already states its
    // outcome, so it never takes a line of its own.
    const children = spans.filter((s) => spans.some((p) => p.span_id === s.parent_span_id));
    const stageSpans = foldUpstreamIntoAttempts(children.length > 0 ? children : spans.length > 1 ? spans : []);

    const spanStages: JourneyStage[] = stageSpans
        .map((span) => {
            const { label, facts, badge } = describeSpan(span);
            return {
                key: span.span_id,
                label,
                facts,
                badge,
                status: spanStatus(span.status_code),
                start: new Date(span.start_time).getTime(),
                end: new Date(span.end_time).getTime(),
                measured: true,
                attributes: span.attributes,
                annotations: [] as ModelRequestEvent[],
            };
        })
        .sort((a, b) => a.start - b.start);

    // The access log envelope repeats the row header verbatim (status,
    // latency, path), so it earns no line — unless it is all we have, as
    // when a request dies in auth before any stage runs.
    const staged = events.filter((e) => e.source !== 'http');
    const relevant = staged.length > 0 ? staged : events;

    const leftovers: ModelRequestEvent[] = [];
    for (const event of relevant) {
        const at = eventTime(event);
        const target = spanStages.find((s) => at >= s.start && at <= s.end);
        if (target) target.annotations.push(event);
        else leftovers.push(event);
    }

    // Events with no measured stage (transform steps, or everything when
    // tracing is off) still get a line, grouped by the stage they name. One
    // whose stage names a measured stage joins it rather than opening a
    // duplicate — event and span clocks can drift at the boundaries.
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
                label: titleCase(key),
                facts: [],
                status: 'unset',
                start: eventTime(event),
                end: eventTime(event),
                measured: false,
                annotations: [],
            };
            derived.set(key, stage);
        }
        stage.end = Math.max(stage.end, eventTime(event));
        stage.annotations.push(event);
        if (isErrorLevel(event.level)) stage.status = 'error';
    }

    return [...spanStages, ...derived.values()].sort((a, b) => a.start - b.start);
};

const StageIcon = ({ status }: { status: StageStatus }) => {
    if (status === 'error') return <ErrorOutline sx={{ fontSize: 14, color: 'error.main' }} />;
    if (status === 'ok') return <CheckCircle sx={{ fontSize: 14, color: 'success.main' }} />;
    return <Circle sx={{ fontSize: 6, color: 'text.disabled' }} />;
};

// The smart-routing evaluation explains *why* a service was chosen. It is
// the one payload worth rendering richly — but on demand: it is four lines
// per rule, and most reads of this view are not about routing.
const RoutingRules = ({ fields }: { fields?: Record<string, any> }) => {
    const rules: any[] = Array.isArray(fields?.trace) ? fields!.trace : [];
    if (rules.length === 0) return null;
    const matchedIdx = typeof fields?.matched_rule_index === 'number' ? fields!.matched_rule_index : -1;
    return (
        <Stack spacing={0.25} sx={{ mt: 0.25 }}>
            {rules.map((rule: any) => {
                const isWinner = matchedIdx === rule.rule_index;
                return (
                    <Box key={rule.rule_index}>
                        <Typography sx={{ fontSize: '0.7rem', color: isWinner ? 'success.main' : 'text.disabled' }}>
                            {isWinner ? '✓' : '·'} #{rule.rule_index} {rule.description || '(no description)'}
                        </Typography>
                        {Array.isArray(rule.ops) &&
                            rule.ops.map((op: any, i: number) => (
                                <Typography
                                    key={i}
                                    sx={{ fontFamily: 'monospace', fontSize: '0.68rem', color: 'text.disabled', pl: 1.5 }}
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

const annotationColor = (level: string) =>
    isErrorLevel(level) ? 'error.main' : level === 'warning' ? 'warning.main' : 'text.secondary';

// "rule matched" says nothing the stage line has not already said. What the
// reader actually wants to know is how much evaluation stands behind the
// choice — and that there is more to see on click.
const annotationMessage = (event: ModelRequestEvent): string => {
    const rules = event.fields?.trace;
    if (event.source !== 'smart_routing' || !Array.isArray(rules) || rules.length === 0) {
        return event.message;
    }
    const matched = typeof event.fields?.matched_rule_index === 'number' ? event.fields.matched_rule_index : -1;
    const noun = rules.length === 1 ? 'rule' : 'rules';
    return matched >= 0
        ? `${rules.length} routing ${noun} evaluated · #${matched} matched`
        : `${rules.length} routing ${noun} evaluated · no match`;
};

const AnnotationLine = ({ event }: { event: ModelRequestEvent }) => {
    const fields = event.fields || {};
    const keys = Object.keys(fields).filter((k) => !SUPPRESSED_FIELDS.has(k));
    return (
        <Stack direction="row" spacing={1} sx={{ alignItems: 'baseline', flexWrap: 'wrap' }}>
            <Typography sx={{ fontSize: '0.72rem', color: annotationColor(event.level) }}>
                {annotationMessage(event)}
            </Typography>
            {keys.length > 0 && (
                <Typography sx={{ fontFamily: 'monospace', fontSize: '0.68rem', color: 'text.disabled', wordBreak: 'break-all' }}>
                    {keys.map((k) => `${k}=${formatFieldValue(k, fields[k])}`).join('  ')}
                </Typography>
            )}
        </Stack>
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

    return (
        <Box
            sx={{
                display: 'grid',
                // icon · stage · facts · status · duration — one grid for the
                // whole journey so every column, including the annotations
                // under each stage, lines up on the same edges.
                gridTemplateColumns: ' 16px 88px 1fr auto 68px',
                columnGap: 1,
                rowGap: 0.15,
                alignItems: 'center',
            }}
        >
            {stages.map((stage) => {
                const open = openStage === stage.key;
                const attrKeys = stage.attributes ? Object.keys(stage.attributes).sort() : [];
                const routingFields = stage.annotations.find((e) => e.source === 'smart_routing')?.fields;
                const expandable = attrKeys.length > 0 || Array.isArray(routingFields?.trace);
                const rowSx = {
                    py: 0.35,
                    cursor: expandable ? 'pointer' : 'default',
                    backgroundColor: open ? 'action.hover' : 'transparent',
                };
                const onClick = () => expandable && setOpenStage(open ? null : stage.key);

                // A stage with nothing to state — a derived one, or any stage
                // when tracing is off — would otherwise render as a label
                // beside two empty cells with its text dangling on the next
                // line. Pull its first annotation up into the facts column so
                // the list stays dense and every line carries content.
                const inlineAnnotation = stage.facts.length === 0 ? stage.annotations[0] : undefined;
                const belowAnnotations = inlineAnnotation ? stage.annotations.slice(1) : stage.annotations;

                return [
                    <Box key={`${stage.key}-i`} onClick={onClick} sx={{ ...rowSx, display: 'flex', justifyContent: 'center' }}>
                        <StageIcon status={stage.status} />
                    </Box>,
                    <Box key={`${stage.key}-l`} onClick={onClick} sx={rowSx}>
                        <Typography sx={{ fontSize: '0.76rem', fontWeight: 500, color: stage.measured ? 'text.primary' : 'text.secondary' }}>
                            {stage.label}
                        </Typography>
                    </Box>,
                    <Box key={`${stage.key}-f`} onClick={onClick} sx={{ ...rowSx, minWidth: 0 }}>
                        {inlineAnnotation ? (
                            <AnnotationLine event={inlineAnnotation} />
                        ) : (
                            <Typography
                                sx={{
                                    fontFamily: 'monospace',
                                    fontSize: '0.72rem',
                                    color: 'text.secondary',
                                    whiteSpace: 'nowrap',
                                    overflow: 'hidden',
                                    textOverflow: 'ellipsis',
                                }}
                            >
                                {stage.facts.join('  ·  ')}
                            </Typography>
                        )}
                    </Box>,
                    <Box key={`${stage.key}-b`} onClick={onClick} sx={rowSx}>
                        {stage.badge && (
                            <Chip
                                size="small"
                                label={stage.badge}
                                color={stage.status === 'error' ? 'error' : 'default'}
                                sx={{ fontSize: '0.6rem', height: 16 }}
                            />
                        )}
                    </Box>,
                    <Box key={`${stage.key}-d`} onClick={onClick} sx={rowSx}>
                        <Typography sx={{ fontFamily: 'monospace', fontSize: '0.7rem', color: 'text.secondary', textAlign: 'right' }}>
                            {stage.measured ? formatDuration(stage.end - stage.start) : ''}
                        </Typography>
                    </Box>,

                    // Annotations and the expanded detail align under the facts
                    // column rather than starting a new indentation ladder.
                    <Box key={`${stage.key}-a`} sx={{ gridColumn: '3 / -1' }}>
                        {belowAnnotations.map((event, i) => (
                            <AnnotationLine key={i} event={event} />
                        ))}
                        <Collapse in={open} timeout="auto" unmountOnExit>
                            <Box sx={{ py: 0.5 }}>
                                <RoutingRules fields={routingFields} />
                                {attrKeys.map((k) => (
                                    <Typography key={k} sx={{ fontFamily: 'monospace', fontSize: '0.68rem', color: 'text.disabled', wordBreak: 'break-all' }}>
                                        {k}={stage.attributes![k]}
                                    </Typography>
                                ))}
                            </Box>
                        </Collapse>
                    </Box>,
                ];
            })}

            {traceMissing && traceId && (
                <Typography key="missing" variant="caption" sx={{ gridColumn: '2 / -1', color: 'text.disabled', fontStyle: 'italic' }}>
                    Stage timings unavailable — this trace is no longer in the in-memory buffer.
                </Typography>
            )}
        </Box>
    );
};

export default RequestJourney;
