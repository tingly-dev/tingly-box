import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Box, Button, LinearProgress, Paper, Stack, Typography } from '@mui/material';
import { PlayArrow as RunIcon, TestPipe as BenchIcon } from '@/components/icons';
import { useTranslation } from 'react-i18next';
import { useLocation, useNavigate } from 'react-router-dom';
import PageHeader from '@/components/PageHeader';
import api from '@/services/api';
import type { FlagSpec } from '@/components/RoutingGraphTypes';
import type { Provider } from '@/types/provider';
import type { ProbeResult } from '@/types/probe';
import { runProbe, buildProbeCurl, type ProbeCurlResult } from '@/components/probe/runProbe';
import { protocolAvailability, visionAvailable } from '@/components/probe/probeConfig';
import type { ProbeProtocol } from '@/types/probe';
import { StatusBar, Journey, CollapsibleSection, CopyBlock, extractText, defaultMessage, ruleProtocolForScenario } from '@/components/probe/ResultSections';
import { BENCH_PATH, parseBenchLink } from './benchLink';
import {
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
import { TargetPicker, ruleLabel } from './TargetPicker';
import { BenchAxes, useAxisAvailability } from './BenchAxes';
import { PluginsPanel, isFlagSet, type FlagBaseline } from './PluginsPanel';
import { RequestEditor } from './RequestEditor';
import { PayloadPanel } from './PayloadPanel';
import { RunHistory } from './RunHistory';

// BenchPage — the customizable end-to-end test workbench
// (.design/bench.md). Three columns answer the user's three questions:
// Compose (what do I send?) · Conversation + Result (what happened?) ·
// Payload (what actually goes out?). Every knob is resident; nothing here is
// written to any rule or scenario.

const MAX_RUNS = 10;

const prettyBody = (raw: string): string => {
    try {
        return JSON.stringify(JSON.parse(raw), null, 2);
    } catch {
        return raw;
    }
};

// Initial state: an explicit URL intent (deep link) beats the persisted
// workbench state, which beats the defaults (§10).
function initialState(search: string): BenchState {
    const base = loadState() ?? cloneState(DEFAULT_STATE);
    const link = parseBenchLink(search);
    if (!link.target && Object.keys(link.axes).length === 0 && !link.message) return base;
    const next: BenchState = { ...base, axes: { ...base.axes, ...link.axes } };
    if (link.target) {
        next.target = link.target;
        // A new target means a new baseline; overlays composed against the
        // old one would silently mean something else.
        next.flags = {};
        next.headers = {};
        next.raw = null;
    }
    if (link.message) {
        next.message = link.message;
        next.raw = null;
    }
    return next;
}

const Panel: React.FC<{ title: string; question: string; action?: React.ReactNode; children: React.ReactNode }> = ({ title, question, action, children }) => (
    <Paper variant="outlined" sx={{ overflow: 'hidden' }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, px: 1.75, py: 1, borderBottom: '1px solid', borderColor: 'divider' }}>
            <Typography variant="overline" sx={{ fontSize: '0.62rem', color: 'text.secondary', lineHeight: 1 }}>{title}</Typography>
            <Typography variant="caption" sx={{ color: 'text.disabled' }}>{question}</Typography>
            <Box sx={{ flex: 1 }} />
            {action}
        </Box>
        <Box sx={{ p: 1.75 }}>{children}</Box>
    </Paper>
);

function computeBaseline(registry: FlagSpec[], ruleFlags: Record<string, unknown> | undefined, scenarioFlags: Record<string, unknown> | undefined): FlagBaseline {
    const values: FlagBaseline['values'] = {};
    const sources: FlagBaseline['sources'] = {};
    registry.forEach((spec) => {
        const rv = ruleFlags?.[spec.key];
        const sv = spec.shared ? scenarioFlags?.[spec.key] : undefined;
        const ruleSet = isFlagSet(spec, rv);
        const scenSet = isFlagSet(spec, sv);
        if (spec.type === 'bool' && spec.inheritanceMode === 'or') {
            if (ruleSet || scenSet) {
                values[spec.key] = true;
                sources[spec.key] = ruleSet ? 'rule' : 'scenario';
            }
        } else if (ruleSet) {
            values[spec.key] = rv;
            sources[spec.key] = 'rule';
        } else if (scenSet) {
            values[spec.key] = sv;
            sources[spec.key] = 'scenario';
        }
    });
    return { values, sources };
}

const BenchPage: React.FC = () => {
    const { t } = useTranslation();
    const location = useLocation();
    const navigate = useNavigate();
    const catalog = useTargetCatalog();

    const [state, setState] = useState<BenchState>(() => initialState(location.search));
    const [registry, setRegistry] = useState<FlagSpec[]>([]);
    const [registryLoading, setRegistryLoading] = useState(true);
    const [scenarioFlags, setScenarioFlags] = useState<Record<string, unknown> | undefined>();
    const [curl, setCurl] = useState<ProbeCurlResult | null>(null);
    const [curlLoading, setCurlLoading] = useState(false);
    const [running, setRunning] = useState(false);
    const [shown, setShown] = useState<{ result: ProbeResult; snapshot: BenchState } | null>(null);
    const [runs, setRuns] = useState<RunRecord[]>([]);
    const [activeRun, setActiveRun] = useState<string | null>(null);

    const patch = useCallback((p: Partial<BenchState>) => setState((s) => ({ ...s, ...p })), []);

    // Persist every change; the deep link has been consumed, so drop it from
    // the URL — a reload must resume the workbench, not replay the link.
    useEffect(() => saveState(state), [state]);
    useEffect(() => {
        if (location.search) navigate(BENCH_PATH, { replace: true });
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

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
    const rule = useMemo(() => (target?.kind === 'rule' ? catalog.rules.find((r) => r.uuid === target.ruleUuid) ?? null : null), [catalog.rules, target]);
    const provider: Provider | null = useMemo(
        () => (target?.kind === 'provider' ? catalog.providers.find((p) => p.uuid === target.providerUuid) ?? null : null),
        [catalog.providers, target],
    );

    // Scenario-level flags feed the inherited baseline of a rule target.
    useEffect(() => {
        const scenario = rule?.scenario;
        if (!scenario) { setScenarioFlags(undefined); return; }
        let cancelled = false;
        api.getScenarioConfig(scenario)
            .then((r: any) => { if (!cancelled) setScenarioFlags(r?.success ? r.data?.flags ?? undefined : undefined); })
            .catch(() => { if (!cancelled) setScenarioFlags(undefined); });
        return () => { cancelled = true; };
    }, [rule?.scenario]);

    const baseline = useMemo(
        () => computeBaseline(registry, (rule?.flags as Record<string, unknown> | undefined) ?? undefined, scenarioFlags),
        [registry, rule?.flags, scenarioFlags],
    );

    // Axis availability per target, and the clamp that keeps the axes legal
    // when the target (or its provider record) changes — same rules as the
    // probe dialog.
    const availability = useAxisAvailability(target, provider, state.axes);
    useEffect(() => {
        setState((s) => {
            const a = { ...s.axes };
            let changed = false;
            const avail = protocolAvailability(provider);
            if (target?.kind === 'rule') {
                if (a.direct) { a.direct = false; changed = true; }
                if (a.protocol !== '') { a.protocol = ''; changed = true; }
            } else if (target?.kind === 'provider' && provider) {
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
    const requestKey = useMemo(() => JSON.stringify(request), [request]);
    const direct = isDirect(state);

    // Protocols a hand-written request may be in for this target: a rule's
    // scenario family, or whatever the provider speaks.
    const rawProtocolOptions = useMemo<ProbeProtocol[]>(() => {
        if (target?.kind === 'rule') {
            return ruleProtocolForScenario(target.scenario) === 'anthropic_v1' ? ['anthropic_v1'] : ['openai_chat', 'openai_responses'];
        }
        const avail = protocolAvailability(provider);
        return avail.options.length ? avail.options : ['openai_chat', 'openai_responses', 'anthropic_v1'];
    }, [target, provider]);
    const wireProtocol: ProbeProtocol = state.raw?.protocol
        ?? (target?.kind === 'rule' ? ruleProtocolForScenario(target.scenario) : (state.axes.protocol || protocolAvailability(provider).default || 'openai_chat'));

    // Live payload: rebuild 500 ms after the last change. Pure construction.
    useEffect(() => {
        if (!request) { setCurl(null); setCurlLoading(false); return; }
        let cancelled = false;
        setCurlLoading(true);
        const timer = setTimeout(async () => {
            const res = await buildProbeCurl(request);
            if (cancelled) return;
            setCurl(res);
            setCurlLoading(false);
        }, 500);
        return () => { cancelled = true; clearTimeout(timer); };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [requestKey]);

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

    // Natural routing can land on a rule other than the one picked — that is
    // a finding, not an error, and the Journey says so.
    const shownRuleMismatch = (() => {
        const snap = shown?.snapshot;
        const matched = shown?.result.data?.matched_rule;
        if (!snap || snap.target?.kind !== 'rule' || snap.routing === 'pinned' || !matched) return false;
        return matched !== snap.target.ruleUuid;
    })();

    const targetName = rule ? ruleLabel(rule) : provider?.name ?? '';
    const shownOverlay = shown ? Object.entries(shown.snapshot.flags) : [];
    const extracted = useMemo(() => extractText(shown?.result.data?.content), [shown?.result.data?.content]);

    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            <PageHeader
                title={t('bench.title', { defaultValue: 'Bench' })}
                subtitle={t('bench.subtitle', { defaultValue: 'One probe request with every knob resident: pick a target, shape the request, overlay flags, edit the payload. Nothing here is saved to a rule.' })}
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
                    alignItems: 'start',
                    gridTemplateColumns: { xs: '1fr', md: 'minmax(0, 1fr) minmax(0, 1fr)', xl: '320px minmax(0, 1fr) 440px' },
                }}
            >
                {/* ① what do I send? */}
                <Stack spacing={2}>
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
                                targetKind={target?.kind ?? null}
                                routing={state.routing}
                                onRoutingChange={(routing) => patch({ routing })}
                                rawProtocol={state.raw?.protocol ?? null}
                            />
                        </Stack>
                    </Panel>
                    <Panel title={t('bench.plugins', { defaultValue: 'Plugins' })} question={t('bench.pluginsQ', { defaultValue: 'flag overlay · this request only' })}>
                        <PluginsPanel
                            registry={registry}
                            loading={registryLoading}
                            baseline={baseline}
                            overlay={state.flags}
                            onChange={(flags) => patch({ flags })}
                            disabled={direct}
                        />
                    </Panel>
                </Stack>

                {/* ② the request itself · ③ what happened? */}
                <Stack spacing={2}>
                    <Panel title={t('bench.requestPanel', { defaultValue: 'Request' })} question={t('bench.requestQ', { defaultValue: 'what the client sends' })}>
                        <RequestEditor
                            message={state.message}
                            onMessageChange={(message) => patch({ message })}
                            raw={state.raw}
                            onRawChange={(raw) => patch({ raw })}
                            protocolOptions={rawProtocolOptions}
                            seedBody={curl?.success && curl.data?.body && !state.raw ? prettyBody(curl.data.body) : undefined}
                            error={state.raw ? built.error : undefined}
                            messagePlaceholder={defaultMessage(state.axes.tool)}
                        />
                    </Panel>
                    <Panel title={t('bench.result', { defaultValue: 'Result' })} question={t('bench.resultQ', { defaultValue: 'what happened?' })}>
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
                                        targetType={shown.snapshot.target?.kind ?? 'provider'}
                                        targetName={targetName}
                                        scenario={shown.snapshot.target?.kind === 'rule' ? shown.snapshot.target.scenario : undefined}
                                        model={shown.snapshot.target?.kind === 'provider' ? shown.snapshot.target.model : undefined}
                                        bypassed={isDirect(shown.snapshot)}
                                        showFlags
                                        ruleExtra={
                                            shownRuleMismatch ? (
                                                <Typography variant="caption" sx={{ display: 'block', color: 'warning.main', fontFamily: 'inherit', mt: 0.25 }}>
                                                    {t('bench.ruleMismatch', { defaultValue: 'TB matched a different rule than the one you picked — this is what a real client would hit. Switch Scope to “Pinned rule” to force yours.' })}
                                                </Typography>
                                            ) : undefined
                                        }
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
                    </Panel>
                </Stack>

                {/* ④ what actually goes out? Spans the row below on narrow screens. */}
                <Box sx={{ gridColumn: { xs: 'auto', md: '1 / -1', xl: 'auto' } }}>
                    <Panel title={t('bench.payload', { defaultValue: 'Payload' })} question={t('bench.payloadQ', { defaultValue: 'what actually goes out' })}>
                        <PayloadPanel
                            request={request}
                            curl={curl}
                            loading={curlLoading}
                            direct={direct}
                            rawMode={!!state.raw}
                            buildError={built.error}
                            headers={state.headers}
                            onHeadersChange={(headers) => patch({ headers })}
                            onEditBody={(body) => patch({ raw: { protocol: wireProtocol, body } })}
                        />
                    </Panel>
                </Box>
            </Box>
        </Box>
    );
};

export default BenchPage;
