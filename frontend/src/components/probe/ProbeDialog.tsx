import React, { useState, useEffect, memo, useCallback, useMemo } from 'react';
import {
    Dialog,
    DialogTitle,
    DialogContent,
    Box,
    Typography,
    Chip,
    LinearProgress,
    IconButton,
    Tooltip,
    Button,
    Collapse,
    Alert,
} from '@mui/material';
import {
    CheckCircle as CheckIcon,
    Error as ErrorIcon,
    Speed as SpeedIcon,
    Token as TokenIcon,
    ContentCopy as CopyIcon,
    Refresh as RefreshIcon,
    PlayArrow as RunIcon,
    ExpandMore as ExpandMoreIcon,
    ExpandLess as ExpandLessIcon,
    Terminal as TerminalIcon,
} from '@/components/icons';
import { useTheme } from '@mui/material/styles';
import { useTranslation } from 'react-i18next';
import type { ProbeResult, ProbeThinking, ProbeProtocol, ProbeTargetType } from '@/types/probe.ts';
import type { Provider } from '@/types/provider';
import { runProbe, buildProbeCurl, type ProbeCurlResult } from './runProbe';
import {
    type ProbeAxes,
    resolveInitialAxes,
    protocolAvailability,
    scopeAvailable,
} from './probeConfig';
import { ProbeControls } from './ProbeControls';
import { useCopyFeedback } from '@/hooks/useCopyFeedback';
import api from '@/services/api';
import { ProbeDevControls } from './ProbeDialogDevControls';

// ── Types ────────────────────────────────────────────────────────────────

interface ProbeDialogProps {
    open: boolean;
    onClose: () => void;
    targetType: ProbeTargetType;
    targetId: string;
    targetName: string;
    scenario?: string;
    model?: string;
    /** Initial thinking effort; overrides the persisted config when given. */
    thinkingLevel?: ProbeThinking;
    /** Provider record when the caller already holds it; fetched by UUID otherwise. */
    provider?: Provider | null;
    /** Pre-computed result to show on open (e.g. from the quick test); re-run replaces it. */
    initialResult?: ProbeResult;
    /** Called with every fresh result this dialog produces (including re-runs), so a caller holding its own copy (e.g. a card's persistent status badge) stays in sync. */
    onResult?: (result: ProbeResult) => void;
}

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

const defaultToolMessage = "Please use the bash tool to list the current directory contents with 'ls -la'.";
const defaultPlainMessage = 'Hello, this is a test message. Please respond with a short greeting.';
const defaultMessage = (tool: boolean): string => (tool ? defaultToolMessage : defaultPlainMessage);

// ruleProtocolForScenario derives the (locked) protocol a rule target speaks,
// mirroring the backend's ScenarioEndpoint scenario→api-style mapping.
function ruleProtocolForScenario(scenario?: string): ProbeProtocol {
    const base = (scenario || 'openai').split(':')[0];
    return ['anthropic', 'claude_code', 'opencode'].includes(base) ? 'anthropic_v1' : 'openai_chat';
}

// extractText pulls the assistant's text out of the raw (JSON-marshaled) SDK
// response so the user sees plain words instead of a serialized object. Returns
// '' when the shape isn't recognized — the caller falls back to raw JSON.
const extractText = (content?: string): string => {
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

const JourneyRow = memo(({ label, value, muted }: { label: string; value: React.ReactNode; muted?: boolean }) => {
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
interface CollapsibleSectionProps {
    title: string;
    defaultExpanded?: boolean;
    children: React.ReactNode;
}

const CollapsibleSection = memo(({ title, defaultExpanded = false, children }: CollapsibleSectionProps) => {
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
const CopyBlock = memo(({ text, maxHeight, fontSize = '0.78rem' }: { text: string; maxHeight?: number; fontSize?: string }) => {
    const { t } = useTranslation();
    const { copied, copy } = useCopyFeedback();
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
            <Tooltip title={copied ? t('probe.copied') : t('probe.copy')}>
                <IconButton
                    size="small"
                    onClick={() => copy(text)}
                    sx={{ position: 'absolute', top: 4, right: 4, color: 'text.secondary' }}
                >
                    <CopyIcon fontSize="small" />
                </IconButton>
            </Tooltip>
        </Box>
    );
});

// StatusBar: the one-glance verdict — success/failure, latency, tokens.
const StatusBar = memo(({ result }: { result: ProbeResult }) => {
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
const Journey = memo(
    ({
        result,
        targetType,
        targetName,
        scenario,
        model,
        bypassed,
    }: {
        result: ProbeResult;
        targetType: ProbeTargetType;
        targetName: string;
        scenario?: string;
        model?: string;
        bypassed: boolean;
    }) => {
        const { t } = useTranslation();
        const d = result.data;
        const isRule = targetType === 'rule';
        const provider = d?.selected_provider || (isRule ? '' : targetName);
        const routedModel = d?.selected_model || model || '';
        const ruleLabel = d?.matched_rule_desc || targetName;
        const endpoint = d?.upstream_api ? UPSTREAM_API_LABELS[d.upstream_api] || d.upstream_api : '';
        const pending = t('probe.pending');

        return (
            <Box>
                {isRule && (
                    <JourneyRow label={t('probe.row.rule')} value={`${ruleLabel}${scenario ? `  ·  ${scenario}` : ''}`} />
                )}
                {isRule && (
                    <JourneyRow
                        label={t('probe.row.flags')}
                        value={d?.applied_flags || t('probe.flagsNone')}
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

// ── Main dialog ──────────────────────────────────────────────────────────

export const ProbeDialog: React.FC<ProbeDialogProps> = ({
    open,
    onClose,
    targetType,
    targetId,
    targetName,
    scenario,
    model,
    thinkingLevel,
    provider,
    initialResult,
    onResult,
}) => {
    const { t } = useTranslation();
    const [axes, setAxes] = useState<ProbeAxes>(() =>
        resolveInitialAxes({ targetType, thinkingLevel, initialResult, provider: provider ?? null }),
    );
    const [message, setMessage] = useState('');
    const [isLoading, setIsLoading] = useState(false);
    const [result, setResult] = useState<ProbeResult | null>(null);
    const { copied: copyTooltipOpen, copy: copyText } = useCopyFeedback();
    const [providerInfo, setProviderInfo] = useState<Provider | null>(provider ?? null);
    const [curl, setCurl] = useState<ProbeCurlResult | null>(null);
    const [curlLoading, setCurlLoading] = useState(false);

    // Provider record for the Protocol axis: use the prop when the caller has
    // it, otherwise fetch by UUID (provider targets only).
    useEffect(() => {
        if (provider !== undefined) {
            setProviderInfo(provider);
            return;
        }
        if (!open || targetType !== 'provider') return;
        let cancelled = false;
        api.getProvider(targetId)
            .then((r: any) => {
                if (!cancelled && r?.success && r.data) setProviderInfo(r.data);
            })
            .catch(() => {});
        return () => {
            cancelled = true;
        };
    }, [open, provider, targetType, targetId]);

    // Reset on open — do NOT auto-run; the user clicks Run Test. State comes
    // from the association chain (props → initialResult → defaults), so the
    // toggles always describe the result on screen. Axes are deliberately not
    // persisted across opens — a probe's defaults must be predictable.
    useEffect(() => {
        if (open) {
            setAxes(resolveInitialAxes({ targetType, thinkingLevel, initialResult, provider: providerInfo }));
            setMessage('');
            setResult(initialResult ?? null);
            setIsLoading(false);
            setCurl(null);
        }
        // providerInfo intentionally excluded — a late provider load only
        // clamps the protocol axis via the effect below.
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [open, thinkingLevel, initialResult]);

    // Clamp the protocol axis when the provider record arrives (or changes):
    // a result-echoed protocol that this target can't speak falls back to the
    // concrete default.
    const protoAvail = useMemo(() => protocolAvailability(providerInfo), [providerInfo]);
    useEffect(() => {
        setAxes((prev) => {
            if (protoAvail.locked && prev.protocol !== protoAvail.default) {
                return { ...prev, protocol: protoAvail.default };
            }
            if (!protoAvail.locked && prev.protocol && !protoAvail.options.includes(prev.protocol)) {
                return { ...prev, protocol: protoAvail.default };
            }
            if (!protoAvail.locked && prev.protocol === '' && protoAvail.default) {
                // No "Auto" — materialize the concrete primary protocol.
                return { ...prev, protocol: protoAvail.default };
            }
            return prev;
        });
    }, [protoAvail]);

    // Protocol axis per target type: providers reduce to what they can speak;
    // rule targets are locked to their scenario's protocol.
    const protocolControl = useMemo(() => {
        if (targetType === 'rule') {
            const value = ruleProtocolForScenario(scenario);
            return { value, options: [value], locked: true, disabled: false, lockHint: t('probe.protocolLockedRule') };
        }
        if (providerInfo?.api_style === 'google') {
            return { value: axes.protocol, options: [], locked: true, disabled: true, lockHint: t('probe.protocolGoogle') };
        }
        return {
            value: axes.protocol,
            options: protoAvail.options,
            locked: protoAvail.locked,
            disabled: false,
            lockHint: t('probe.protocolLockedProvider'),
        };
    }, [targetType, scenario, providerInfo, axes.protocol, protoAvail, t]);

    const scopeDisabled = !scopeAvailable(targetType);
    const scopeHint = scopeDisabled ? t('probe.scopeRuleLocked') : t('probe.scopeHint');

    // buildBody is the single request constructor shared by Run Test and the
    // cURL section — the two can never disagree about what would be sent.
    const buildBody = useCallback(
        () => ({
            target_type: targetType,
            ...(targetType === 'rule'
                ? { scenario: scenario || 'openai', rule_uuid: targetId }
                : {
                      provider_uuid: targetId,
                      model: model || '',
                      direct: axes.direct,
                      ...(axes.protocol ? { protocol: axes.protocol } : {}),
                      ...(scenario ? { scenario } : {}),
                  }),
            stream: axes.stream,
            tool: axes.tool,
            thinking: axes.thinking,
            ...(message ? { message } : {}),
        }),
        [targetType, scenario, targetId, model, axes, message],
    );

    const runTest = useCallback(async () => {
        setIsLoading(true);
        setResult(null);
        const res = await runProbe(buildBody());
        setResult(res);
        setIsLoading(false);
        onResult?.(res);
    }, [buildBody, onResult]);

    // cURL section: regenerate (debounced) from the current config while the
    // dialog is open. Pure construction — nothing is executed.
    const bodyDeps = useMemo(() => JSON.stringify(buildBody()), [buildBody]);
    useEffect(() => {
        if (!open) return;
        let cancelled = false;
        setCurlLoading(true);
        const timer = setTimeout(async () => {
            const res = await buildProbeCurl(buildBody());
            if (cancelled) return;
            setCurl(res);
            setCurlLoading(false);
        }, 500);
        return () => {
            cancelled = true;
            clearTimeout(timer);
            setCurlLoading(false);
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [open, bodyDeps]);

    const copyCurl = useCallback(async () => {
        let command = curl?.data?.command;
        if (!command) {
            const res = await buildProbeCurl(buildBody());
            command = res.data?.command;
        }
        if (!command) return;
        copyText(command);
    }, [curl, buildBody, copyText]);

    const handleCopy = () => {
        if (!result) return;
        copyText(JSON.stringify(result, null, 2));
    };

    const bypassed = targetType === 'provider' && axes.direct;
    const extracted = useMemo(() => extractText(result?.data?.content), [result?.data?.content]);

    return (
        <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth slotProps={{
            // Resizable like a window: the user pulls the corner when they need
            // more room for the journey/cURL instead of being stuck at md.
            paper: { sx: { minHeight: 460, minWidth: 560, resize: 'both', overflow: 'hidden' } }
        }}>
            <DialogTitle sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 1 }}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap', minWidth: 0, overflow: 'hidden' }}>
                    <Typography
                        variant="subtitle1"
                        sx={{
                            fontWeight: 600,
                            overflow: 'hidden',
                            textOverflow: 'ellipsis',
                            whiteSpace: 'nowrap'
                        }}>
                        {model ? `${targetName} · ${model}` : targetName}
                    </Typography>
                </Box>
                <Box sx={{ display: 'flex', gap: 0.5, alignItems: 'center' }}>
                    <ProbeDevControls
                        targetName={targetName}
                        model={model}
                        stream={axes.stream}
                        onSimulate={(r) => {
                            setResult(r);
                            setIsLoading(false);
                        }}
                    />
                    <Tooltip
                        title={copyTooltipOpen ? t('probe.copied') : t('probe.curlCopy')}
                        open={copyTooltipOpen || undefined}
                        disableHoverListener={copyTooltipOpen}
                    >
                        <IconButton onClick={copyCurl} size="small" sx={{ color: 'text.secondary' }}>
                            <TerminalIcon fontSize="small" />
                        </IconButton>
                    </Tooltip>
                    {result && (
                        <>
                            <Tooltip title={t('probe.copyResponse')}>
                                <IconButton onClick={handleCopy} size="small" sx={{ color: 'text.secondary' }}>
                                    <CopyIcon fontSize="small" />
                                </IconButton>
                            </Tooltip>
                            <Tooltip title={t('probe.rerun')}>
                                <IconButton onClick={runTest} size="small" sx={{ color: 'text.secondary' }} disabled={isLoading}>
                                    <RefreshIcon fontSize="small" />
                                </IconButton>
                            </Tooltip>
                        </>
                    )}
                    <Button
                        variant="contained"
                        size="small"
                        startIcon={isLoading ? null : <RunIcon fontSize="small" />}
                        onClick={runTest}
                        disabled={isLoading}
                        sx={{ minWidth: 100 }}
                    >
                        {isLoading ? t('probe.running') : t('probe.run')}
                    </Button>
                </Box>
            </DialogTitle>
            <DialogContent sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                {/* Control rail + results — controls are what you send, results are
                 *  what you asked; the split keeps the results as the anchor. */}
                <Box sx={{ display: 'flex', gap: 2, alignItems: 'stretch', minHeight: 0, flex: 1 }}>
                {/* Control rail: the instrument panel — what will be sent. */}
                <Box
                    sx={{
                        width: 300,
                        flexShrink: 0,
                        border: '1px solid',
                        borderColor: 'divider',
                        borderRadius: 1.5,
                        p: 1.5,
                        alignSelf: 'stretch',
                        overflowY: 'auto',
                    }}
                >
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1.5 }}>
                        {t('probe.requestConfig')}
                    </Typography>
                    {/* Orthogonal axes — shape × tool × thinking × protocol × scope (+ message) */}
                    <ProbeControls
                        axes={axes}
                        onAxesChange={setAxes}
                        message={message}
                        onMessageChange={setMessage}
                        messagePlaceholder={defaultMessage(axes.tool)}
                        protocol={protocolControl}
                        scopeDisabled={scopeDisabled}
                        scopeHint={scopeHint}
                    />
                </Box>

                {/* Results column: the subject — what came back and how it went. */}
                <Box sx={{ flex: 1, minWidth: 0, overflowY: 'auto', display: 'flex', flexDirection: 'column' }}>
                    {isLoading && <LinearProgress sx={{ height: 6, borderRadius: 3, mt: 1.5 }} />}

                    {!isLoading && !result && (
                        <Box
                            sx={{
                                flex: 1,
                                display: 'flex',
                                flexDirection: 'column',
                                justifyContent: 'center',
                                alignItems: 'center',
                                border: '1px dashed',
                                borderColor: 'divider',
                                borderRadius: 2,
                                px: 3,
                                py: 6,
                                textAlign: 'center',
                            }}
                        >
                            <Typography variant="body2" sx={{ fontWeight: 600 }}>
                                {t('probe.emptyTitle')}
                            </Typography>
                            <Typography variant="caption" sx={{ color: 'text.secondary', display: 'block', mt: 0.5 }}>
                                {t('probe.emptyBody')}
                            </Typography>
                        </Box>
                    )}

                    {!isLoading && result && (
                        <Box>
                        <StatusBar result={result} />

                        <CollapsibleSection title={t('probe.journey')} defaultExpanded={false}>
                            <Journey
                                result={result}
                                targetType={targetType}
                                targetName={targetName}
                                scenario={scenario}
                                model={model}
                                bypassed={bypassed}
                            />
                        </CollapsibleSection>

                        {result.success && (
                            <CollapsibleSection title={t('probe.response')} defaultExpanded={false}>
                                <CopyBlock text={extracted || t('probe.noText')} maxHeight={180} />
                            </CollapsibleSection>
                        )}

                        {result.success && result.data?.content && (
                            <CollapsibleSection title={t('probe.rawJson')} defaultExpanded={false}>
                                <CopyBlock text={result.data.content} maxHeight={240} fontSize="0.72rem" />
                            </CollapsibleSection>
                        )}
                    </Box>
                )}
                </Box>
                </Box>

                {/* cURL spans the full dialog width — its lines are long, and it
                 *  belongs to the whole panel (config + target), not the results. */}
                <CollapsibleSection title={t('probe.curl')} defaultExpanded={false}>
                    {curlLoading && <LinearProgress sx={{ height: 4, borderRadius: 2 }} />}
                    {!curlLoading && curl?.data?.command && (
                        <Box>
                            <CopyBlock text={curl.data.command} maxHeight={240} fontSize="0.72rem" />
                            <Typography variant="caption" sx={{ color: 'text.secondary', display: 'block', mt: 1 }}>
                                {t('probe.curlKeyHint', { key: curl.data.key_env_var })}
                            </Typography>
                        </Box>
                    )}
                    {!curlLoading && curl && !curl.success && (
                        <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                            {curl.error?.message || t('probe.curlFailed')}
                        </Typography>
                    )}
                </CollapsibleSection>
            </DialogContent>
        </Dialog>
    );
};

export default ProbeDialog;





