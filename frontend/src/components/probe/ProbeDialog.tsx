import React, { useState, useEffect, useCallback, useMemo } from 'react';
import {
    Dialog,
    DialogTitle,
    DialogContent,
    Box,
    Typography,
    LinearProgress,
    IconButton,
    Tooltip,
    Button,
} from '@mui/material';
import {
    ContentCopy as CopyIcon,
    Refresh as RefreshIcon,
    PlayArrow as RunIcon,
    Terminal as TerminalIcon,
    OpenInNew as OpenInPlaygroundIcon,
} from '@/components/icons';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import type { ProbeResult, ProbeThinking, ProbeTargetType } from '@/types/probe.ts';
import type { Provider } from '@/types/provider';
import { runProbe, buildProbeCurl, type ProbeCurlResult } from './runProbe';
import {
    type ProbeAxes,
    resolveInitialAxes,
    protocolAvailability,
    visionAvailable,
    scopeAvailable,
} from './probeConfig';
import { ProbeControls } from './ProbeControls';
import { useCopyFeedback } from '@/hooks/useCopyFeedback';
import api from '@/services/api';
import { ProbeDevControls } from './ProbeDialogDevControls';
import {
    StatusBar,
    Journey,
    CollapsibleSection,
    CopyBlock,
    extractText,
    defaultMessage,
    ruleProtocolForScenario,
} from './ResultSections';
import { playgroundDeepLink } from '@/pages/playground/playgroundLink';

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
    const navigate = useNavigate();
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
    const visionOk = visionAvailable(providerInfo);
    useEffect(() => {
        setAxes((prev) => {
            if (!visionOk && prev.vision !== 'none') {
                return { ...prev, vision: 'none' };
            }
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
    }, [protoAvail, visionOk]);

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
    const visionControl = useMemo(
        () => ({ disabled: !visionOk, hint: t('probe.visionGoogle') }),
        [visionOk, t],
    );

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
            ...(axes.vision !== 'none' ? { vision: axes.vision } : {}),
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
                    {/* Escalation path: found something in the quick diagnostic, go
                        deeper in the workbench with the same target and knobs. Saved
                        (unsaved-config) targets have no playground identity. */}
                    {targetType !== 'provider_config' && (
                        <Tooltip title={t('probe.openInPlayground')}>
                            <IconButton
                                size="small"
                                sx={{ color: 'text.secondary' }}
                                onClick={() => {
                                    onClose();
                                    navigate(playgroundDeepLink({ targetType, targetId, scenario, model, axes, message }));
                                }}
                            >
                                <OpenInPlaygroundIcon fontSize="small" />
                            </IconButton>
                        </Tooltip>
                    )}
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
            <DialogContent
                sx={{
                    display: 'flex',
                    flexDirection: 'column',
                    gap: 2,
                    overflowY: 'auto',
                    // Never squeeze sections to make content "fit": flex would
                    // otherwise crush the last section (the cURL block) to a
                    // sliver instead of letting the panel scroll.
                    '& > *': { flexShrink: 0 },
                }}
            >
                {/* Control rail + results — controls are what you send, results are
                 *  what you asked; the split keeps the results as the anchor.
                 *  DialogContent is the ONE vertical scroll container: content
                 *  flows to its natural height and the whole panel scrolls, so
                 *  a long section (vision-probe cURL bodies, raw stream JSON)
                 *  can never be clipped out of reach by nested fixed heights. */}
                <Box sx={{ display: 'flex', gap: 2, alignItems: 'flex-start', flex: 1 }}>
                {/* Control rail: the instrument panel — what will be sent. */}
                <Box
                    sx={{
                        width: 300,
                        flexShrink: 0,
                        border: '1px solid',
                        borderColor: 'divider',
                        borderRadius: 1.5,
                        p: 1.5,
                        // Stay in view while long results scroll past; scroll
                        // internally only when the window is shorter than the rail.
                        position: 'sticky',
                        top: 0,
                        maxHeight: '75vh',
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
                        vision={visionControl}
                    />
                </Box>

                {/* Results column: the subject — what came back and how it went. */}
                <Box sx={{ flex: 1, minWidth: 0, alignSelf: 'stretch', display: 'flex', flexDirection: 'column' }}>
                    {isLoading && <LinearProgress sx={{ height: 6, borderRadius: 3, mt: 1.5 }} />}

                    {!isLoading && !result && (
                        <Box
                            sx={{
                                flex: 1,
                                minHeight: 260,
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
                                <CopyBlock text={extracted || t('probe.noText')} maxHeight="40vh" />
                            </CollapsibleSection>
                        )}

                        {result.success && result.data?.content && (
                            <CollapsibleSection title={t('probe.rawJson')} defaultExpanded={false}>
                                <CopyBlock text={result.data.content} maxHeight="45vh" fontSize="0.72rem" />
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
                            <CopyBlock text={curl.data.command} maxHeight="45vh" fontSize="0.72rem" />
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





