import React, { useState, useEffect, memo, useCallback, useMemo } from 'react';
import {
    Dialog,
    DialogTitle,
    DialogContent,
    Box,
    Typography,
    Chip,
    LinearProgress,
    Alert,
    IconButton,
    Tooltip,
    Button,
    ToggleButton,
    ToggleButtonGroup,
    Collapse,
} from '@mui/material';
import {
    CheckCircle as CheckIcon,
    Error as ErrorIcon,
    Speed as SpeedIcon,
    Token as TokenIcon,
    ContentCopy as CopyIcon,
    Refresh as RefreshIcon,
    PlayArrow as RunIcon,
} from '@/components/icons';
import { useTheme } from '@mui/material/styles';
import { useTranslation } from 'react-i18next';
import { toggleButtonGroupStyle } from '@/styles/toggleStyles';
import type { ProbeV2TestMode, ProbeV2TargetType } from '@/types/probe-v2.ts';

// ── Types ────────────────────────────────────────────────────────────────

interface ProbeResultData {
    content?: string;
    latency_ms: number;
    request_url?: string;
    stream?: boolean;
    prompt_tokens?: number;
    completion_tokens?: number;
    total_tokens?: number;
    tool_calls?: Array<{ id: string; name: string; input: Record<string, unknown> }>;
    // Routing trace — populated for TB-loopback probes.
    selected_provider?: string;
    selected_provider_uuid?: string;
    selected_model?: string;
    routing_source?: string;
    matched_smart_rule?: number;
    // Execution-level facts (real upstream endpoint, matched rule, applied flags).
    upstream_api?: string;
    upstream_url?: string;
    matched_rule?: string;
    matched_rule_desc?: string;
    applied_flags?: string;
}

interface ProbeResult {
    success: boolean;
    error?: { message: string; type: string };
    data?: ProbeResultData;
}

interface ProbeV2DialogProps {
    open: boolean;
    onClose: () => void;
    targetType: ProbeV2TargetType;
    targetId: string;
    targetName: string;
    scenario?: string;
    model?: string;
    /** Initial request shape; can be changed inside the dialog. Defaults to stream. */
    testMode?: ProbeV2TestMode;
}

// ── Constants / helpers ────────────────────────────────────────────────────

// Request shapes the probe exercises (the "形态" dimension): we only need
// non-streaming vs streaming. ('simple' is the backend's value for nonstream.)
const MODES: { value: ProbeV2TestMode; tKey: string }[] = [
    { value: 'simple', tKey: 'probe.nonstream' },
    { value: 'streaming', tKey: 'probe.stream' },
];

// Human-friendly labels for routing_source values from the backend.
const ROUTING_SOURCE_LABELS: Record<string, string> = {
    affinity: 'Session Affinity',
    smart_routing: 'Smart Routing',
    load_balancer: 'Load Balancer',
    probe_pin: 'Pinned (probe)',
};

// Human-friendly labels for the resolved upstream API type.
const UPSTREAM_API_LABELS: Record<string, string> = {
    openai_chat: 'Chat Completions',
    openai_responses: 'Responses',
    anthropic_v1: 'Messages',
    anthropic_beta: 'Messages (beta)',
    google: 'GenerateContent',
};

const getUserAuthToken = (): string | null => localStorage.getItem('user_auth_token');

const defaultMessage = (mode: ProbeV2TestMode): string =>
    mode === 'tool'
        ? "Please use the bash tool to list the current directory contents with 'ls -la'."
        : 'Hello, this is a test message. Please respond with a short greeting.';

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

const SectionTitle = ({ children }: { children: React.ReactNode }) => (
    <Typography variant="body2" sx={{ fontWeight: 600, color: 'primary.main', mt: 2, mb: 0.5 }}>
        {children}
    </Typography>
);

// StatusBar: the one-glance verdict — success/failure, latency, tokens.
const StatusBar = memo(({ result }: { result: ProbeResult }) => {
    const theme = useTheme();
    const { t } = useTranslation();
    const ok = result.success;
    const d = result.data;
    return (
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
            {ok ? (
                <CheckIcon sx={{ color: theme.palette.success.main, fontSize: 24 }} />
            ) : (
                <ErrorIcon sx={{ color: theme.palette.error.main, fontSize: 24 }} />
            )}
            <Typography variant="subtitle2" fontWeight={600}>
                {ok ? t('probe.success') : t('probe.failed')}
            </Typography>
            {d?.latency_ms ? (
                <Chip icon={<SpeedIcon sx={{ fontSize: 14 }} />} label={`${d.latency_ms}ms`} size="small" sx={{ height: 24 }} />
            ) : null}
            {d?.total_tokens ? (
                <Chip
                    icon={<TokenIcon sx={{ fontSize: 14 }} />}
                    label={`${d.total_tokens} tokens`}
                    size="small"
                    variant="outlined"
                    sx={{ height: 24 }}
                />
            ) : null}
        </Box>
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
        targetType: ProbeV2TargetType;
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
                <SectionTitle>{t('probe.journey')}</SectionTitle>
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

export const ProbeV2Dialog: React.FC<ProbeV2DialogProps> = ({
    open,
    onClose,
    targetType,
    targetId,
    targetName,
    scenario,
    model,
    testMode = 'streaming',
}) => {
    const { t } = useTranslation();
    const [mode, setMode] = useState<ProbeV2TestMode>(testMode);
    // 范围: false = 经过 TB (default), true = 直连上游. Provider targets only.
    const [direct, setDirect] = useState(false);
    const [isLoading, setIsLoading] = useState(false);
    const [result, setResult] = useState<ProbeResult | null>(null);
    const [rawExpanded, setRawExpanded] = useState(false);
    const [copyTooltipOpen, setCopyTooltipOpen] = useState(false);

    // Reset on open — do NOT auto-run; the user clicks 运行测试.
    useEffect(() => {
        if (open) {
            setMode(testMode);
            setDirect(false);
            setResult(null);
            setIsLoading(false);
            setRawExpanded(false);
        }
    }, [open, testMode]);

    const runTest = useCallback(async () => {
        setIsLoading(true);
        setResult(null);
        setRawExpanded(false);

        const body = {
            target_type: targetType,
            ...(targetType === 'rule'
                ? { scenario: scenario || 'openai', rule_uuid: targetId }
                : { provider_uuid: targetId, model: model || '', direct }),
            test_mode: mode,
            message: defaultMessage(mode),
        };

        const token = getUserAuthToken();
        const headers: Record<string, string> = { 'Content-Type': 'application/json' };
        if (token) headers['Authorization'] = `Bearer ${token}`;

        try {
            const response = await fetch('/api/v2/probe', {
                method: 'POST',
                headers,
                body: JSON.stringify(body),
            });
            if (!response.ok) {
                let message = `HTTP ${response.status}`;
                try {
                    const e = await response.json();
                    message = e.error?.message || message;
                } catch {
                    /* ignore */
                }
                setResult({ success: false, error: { message, type: 'http_error' } });
                return;
            }
            setResult(await response.json());
        } catch (err: any) {
            setResult({ success: false, error: { message: err?.message || 'Probe failed', type: 'client_error' } });
        } finally {
            setIsLoading(false);
        }
    }, [targetType, scenario, targetId, model, direct, mode]);

    const handleCopy = () => {
        if (!result) return;
        navigator.clipboard.writeText(JSON.stringify(result, null, 2)).then(() => {
            setCopyTooltipOpen(true);
            setTimeout(() => setCopyTooltipOpen(false), 2000);
        });
    };

    const bypassed = targetType === 'provider' && direct;
    const extracted = useMemo(() => extractText(result?.data?.content), [result?.data?.content]);

    return (
        <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth PaperProps={{ sx: { minHeight: 420 } }}>
            <DialogTitle sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 1 }}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap', minWidth: 0 }}>
                    <Typography variant="subtitle1" fontWeight={600}>
                        {targetType === 'rule' ? t('probe.testRule') : t('probe.testProvider')}
                    </Typography>
                    <Chip
                        label={model ? `${targetName} | ${model}` : targetName}
                        size="small"
                        variant="outlined"
                        sx={{ fontFamily: 'monospace', fontSize: '0.75rem', maxWidth: 360 }}
                    />
                </Box>
                {result && (
                    <Box sx={{ display: 'flex', gap: 0.5 }}>
                        <Tooltip
                            title={copyTooltipOpen ? t('probe.copied') : t('probe.copyResponse')}
                            open={copyTooltipOpen || undefined}
                            disableHoverListener={copyTooltipOpen}
                        >
                            <IconButton onClick={handleCopy} size="small" sx={{ color: 'text.secondary' }}>
                                <CopyIcon fontSize="small" />
                            </IconButton>
                        </Tooltip>
                        <Tooltip title={t('probe.rerun')}>
                            <IconButton onClick={runTest} size="small" sx={{ color: 'text.secondary' }} disabled={isLoading}>
                                <RefreshIcon fontSize="small" />
                            </IconButton>
                        </Tooltip>
                    </Box>
                )}
            </DialogTitle>

            <DialogContent>
                {/* Controls: 形态 (request shape) + 范围 (scope) + run */}
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 3, flexWrap: 'wrap', mb: 1 }}>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        <Typography variant="caption" color="text.secondary">
                            {t('probe.shape')}
                        </Typography>
                        <ToggleButtonGroup
                            size="small"
                            exclusive
                            value={mode}
                            onChange={(_, v) => v && setMode(v)}
                            sx={toggleButtonGroupStyle}
                        >
                            {MODES.map((m) => (
                                <ToggleButton key={m.value} value={m.value}>
                                    {t(m.tKey)}
                                </ToggleButton>
                            ))}
                        </ToggleButtonGroup>
                    </Box>

                    {targetType === 'provider' && (
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                            <Tooltip title={t('probe.scopeHint')}>
                                <Typography variant="caption" color="text.secondary">
                                    {t('probe.scope')}
                                </Typography>
                            </Tooltip>
                            <ToggleButtonGroup
                                size="small"
                                exclusive
                                value={direct ? 'direct' : 'tb'}
                                onChange={(_, v) => v && setDirect(v === 'direct')}
                                sx={toggleButtonGroupStyle}
                            >
                                <ToggleButton value="tb">{t('probe.throughTB')}</ToggleButton>
                                <ToggleButton value="direct">{t('probe.direct')}</ToggleButton>
                            </ToggleButtonGroup>
                        </Box>
                    )}

                    <Button
                        variant="contained"
                        size="small"
                        startIcon={<RunIcon fontSize="small" />}
                        onClick={runTest}
                        disabled={isLoading}
                        sx={{ ml: 'auto' }}
                    >
                        {isLoading ? t('probe.running') : t('probe.run')}
                    </Button>
                </Box>

                {isLoading && <LinearProgress sx={{ height: 6, borderRadius: 3, mt: 1 }} />}

                {!isLoading && !result && (
                    <Box sx={{ textAlign: 'center', py: 8 }}>
                        <Typography variant="body2" color="text.secondary">
                            {t('probe.runHint')}
                        </Typography>
                    </Box>
                )}

                {!isLoading && result && (
                    <Box>
                        <Box sx={{ mt: 1 }}>
                            <StatusBar result={result} />
                        </Box>

                        {!result.success && result.error && (
                            <Alert severity="error" variant="outlined" sx={{ mt: 2, borderRadius: 2 }}>
                                <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.8rem' }}>
                                    {result.error.message}
                                </Typography>
                                {result.error.type && (
                                    <Typography variant="caption" color="text.secondary">
                                        Type: {result.error.type}
                                    </Typography>
                                )}
                            </Alert>
                        )}

                        <Journey
                            result={result}
                            targetType={targetType}
                            targetName={targetName}
                            scenario={scenario}
                            model={model}
                            bypassed={bypassed}
                        />

                        {result.success && (
                            <Box>
                                <SectionTitle>{t('probe.response')}</SectionTitle>
                                <Box
                                    sx={{
                                        p: 1.5,
                                        bgcolor: 'grey.50',
                                        borderRadius: 1.5,
                                        fontFamily: 'monospace',
                                        fontSize: '0.8rem',
                                        whiteSpace: 'pre-wrap',
                                        wordBreak: 'break-word',
                                        maxHeight: 180,
                                        overflow: 'auto',
                                    }}
                                >
                                    {extracted || t('probe.noText')}
                                </Box>
                                {result.data?.content && (
                                    <>
                                        <Button size="small" onClick={() => setRawExpanded((v) => !v)} sx={{ mt: 0.5, textTransform: 'none' }}>
                                            {rawExpanded ? t('probe.rawJsonHide') : t('probe.rawJson')}
                                        </Button>
                                        <Collapse in={rawExpanded}>
                                            <Box
                                                sx={{
                                                    p: 1.5,
                                                    bgcolor: 'grey.50',
                                                    borderRadius: 1.5,
                                                    fontFamily: 'monospace',
                                                    fontSize: '0.72rem',
                                                    whiteSpace: 'pre-wrap',
                                                    wordBreak: 'break-all',
                                                    maxHeight: 240,
                                                    overflow: 'auto',
                                                }}
                                            >
                                                {result.data.content}
                                            </Box>
                                        </Collapse>
                                    </>
                                )}
                            </Box>
                        )}
                    </Box>
                )}
            </DialogContent>
        </Dialog>
    );
};

export default ProbeV2Dialog;
