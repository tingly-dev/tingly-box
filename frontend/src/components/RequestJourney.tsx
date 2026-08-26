import { Box, Chip, Collapse, Stack, Typography } from '@mui/material';
import { Fragment, useEffect, useMemo, useState } from 'react';
import type { ModelRequestEvent } from '@/components/AILogViewer';

// RequestJourney is the single answer to "how did this request go".
//
// It replaces what used to be two parallel accounts of the same story (a trace
// waterfall plus a raw event timeline) that the reader had to join in their
// head — see .design/ux-principles.md §1.
//
// Everything the request produced — measured trace stages and log lines alike
// — is rendered as ONE row shape, in time order:
//
//     [KIND]  name        detail                    → result      12 ms
//
// The kind badge carries what the record is, so subordination never needs an
// indentation ladder; time order already puts a stage's log lines directly
// under it. One shape means one way to read the list and one way to open a
// detail, instead of the four the page used to mix (table row, stage row,
// annotation sub-line, attribute dump).
//
// It degrades on purpose: with tracing off (or the trace evicted from the
// in-memory ring) the log rows stand alone — same shape, minus the timings.

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

type RowTone = 'ok' | 'error' | 'warning' | 'plain';

// One row per record. `stage` rows come from trace spans and carry a measured
// duration; `log` rows come from the pipeline's log lines and do not.
interface JourneyRow {
    key: string;
    kind: 'stage' | 'log';
    name: string;
    detail: string;
    result?: string;
    tone: RowTone;
    start: number;
    durationMs?: number;
    // Full payload, rendered the same way whatever the row is.
    payload: [string, string][];
    // Smart routing's rule-by-rule evaluation: the one payload with real
    // structure, kept structured but shown inside the same detail panel.
    rules?: RoutingRule[];
    matchedRule?: number;
}

interface RoutingRule {
    rule_index: number;
    description?: string;
    matched?: boolean;
    ops?: { position?: string; operation?: string; reason?: string }[];
}

interface RequestJourneyProps {
    events: ModelRequestEvent[];
    traceId?: string;
    getTrace: (traceId: string) => Promise<TraceDetail | null>;
}

// Fields already stated by the row header, the row line itself, or the rule
// breakdown. Repeating them in the payload is pure noise.
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
        if (key.endsWith('_ns')) return formatDuration(value / 1e6);
        if (key.endsWith('_ms')) return formatDuration(value);
    }
    if (typeof value === 'object' && value !== null) return JSON.stringify(value);
    return String(value);
};

const isErrorLevel = (level: string) => level === 'error' || level === 'fatal' || level === 'panic';

type TimedSpan = TraceSpan & { start: number; end: number };

const timed = (span: TraceSpan): TimedSpan => ({
    ...span,
    start: new Date(span.start_time).getTime(),
    end: new Date(span.end_time).getTime(),
});

// Presentation for the span names the gateway emits. Anything unrecognized
// falls through to its raw span name rather than being hidden.
const describeSpan = (span: TimedSpan): { name: string; detail: string } => {
    const attrs = span.attributes || {};
    const service = attrs['tingly.lb.service_id'];
    switch (span.name) {
        case 'routing':
            return { name: 'routing', detail: [service, attrs['tingly.lb.tactic']].filter(Boolean).join('  ·  ') };
        case 'failover.attempt':
            return { name: `attempt ${attrs['tingly.failover.attempt'] || '?'}`, detail: service || '' };
        default:
            return { name: span.name, detail: '' };
    }
};

const buildRows = (events: ModelRequestEvent[], spans: TraceSpan[]): JourneyRow[] => {
    // The root span is the request itself — the table row already states its
    // outcome, so it never takes a row of its own.
    const ids = new Set(spans.map((s) => s.span_id));
    const all = spans.map(timed);
    const children = all.filter((s) => s.parent_span_id && ids.has(s.parent_span_id));
    const stageSpans = children.length > 0 ? children : all.length > 1 ? all : [];

    const rows: JourneyRow[] = stageSpans.map((span) => {
        const { name, detail } = describeSpan(span);
        const attrs = span.attributes || {};
        return {
            key: span.span_id,
            kind: 'stage',
            name,
            detail,
            result: attrs['http.response.status_code'] || span.status_message,
            tone: span.status_code === 'Error' ? 'error' : span.status_code === 'Ok' ? 'ok' : 'plain',
            start: span.start,
            durationMs: span.end - span.start,
            payload: Object.entries(attrs).sort(([a], [b]) => a.localeCompare(b)),
        };
    });

    // The access log envelope repeats the row header verbatim (status, latency,
    // path), so it earns no row — unless it is all we have, as when a request
    // dies in auth before any stage runs.
    const staged = events.filter((e) => e.source !== 'http');
    const relevant = staged.length > 0 ? staged : events;

    relevant.forEach((event, i) => {
        const fields = event.fields || {};
        const rules = Array.isArray(fields.trace) ? (fields.trace as RoutingRule[]) : undefined;
        rows.push({
            key: `log:${i}`,
            kind: 'log',
            name: event.stage || event.source,
            // Smart routing's message ("rule matched") says nothing the stage
            // row has not; what the reader wants is how much evaluation stands
            // behind the choice, and that there is more on click.
            detail: rules
                ? `${rules.length} routing rule${rules.length === 1 ? '' : 's'} evaluated` +
                  (typeof fields.matched_rule_index === 'number' ? ` · #${fields.matched_rule_index} matched` : ' · no match')
                : event.message,
            tone: isErrorLevel(event.level) ? 'error' : event.level === 'warning' ? 'warning' : 'plain',
            start: new Date(event.time).getTime(),
            payload: Object.keys(fields)
                .filter((k) => !SUPPRESSED_FIELDS.has(k))
                .sort()
                .map((k) => [k, formatFieldValue(k, fields[k])] as [string, string]),
            rules,
            matchedRule: typeof fields.matched_rule_index === 'number' ? fields.matched_rule_index : -1,
        });
    });

    // Time order alone expresses the grouping: a stage's log lines fall inside
    // its window, so they land directly beneath it without any indentation.
    return rows.sort((a, b) => a.start - b.start);
};

const toneColor = (tone: RowTone): string =>
    tone === 'error' ? 'error.main' : tone === 'warning' ? 'warning.main' : 'text.secondary';

const KindBadge = ({ kind, tone }: { kind: JourneyRow['kind']; tone: RowTone }) => (
    <Chip
        size="small"
        label={kind}
        sx={{
            width: 58,
            height: 17,
            fontSize: '0.58rem',
            fontWeight: 700,
            letterSpacing: '0.04em',
            textTransform: 'uppercase',
            borderRadius: 0.5,
            color: tone === 'error' ? 'error.main' : kind === 'stage' ? 'primary.main' : 'text.disabled',
            backgroundColor: tone === 'error'
                ? 'rgba(211,47,47,0.10)'
                : kind === 'stage'
                    ? 'rgba(25,118,210,0.10)'
                    : 'action.hover',
            '& .MuiChip-label': { px: 0 },
        }}
    />
);

// The rule-by-rule evaluation is the one payload with real structure. It stays
// structured, but inside the same panel every other row opens into.
const RoutingRules = ({ rules, matched }: { rules: RoutingRule[]; matched: number }) => (
    <>
        {rules.map((rule) => (
            <Box key={rule.rule_index}>
                <Typography
                    sx={{
                        fontFamily: 'monospace',
                        fontSize: '0.68rem',
                        color: matched === rule.rule_index ? 'success.main' : 'text.disabled',
                    }}
                >
                    {matched === rule.rule_index ? '✓' : '·'} #{rule.rule_index} {rule.description || '(no description)'}
                </Typography>
                {rule.ops?.map((op, i) => (
                    <Typography
                        key={i}
                        sx={{ fontFamily: 'monospace', fontSize: '0.68rem', color: 'text.disabled', pl: 2 }}
                    >
                        {op.position}.{op.operation} — {op.reason}
                    </Typography>
                ))}
            </Box>
        ))}
    </>
);

const RequestJourney = ({ events, traceId, getTrace }: RequestJourneyProps) => {
    // undefined = still loading, null = evicted from the ring, object = loaded
    const [trace, setTrace] = useState<TraceDetail | null | undefined>(undefined);
    const [openRow, setOpenRow] = useState<string | null>(null);

    useEffect(() => {
        if (!traceId) return;
        let cancelled = false;
        getTrace(traceId)
            .then((detail) => {
                if (!cancelled) setTrace(detail);
            })
            .catch(() => {
                if (!cancelled) setTrace(null);
            });
        return () => {
            cancelled = true;
        };
    }, [traceId, getTrace]);

    const rows = useMemo(() => buildRows(events, trace?.spans ?? []), [events, trace]);

    if (rows.length === 0) {
        return (
            <Typography variant="body2" sx={{ color: 'text.secondary', fontStyle: 'italic' }}>
                Nothing recorded for this request.
            </Typography>
        );
    }

    return (
        <Box
            // The journey is span names, URLs, status codes and routing traces —
            // machine text throughout, so it stays LTR under an RTL locale
            // (see index.css).
            data-ltr
            sx={{
                display: 'grid',
                // kind · name · detail · result · duration — one grid for the
                // whole journey so every row, and every opened panel, lines up
                // on the same edges.
                gridTemplateColumns: 'auto 92px 1fr auto 64px',
                columnGap: 1,
                rowGap: 0.1,
                alignItems: 'center',
            }}
        >
            {rows.map((row) => {
                const open = openRow === row.key;
                const expandable = row.payload.length > 0 || !!row.rules;
                const cellSx = {
                    py: 0.32,
                    cursor: expandable ? 'pointer' : 'default',
                    backgroundColor: open ? 'action.hover' : 'transparent',
                };
                const onClick = () => expandable && setOpenRow(open ? null : row.key);
                const cell = { onClick, sx: cellSx };

                return (
                    <Fragment key={row.key}>
                        <Box {...cell} sx={{ ...cellSx, display: 'flex' }}>
                            <KindBadge kind={row.kind} tone={row.tone} />
                        </Box>
                        <Box {...cell}>
                            <Typography
                                sx={{
                                    fontFamily: 'monospace',
                                    fontSize: '0.73rem',
                                    fontWeight: 500,
                                    color: row.kind === 'stage' ? 'text.primary' : 'text.secondary',
                                    whiteSpace: 'nowrap',
                                    overflow: 'hidden',
                                    textOverflow: 'ellipsis',
                                }}
                            >
                                {row.name}
                            </Typography>
                        </Box>
                        <Box {...cell} sx={{ ...cellSx, minWidth: 0 }}>
                            <Typography
                                sx={{
                                    fontFamily: 'monospace',
                                    fontSize: '0.72rem',
                                    color: toneColor(row.tone),
                                    whiteSpace: 'nowrap',
                                    overflow: 'hidden',
                                    textOverflow: 'ellipsis',
                                }}
                            >
                                {row.detail}
                            </Typography>
                        </Box>
                        <Box {...cell}>
                            {row.result && (
                                <Typography
                                    sx={{
                                        fontFamily: 'monospace',
                                        fontSize: '0.72rem',
                                        color: row.tone === 'error' ? 'error.main' : 'text.secondary',
                                        whiteSpace: 'nowrap',
                                    }}
                                >
                                    → {row.result}
                                </Typography>
                            )}
                        </Box>
                        <Box {...cell}>
                            <Typography
                                sx={{ fontFamily: 'monospace', fontSize: '0.7rem', color: 'text.disabled', textAlign: 'right' }}
                            >
                                {row.durationMs != null ? formatDuration(row.durationMs) : ''}
                            </Typography>
                        </Box>

                        {/* One detail panel shape for every row, aligned under
                            the detail column rather than starting a new indent. */}
                        <Box sx={{ gridColumn: '3 / -1' }}>
                            <Collapse in={open} timeout="auto" unmountOnExit>
                                <Stack spacing={0.1} sx={{ py: 0.5 }}>
                                    {row.rules && <RoutingRules rules={row.rules} matched={row.matchedRule ?? -1} />}
                                    {row.payload.map(([k, v]) => (
                                        <Typography
                                            key={k}
                                            sx={{ fontFamily: 'monospace', fontSize: '0.68rem', color: 'text.disabled', wordBreak: 'break-all' }}
                                        >
                                            {k}={v}
                                        </Typography>
                                    ))}
                                </Stack>
                            </Collapse>
                        </Box>
                    </Fragment>
                );
            })}

            {trace === null && traceId && (
                <Typography key="missing" variant="caption" sx={{ gridColumn: '2 / -1', color: 'text.disabled', fontStyle: 'italic' }}>
                    Stage timings unavailable — this trace is no longer in the in-memory buffer.
                </Typography>
            )}
        </Box>
    );
};

export default RequestJourney;
