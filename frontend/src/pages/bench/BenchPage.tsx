import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Box, Button, LinearProgress, Paper, Stack, Tooltip, Typography } from '@mui/material';
import { HelpOutline, PlayArrow as RunIcon, TestPipe as BenchIcon } from '@/components/icons';
import { useTranslation } from 'react-i18next';
import PageHeader from '@/components/PageHeader';
import api from '@/services/api';
import type { FlagSpec, RuleFlagsApi } from '@/components/RoutingGraphTypes';
import FlagCatalogDialog from '@/components/rule-card/FlagCatalogDialog';
import { apiToFlags, flagsToApi } from '@/components/rule-card/flagHelpers';
import type { Provider } from '@/types/provider';
import type { ProbeRequest, ProbeResult } from '@/types/probe';
import { runProbe, buildProbeCurl, type ProbeCurlResult } from '@/components/probe/runProbe';
import { protocolAvailability, visionAvailable } from '@/components/probe/probeConfig';
import type { ProbeProtocol } from '@/types/probe';
import { StatusBar, Journey, CollapsibleSection, CopyBlock, extractText, defaultMessage } from '@/components/probe/ResultSections';
import {
    BLANK_REQUEST,
    DEFAULT_STATE,
    buildProbeRequest,
    cloneState,
    isDirect,
    loadState,
    runLabel,
    saveState,
    targetKey,
    type BenchState,
    type RunRecord,
} from './benchState';
import { useTargetCatalog } from './useTargetCatalog';
import { TargetPicker } from './TargetPicker';
import { BenchAxes, useAxisAvailability, type RequestMode } from './BenchAxes';
import { RequestEditor } from './RequestEditor';
import { PayloadPanel } from './PayloadPanel';
import { RunHistory } from './RunHistory';

// BenchPage — the customizable end-to-end test workbench
// (.design/bench.md). Three columns answer the user's three questions:
// Compose (what do I send?) · Conversation + Result (what happened?) ·
// Payload (what actually goes out?). Request axes stay resident; Plugins uses
// the same compact card and catalog as the routing graph. Bench targets a
// real provider model directly — no Rule/deep-link jump-in from elsewhere;
// it is reached only through its own nav entry.

const MAX_RUNS = 10;

// useDebouncedCurl: rebuild the curl preview 500 ms after the last change to
// `request` — pure construction, so debouncing just avoids redundant work,
// never a stale result. Shared by the live payload and the preset-preview
// fetch below, which differ only in which request they track.
function useDebouncedCurl(request: ProbeRequest | null): { data: ProbeCurlResult | null; loading: boolean } {
    const [data, setData] = useState<ProbeCurlResult | null>(null);
    const [loading, setLoading] = useState(false);
    const key = useMemo(() => JSON.stringify(request), [request]);
    useEffect(() => {
        if (!request) { setData(null); setLoading(false); return; }
        let cancelled = false;
        setLoading(true);
        const timer = setTimeout(async () => {
            const res = await buildProbeCurl(request);
            if (cancelled) return;
            setData(res);
            setLoading(false);
        }, 500);
        return () => { cancelled = true; clearTimeout(timer); };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [key]);
    return { data, loading };
}

const prettyBody = (raw: string): string => {
    try {
        return JSON.stringify(JSON.parse(raw), null, 2);
    } catch {
        return raw;
    }
};

const BENCH_WORKSPACE_HEIGHT = 840;

const Panel: React.FC<{
    title: string;
    question: string;
    action?: React.ReactNode;
    scroll?: boolean;
    children: React.ReactNode;
}> = ({ title, question, action, scroll = false, children }) => (
    <Paper
        variant="outlined"
        sx={{
            overflow: 'hidden',
            bgcolor: 'background.paper',
            height: { lg: '100%' },
            ...(scroll && {
                display: { lg: 'flex' },
                flexDirection: { lg: 'column' },
                minHeight: 0,
            }),
        }}
    >
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, px: 1.75, py: 1, borderBottom: '1px solid', borderColor: 'divider', flexShrink: 0 }}>
            <Typography variant="overline" sx={{ fontSize: '0.62rem', color: 'text.secondary', lineHeight: 1 }}>{title}</Typography>
            <Typography variant="caption" sx={{ color: 'text.disabled' }}>{question}</Typography>
            <Box sx={{ flex: 1 }} />
            {action}
        </Box>
        <Box
            sx={{
                p: 1.75,
                ...(scroll && {
                    flex: { lg: 1 },
                    minHeight: 0,
                    overflowY: { lg: 'auto' },
                    overscrollBehavior: { lg: 'contain' },
                    scrollbarGutter: { lg: 'stable' },
                }),
            }}
        >
            {children}
        </Box>
    </Paper>
);

const BenchPage: React.FC = () => {
    const { t } = useTranslation();
    const catalog = useTargetCatalog();

    const [state, setState] = useState<BenchState>(() => loadState() ?? cloneState(DEFAULT_STATE));
    const [registry, setRegistry] = useState<FlagSpec[]>([]);
    const [registryLoading, setRegistryLoading] = useState(true);
    const [pluginCatalogOpen, setPluginCatalogOpen] = useState(false);
    const [running, setRunning] = useState(false);
    const [shown, setShown] = useState<{ result: ProbeResult; snapshot: BenchState } | null>(null);
    const [runs, setRuns] = useState<RunRecord[]>([]);
    const [activeRun, setActiveRun] = useState<string | null>(null);

    const patch = useCallback((p: Partial<BenchState>) => setState((s) => ({ ...s, ...p })), []);

    useEffect(() => saveState(state), [state]);

    // Flag registry — the single source of truth for the Plugins panel.
    useEffect(() => {
        let cancelled = false;
        api.getRuleFlagRegistry()
            .then((r: any) => { if (!cancelled && r?.success && Array.isArray(r.data)) setRegistry(r.data); })
            .catch(() => {})
            .finally(() => { if (!cancelled) setRegistryLoading(false); });
        return () => { cancelled = true; };
    }, []);

    const { target } = state;
    const provider: Provider | null = useMemo(
        () => (target ? catalog.providers.find((p) => p.uuid === target.providerUuid) ?? null : null),
        [catalog.providers, target],
    );

    // Axis availability per target, and the clamp that keeps the axes legal
    // when the target (or its provider record) changes — same rules as the
    // probe dialog.
    const availability = useAxisAvailability(target, provider, state.axes);
    useEffect(() => {
        setState((s) => {
            const a = { ...s.axes };
            let changed = false;
            if (provider) {
                const avail = protocolAvailability(provider);
                if (avail.locked && a.protocol !== avail.default) { a.protocol = avail.default; changed = true; }
                else if (!avail.locked && a.protocol && !avail.options.includes(a.protocol)) { a.protocol = avail.default; changed = true; }
                else if (!avail.locked && a.protocol === '' && avail.default) { a.protocol = avail.default; changed = true; }
                if (!visionAvailable(provider) && a.vision !== 'none') { a.vision = 'none'; changed = true; }
            }
            return changed ? { ...s, axes: a } : s;
        });
    }, [target, provider]);

    const built = useMemo(() => buildProbeRequest(state), [state]);
    const request = built.request;
    const direct = isDirect(state);
    const pluginFlags = useMemo(() => apiToFlags(state.flags as RuleFlagsApi), [state.flags]);

    useEffect(() => {
        if (direct) setPluginCatalogOpen(false);
    }, [direct]);

    // Protocols a hand-written request may be in for this target: whatever
    // the provider speaks.
    const rawProtocolOptions = useMemo<ProbeProtocol[]>(() => {
        const avail = protocolAvailability(provider);
        return avail.options.length ? avail.options : ['openai_chat', 'openai_responses', 'anthropic_v1'];
    }, [provider]);

    // Live payload: pure construction, so debouncing just avoids redundant work.
    const { data: curl, loading: curlLoading } = useDebouncedCurl(request);

    // Preset preview: the same construction, but with raw forced off, so
    // "Copy the preset request" stays accurate once a custom request is
    // active. Only runs while raw is active — otherwise `request` above
    // already is the preset request and this would just duplicate the fetch.
    const presetPreviewRequest = useMemo(() => (state.raw ? buildProbeRequest({ ...state, raw: null }).request : null), [state]);
    const { data: presetPreviewCurl } = useDebouncedCurl(presetPreviewRequest);

    const seedBody = state.raw
        ? (presetPreviewCurl?.success && presetPreviewCurl.data?.body ? prettyBody(presetPreviewCurl.data.body) : undefined)
        : (curl?.success && curl.data?.body ? prettyBody(curl.data.body) : undefined);

    // Request mode and Protocol are unified, single controls in Compose now
    // (.design/bench.md §1) — mode is just whether raw is set; Protocol's
    // value/options/onChange are resolved per mode here, since preset reads
    // axes.protocol (via availability) and custom reads raw.protocol.
    const mode: RequestMode = state.raw ? 'custom' : 'preset';
    const protocolValue: ProbeProtocol = state.raw?.protocol ?? (state.axes.protocol || protocolAvailability(provider).default || 'openai_chat');
    const onModeChange = (next: RequestMode) => {
        if (next === mode) return;
        if (next === 'custom') {
            patch({ raw: { protocol: protocolValue, body: seedBody ?? JSON.stringify(BLANK_REQUEST[protocolValue], null, 2) } });
        } else {
            // Carry the protocol forward so the unified control doesn't
            // silently change value just because mode flipped back.
            patch({ raw: null, axes: { ...state.axes, protocol: state.raw!.protocol } });
        }
    };
    const onProtocolChange = (p: ProbeProtocol) => {
        if (p === protocolValue) return;
        if (state.raw) {
            // Same move as picking a different template — a new protocol
            // means a new starting body, never a relabeled old one
            // (.design/bench.md §6.3).
            patch({ raw: { protocol: p, body: JSON.stringify(BLANK_REQUEST[p], null, 2) } });
        } else {
            patch({ axes: { ...state.axes, protocol: p } });
        }
    };

    const run = useCallback(async () => {
        if (!request || running) return;
        const snapshot = cloneState(state);
        setRunning(true);
        setShown(null);
        const result = await runProbe(request);
        const record: RunRecord = { id: `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`, at: Date.now(), result, snapshot, label: runLabel(snapshot) };
        setRuns((prev) => [record, ...prev].slice(0, MAX_RUNS));
        setActiveRun(record.id);
        setShown({ result, snapshot });
        setRunning(false);
    }, [request, running, state]);

    const runRef = useRef(run);
    runRef.current = run;
    useEffect(() => {
        const onKey = (e: KeyboardEvent) => {
            if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
                e.preventDefault();
                void runRef.current();
            }
        };
        window.addEventListener('keydown', onKey);
        return () => window.removeEventListener('keydown', onKey);
    }, []);

    const selectRun = (record: RunRecord) => {
        setActiveRun(record.id);
        setState(cloneState(record.snapshot));
        setShown({ result: record.result, snapshot: record.snapshot });
    };

    const setTarget = (next: BenchState['target']) => {
        if (targetKey(next) === targetKey(state.target)) return;
        // New target, new baseline: overlays and hand edits were composed
        // against the old one.
        patch({ target: next, flags: {}, headers: {}, raw: null });
        setActiveRun(null);
    };

    const targetName = provider?.name ?? '';
    const shownOverlay = shown ? Object.entries(shown.snapshot.flags) : [];
    const extracted = useMemo(() => extractText(shown?.result.data?.content), [shown?.result.data?.content]);

    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            <PageHeader
                title={
                    <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center' }}>
                        <Box component="span">{t('bench.title', { defaultValue: 'Bench' })}</Box>
                        <Tooltip
                            arrow
                            placement="right"
                            title={t('bench.subtitle', {
                                defaultValue: 'One probe request with every knob resident: pick a provider model, shape the request, overlay flags, edit the payload.',
                            })}
                        >
                            <HelpOutline
                                aria-label={t('bench.about', { defaultValue: 'About Bench' })}
                                tabIndex={0}
                                sx={{ fontSize: 17, color: 'text.disabled', cursor: 'help' }}
                            />
                        </Tooltip>
                    </Stack>
                }
                icon={<BenchIcon sx={{ fontSize: 26 }} />}
                actions={
                    <Button variant="contained" startIcon={<RunIcon />} onClick={run} disabled={!request || running} sx={{ minWidth: 120 }} title={t('bench.runHint', { defaultValue: '⌘ / Ctrl + Enter' })}>
                        {running ? t('bench.running', { defaultValue: 'Running…' }) : t('bench.run', { defaultValue: 'Run' })}
                        <Box component="kbd" sx={{ ml: 1, fontFamily: 'monospace', fontSize: '0.65rem', opacity: 0.75, border: '1px solid', borderColor: 'rgba(255,255,255,.4)', borderRadius: 0.5, px: 0.5 }}>⌘↵</Box>
                    </Button>
                }
            />

            <RunHistory runs={runs} activeId={activeRun} onSelect={selectRun} />

            <Box
                sx={{
                    display: 'grid',
                    gap: 2,
                    alignItems: 'stretch',
                    height: { lg: BENCH_WORKSPACE_HEIGHT },
                    gridTemplateColumns: { xs: '1fr', md: 'minmax(0, 1fr) minmax(0, 1fr)', lg: '300px minmax(0, 1fr) minmax(360px, 420px)' },
                }}
            >
                {/* ① what do I send? */}
                <Box sx={{ minWidth: 0, height: { lg: '100%' } }}>
                    <Panel title={t('bench.compose', { defaultValue: 'Compose' })} question={t('bench.composeQ', { defaultValue: 'what do I send?' })}>
                        <Stack spacing={1.5}>
                            <Box>
                                <Typography variant="caption" sx={{ color: 'text.secondary', fontWeight: 500, display: 'block', mb: 0.5 }}>
                                    {t('bench.target', { defaultValue: 'Target' })}
                                </Typography>
                                <TargetPicker catalog={catalog} value={state.target} onChange={setTarget} />
                            </Box>
                            <BenchAxes
                                axes={state.axes}
                                onChange={(axes) => patch({ axes })}
                                availability={availability}
                                mode={mode}
                                onModeChange={onModeChange}
                                customProtocolOptions={rawProtocolOptions}
                                protocolValue={protocolValue}
                                onProtocolChange={onProtocolChange}
                                pluginFlags={pluginFlags}
                                pluginRegistry={registry}
                                pluginsActive={!direct}
                                onOpenPlugins={() => {
                                    if (!direct) setPluginCatalogOpen(true);
                                }}
                            />
                        </Stack>
                    </Panel>
                    <FlagCatalogDialog
                        open={pluginCatalogOpen}
                        flags={pluginFlags}
                        registry={registry}
                        loading={registryLoading}
                        providers={catalog.providers}
                        onClose={() => setPluginCatalogOpen(false)}
                        onSave={(next) => {
                            patch({ flags: { ...flagsToApi(next) } });
                            setPluginCatalogOpen(false);
                        }}
                    />
                </Box>

                {/* ② the request itself · ③ what happened? */}
                <Box sx={{ minWidth: 0, height: { lg: '100%' } }}>
                    <Panel scroll title={t('bench.requestPanel', { defaultValue: 'Request' })} question={t('bench.requestQ', { defaultValue: 'what the client sends' })}>
                        <Stack spacing={2}>
                            <RequestEditor
                                message={state.message}
                                onMessageChange={(message) => patch({ message })}
                                raw={state.raw}
                                onRawChange={(raw) => patch({ raw })}
                                seedBody={seedBody}
                                error={state.raw ? built.error : undefined}
                                messagePlaceholder={defaultMessage(state.axes.tool)}
                            />
                            <Box sx={{ pt: 1.5, borderTop: '1px solid', borderColor: 'divider' }}>
                                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1.25 }}>
                                    <Typography variant="overline" sx={{ fontSize: '0.62rem', color: 'text.secondary', lineHeight: 1 }}>
                                        {t('bench.result', { defaultValue: 'Result' })}
                                    </Typography>
                                    <Typography variant="caption" sx={{ color: 'text.disabled' }}>
                                        {t('bench.resultQ', { defaultValue: 'what happened?' })}
                                    </Typography>
                                </Box>
                                {running && <LinearProgress sx={{ height: 6, borderRadius: 3 }} />}
                                {!running && !shown && (
                                    <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                                        {t('bench.resultEmpty', { defaultValue: 'Not run yet — press Run to send exactly this request.' })}
                                    </Typography>
                                )}
                                {!running && shown && (
                                    <Box sx={{ mt: -2 }}>
                                        <StatusBar result={shown.result} />
                                        <CollapsibleSection title={t('probe.journey')} defaultExpanded>
                                            <Journey
                                                result={shown.result}
                                                targetType="provider"
                                                targetName={targetName}
                                                model={shown.snapshot.target?.model}
                                                bypassed={isDirect(shown.snapshot)}
                                                showFlags
                                                flagsExtra={
                                                    shownOverlay.length > 0 ? (
                                                        <Typography variant="caption" sx={{ display: 'block', color: 'primary.main', fontFamily: 'inherit', mt: 0.25 }}>
                                                            {t('bench.overlaySent', { flags: shownOverlay.map(([k, v]) => `${k}=${JSON.stringify(v)}`).join(', '), defaultValue: 'overlay sent: {{flags}}' })}
                                                        </Typography>
                                                    ) : undefined
                                                }
                                            />
                                        </CollapsibleSection>
                                        {shown.result.success && (
                                            <CollapsibleSection title={t('probe.response')} defaultExpanded={false}>
                                                <CopyBlock text={extracted || t('probe.noText')} maxHeight="40vh" />
                                            </CollapsibleSection>
                                        )}
                                        {shown.result.success && shown.result.data?.content && (
                                            <CollapsibleSection title={t('probe.rawJson')} defaultExpanded={false}>
                                                <CopyBlock text={shown.result.data.content} maxHeight="45vh" fontSize="0.72rem" />
                                            </CollapsibleSection>
                                        )}
                                    </Box>
                                )}
                            </Box>
                        </Stack>
                    </Panel>
                </Box>

                {/* ④ what actually goes out? Spans the row below on narrow screens. */}
                <Box sx={{ minWidth: 0, gridColumn: { xs: 'auto', md: '1 / -1', lg: 'auto' }, height: { lg: '100%' } }}>
                    <Panel scroll title={t('bench.payload', { defaultValue: 'Payload' })} question={t('bench.payloadQ', { defaultValue: 'what actually goes out' })}>
                        <PayloadPanel
                            request={request}
                            curl={curl}
                            loading={curlLoading}
                            direct={direct}
                            rawMode={!!state.raw}
                            buildError={built.error}
                            headers={state.headers}
                            onHeadersChange={(headers) => patch({ headers })}
                            onEditBody={(body) => patch({ raw: { protocol: protocolValue, body } })}
                        />
                    </Panel>
                </Box>
            </Box>
        </Box>
    );
};

export default BenchPage;
