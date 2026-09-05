import React, { memo, useState } from 'react';
import { Box, Typography, Chip, Collapse, Alert } from '@mui/material';
import {
    CheckCircle as CheckIcon,
    Error as ErrorIcon,
    Speed as SpeedIcon,
    Token as TokenIcon,
    ExpandMore as ExpandMoreIcon,
    ExpandLess as ExpandLessIcon,
} from '@/components/icons';
import { useTheme } from '@mui/material/styles';
import { useTranslation } from 'react-i18next';
import type { ProbeResult, ProbeProtocol, ProbeTargetType } from '@/types/probe';
import { CopyIconButton } from '@/components/CopyIconButton';

// ResultSections: the probe result vocabulary shared by the probe dialog and
// the Playground page — one-glance verdict (StatusBar), the request's path
// through TB (Journey), the collapsible artifact sections and the copyable
// monospace block. Both surfaces render the same result the same way; only
// the layout around these pieces differs (.design/playground.md §8).

// ── Constants / helpers ────────────────────────────────────────────────────

// Human-friendly labels for routing_source values from the backend.
const ROUTING_SOURCE_LABELS: Record<string, string> = {
    affinity: 'Session Affinity',
    smart_routing: 'Smart Routing',
    load_balancer: 'Load Balancer',
    probe_pin: 'Pinned (probe)',
};

// Human-friendly labels for the resolved upstream API type. Brand-first —
// the same vocabulary the Protocol axis uses (bare "Responses"/"Messages"
// assume SDK knowledge users don't have).
const UPSTREAM_API_LABELS: Record<string, string> = {
    openai_chat: 'OpenAI Chat',
    openai_responses: 'OpenAI Responses',
    anthropic_v1: 'Anthropic Messages',
    anthropic_beta: 'Anthropic Messages (beta)',
    google: 'GenerateContent',
};

export const defaultToolMessage = "Please use the bash tool to list the current directory contents with 'ls -la'.";
export const defaultPlainMessage = 'Hello, this is a test message. Please respond with a short greeting.';
export const defaultMessage = (tool: boolean): string => (tool ? defaultToolMessage : defaultPlainMessage);

// ruleProtocolForScenario derives the (locked) protocol a rule target speaks,
// mirroring the backend's ScenarioEndpoint scenario→api-style mapping.
export function ruleProtocolForScenario(scenario?: string): ProbeProtocol {
    const base = (scenario || 'openai').split(':')[0];
    return ['anthropic', 'claude_code', 'opencode'].includes(base) ? 'anthropic_v1' : 'openai_chat';
}

// extractText pulls the assistant's text out of the raw (JSON-marshaled) SDK
// response so the user sees plain words instead of a serialized object. Returns
// '' when the shape isn't recognized — the caller falls back to raw JSON.
export const extractText = (content?: string): string => {
    if (!content) return '';
    try {
        const data = JSON.parse(content);
        if (Array.isArray(data)) {
            // Streaming: concat OpenAI chat deltas and/or Anthropic text deltas.
            let text = '';
            for (const ch of data) {
                text += ch?.choices?.[0]?.delta?.content ?? '';
                text += ch?.delta?.text ?? '';
            }
            return text;
        }
        // OpenAI chat (non-stream)
        if (data?.choices?.[0]?.message?.content) return data.choices[0].message.content;
        // Anthropic messages
        if (Array.isArray(data?.content)) {
            return data.content
                .filter((b: any) => b?.type === 'text')
                .map((b: any) => b.text)
                .join('');
        }
        // OpenAI Responses
        if (Array.isArray(data?.output)) {
            let t = '';
            for (const o of data.output) {
                if (Array.isArray(o?.content)) {
                    t += o.content
                        .filter((c: any) => c?.text)
                        .map((c: any) => c.text)
                        .join('');
                }
            }
            return t;
        }
    } catch {
        // not JSON — fall through
    }
    return '';
};

// ── Sub-components ──────────────────────────────────────────────────────────

export const JourneyRow = memo(({ label, value, muted }: { label: string; value: React.ReactNode; muted?: boolean }) => {
    const theme = useTheme();
    return (
        <Box sx={{ display: 'flex', alignItems: 'baseline', py: 0.75, borderBottom: `1px solid ${theme.palette.divider}` }}>
            <Typography sx={{ width: 92, flexShrink: 0, color: 'text.secondary', fontSize: '0.78rem' }}>
                {label}
            </Typography>
            <Box
                sx={{
                    flex: 1,
                    minWidth: 0,
                    fontFamily: 'monospace',
                    fontSize: '0.78rem',
                    color: muted ? 'text.disabled' : 'text.primary',
                    wordBreak: 'break-all',
                }}
            >
                {value}
            </Box>
        </Box>
    );
});

// CollapsibleSection: a section with title and expand/collapse functionality
export interface CollapsibleSectionProps {
    title: string;
    defaultExpanded?: boolean;
    children: React.ReactNode;
}

export const CollapsibleSection = memo(({ title, defaultExpanded = false, children }: CollapsibleSectionProps) => {
    const [expanded, setExpanded] = useState(defaultExpanded);
    const theme = useTheme();

    return (
        <Box
            sx={{
                mt: 2,
                border: `1px solid ${theme.palette.divider}`,
                borderRadius: 1.5,
                overflow: 'hidden',
            }}
        >
            <Box
                sx={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    px: 1.5,
                    py: 1,
                    bgcolor: 'action.hover',
                    cursor: 'pointer',
                    '&:hover': {
                        bgcolor: 'action.selected',
                    },
                }}
                onClick={() => setExpanded(!expanded)}
            >
                <Typography
                    variant="subtitle2"
                    sx={{
                        fontWeight: 600,
                        color: "text.primary"
                    }}>
                    {title}
                </Typography>
                {expanded ? <ExpandLessIcon /> : <ExpandMoreIcon />}
            </Box>
            <Collapse in={expanded}>
                <Box sx={{ p: 1.5 }}>{children}</Box>
            </Collapse>
        </Box>
    );
});

// CopyBlock: a monospace content panel with its own copy affordance — every
// artifact section (cURL, Response, Raw JSON) hands over the exact text.
export const CopyBlock = memo(({ text, maxHeight, fontSize = '0.78rem' }: { text: string; maxHeight?: number | string; fontSize?: string }) => {
    const { t } = useTranslation();
    return (
        <Box sx={{ position: 'relative' }}>
            <Box
                sx={{
                    p: 1.5,
                    pr: 5,
                    bgcolor: 'background.default',
                    color: 'text.primary',
                    borderRadius: 1.5,
                    fontFamily: 'monospace',
                    fontSize,
                    whiteSpace: 'pre-wrap',
                    wordBreak: 'break-all',
                    maxHeight,
                    overflow: 'auto',
                }}
            >
                {text}
            </Box>
            <CopyIconButton
                value={text}
                label={t('probe.copy')}
                copiedLabel={t('probe.copied')}
                sx={{ position: 'absolute', top: 4, right: 4 }}
            />
        </Box>
    );
});

// StatusBar: the one-glance verdict — success/failure, latency, tokens.
export const StatusBar = memo(({ result }: { result: ProbeResult }) => {
    const theme = useTheme();
    const { t } = useTranslation();
    const ok = result.success;
    const d = result.data;
    return (
        <Alert
            severity={ok ? 'success' : 'error'}
            variant="outlined"
            sx={{
                mt: 2,
                borderRadius: 2,
                borderWidth: 2,
                '& .MuiAlert-icon': {
                    fontSize: 28,
                },
            }}
            icon={ok ? <CheckIcon sx={{ fontSize: 28 }} /> : <ErrorIcon sx={{ fontSize: 28 }} />}
        >
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, flexWrap: 'wrap' }}>
                <Typography
                    variant="subtitle1"
                    sx={{
                        fontWeight: 700,
                        fontSize: '1rem'
                    }}>
                    {ok ? t('probe.success') : t('probe.failed')}
                </Typography>
                {d?.latency_ms ? (
                    <Chip
                        icon={<SpeedIcon sx={{ fontSize: 16 }} />}
                        label={`${d.latency_ms}ms`}
                        size="medium"
                        sx={{
                            height: 28,
                            bgcolor: ok ? 'success.main' : 'error.main',
                            color: 'common.white',
                            '& .MuiChip-icon': {
                                color: 'common.white',
                            },
                        }}
                    />
                ) : null}
                {(() => {
                    // Canonical TokenUsage total = input + output (cache tracked
                    // separately, not added). Mirrors protocol.TokenUsage.TotalTokens().
                    const total =
                        (d?.usage?.input_tokens || 0) + (d?.usage?.output_tokens || 0);
                    if (!total) return null;
                    return (
                        <Chip
                            icon={<TokenIcon sx={{ fontSize: 16 }} />}
                            label={`${total} tokens`}
                            size="medium"
                            sx={{
                                height: 28,
                                bgcolor: ok ? 'success.main' : 'error.main',
                                color: 'common.white',
                                '& .MuiChip-icon': {
                                    color: 'common.white',
                                },
                            }}
                        />
                    );
                })()}
            </Box>
            {!ok && result.error && (
                <Typography
                    variant="body2"
                    sx={{
                        fontFamily: 'monospace',
                        fontSize: '0.85rem',
                        mt: 1,
                        color: 'text.primary',
                        wordBreak: 'break-word',
                    }}
                >
                    {result.error.message}
                </Typography>
            )}
        </Alert>
    );
});

// Journey: the request's path through TB — rule, routing, provider, endpoint.
// Fields the backend doesn't yet bubble up render as greyed placeholders.
export const Journey = memo(
    ({
        result,
        targetType,
        targetName,
        scenario,
        model,
        bypassed,
        showFlags,
        flagsExtra,
        ruleExtra,
    }: {
        result: ProbeResult;
        targetType: ProbeTargetType;
        targetName: string;
        scenario?: string;
        model?: string;
        bypassed: boolean;
        /** Render the Flags row even for non-rule targets (default: rule targets only). */
        showFlags?: boolean;
        /** Extra content under the applied-flags value (e.g. the playground's overlay note). */
        flagsExtra?: React.ReactNode;
        /** Extra content under the rule value (e.g. a matched-a-different-rule warning). */
        ruleExtra?: React.ReactNode;
    }) => {
        const { t } = useTranslation();
        const d = result.data;
        const isRule = targetType === 'rule';
        const flagsRow = showFlags ?? isRule;
        const provider = d?.selected_provider || (isRule ? '' : targetName);
        const routedModel = d?.selected_model || model || '';
        const ruleLabel = d?.matched_rule_desc || targetName;
        const endpoint = d?.upstream_api ? UPSTREAM_API_LABELS[d.upstream_api] || d.upstream_api : '';
        const pending = t('probe.pending');

        return (
            <Box>
                {isRule && (
                    <JourneyRow
                        label={t('probe.row.rule')}
                        value={
                            ruleExtra ? (
                                <Box>
                                    <Box component="span">{`${ruleLabel}${scenario ? `  ·  ${scenario}` : ''}`}</Box>
                                    {ruleExtra}
                                </Box>
                            ) : (
                                `${ruleLabel}${scenario ? `  ·  ${scenario}` : ''}`
                            )
                        }
                    />
                )}
                {flagsRow && !bypassed && (
                    <JourneyRow
                        label={t('probe.row.flags')}
                        value={
                            flagsExtra ? (
                                <Box>
                                    <Box component="span">{d?.applied_flags || t('probe.flagsNone')}</Box>
                                    {flagsExtra}
                                </Box>
                            ) : (
                                d?.applied_flags || t('probe.flagsNone')
                            )
                        }
                        muted={!d?.applied_flags}
                    />
                )}
                {bypassed ? (
                    <JourneyRow label={t('probe.scope')} value={t('probe.directValue')} />
                ) : (
                    <JourneyRow
                        label={t('probe.row.routing')}
                        value={
                            d?.routing_source
                                ? `${ROUTING_SOURCE_LABELS[d.routing_source] || d.routing_source}${
                                      d.matched_smart_rule !== undefined && d.matched_smart_rule >= 0
                                          ? `  ·  smart rule #${d.matched_smart_rule}`
                                          : ''
                                  }`
                                : pending
                        }
                        muted={!d?.routing_source}
                    />
                )}
                <JourneyRow
                    label={t('probe.row.provider')}
                    value={provider ? `${provider}${routedModel ? `  →  ${routedModel}` : ''}` : pending}
                    muted={!provider}
                />
                <JourneyRow label={t('probe.row.endpoint')} value={endpoint || pending} muted={!endpoint} />
                <JourneyRow label={t('probe.row.upstreamUrl')} value={d?.upstream_url || pending} muted={!d?.upstream_url} />
                {d?.request_url && <JourneyRow label={t('probe.row.requestUrl')} value={d.request_url} />}
            </Box>
        );
    }
);

