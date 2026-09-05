import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Box, Button, LinearProgress, MenuItem, Paper, Select, Stack, Tooltip, Typography } from '@mui/material';
import { PlayArrow as RunIcon, TestPipe as PlaygroundIcon } from '@/components/icons';
import { useTranslation } from 'react-i18next';
import { useLocation, useNavigate } from 'react-router-dom';
import PageHeader from '@/components/PageHeader';
import api from '@/services/api';
import type { FlagSpec } from '@/components/RoutingGraphTypes';
import type { Provider } from '@/types/provider';
import { PROBE_CLIENTS, type ProbeClient, type ProbeProtocol, type ProbeResult } from '@/types/probe';
import { runProbe, buildProbeCurl, type ProbeCurlResult } from '@/components/probe/runProbe';
import { protocolAvailability, visionAvailable } from '@/components/probe/probeConfig';
import { StatusBar, Journey, CollapsibleSection, CopyBlock, extractText, ruleProtocolForScenario } from '@/components/probe/ResultSections';
import { PLAYGROUND_PATH, parsePlaygroundLink } from './playgroundLink';
import {
    DEFAULT_STATE,
    buildProbeRequest,
    cloneState,
    isDirect,
    loadState,
    runLabel,
    saveState,
    targetKey,
    type PlaygroundState,
    type RunRecord,
} from './playgroundState';
import { useTargetCatalog } from './useTargetCatalog';
import { TargetPicker, ruleLabel } from './TargetPicker';
import { PlaygroundAxes, useAxisAvailability } from './PlaygroundAxes';
import { PluginsPanel, isFlagSet, type FlagBaseline } from './PluginsPanel';
import { ConversationEditor } from './ConversationEditor';
import { PayloadPanel } from './PayloadPanel';
import { RunHistory } from './RunHistory';

// PlaygroundPage — the customizable end-to-end test workbench
// (.design/playground.md). Three columns answer the user's three questions:
// Compose (what do I send?) · Conversation + Result (what happened?) ·
// Payload (what actually goes out?). Every knob is resident; nothing here is
// written to any rule or scenario.

const MAX_RUNS = 10;

// Initial state: an explicit URL intent (deep link) beats the persisted
// workbench state, which beats the defaults (§10).
function initialState(search: string): PlaygroundState {
    const base = loadState() ?? cloneState(DEFAULT_STATE);
    const link = parsePlaygroundLink(search);
    if (!link.target && Object.keys(link.axes).length === 0 && !link.message) return base;
    const next: PlaygroundState = { ...base, axes: { ...base.axes, ...link.axes } };
    if (link.target) {
        next.target = link.target;
        // A new target means a new baseline; overlays composed against the
        // old one would silently mean something else.
        next.flags = {};
        next.bodyOverrides = {};
        next.headers = {};
    }
    if (link.message) next.turns = [{ role: 'user', text: link.message }];
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

const PlaygroundPage: React.FC = () => {
    const { t } = useTranslation();
    const location = useLocation();
    const navigate = useNavigate();
    const catalog = useTargetCatalog();

    const [state, setState] = useState<PlaygroundState>(() => initialState(location.search));
    const [registry, setRegistry] = useState<FlagSpec[]>([]);
    const [registryLoading, setRegistryLoading] = useState(true);
    const [scenarioFlags, setScenarioFlags] = useState<Record<string, unknown> | undefined>();
    const [curl, setCurl] = useState<ProbeCurlResult | null>(null);
    const [curlLoading, setCurlLoading] = useState(false);
    const [running, setRunning] = useState(false);
    const [shown, setShown] = useState<{ result: ProbeResult; snapshot: PlaygroundState } | null>(null);
    const [runs, setRuns] = useState<RunRecord[]>([]);
    const [activeRun, setActiveRun] = useState<string | null>(null);

    const patch = useCallback((p: Partial<PlaygroundState>) => setState((s) => ({ ...s, ...p })), []);

    // Persist every change; the deep link has been consumed, so drop it from
    // the URL — a reload must resume the workbench, not replay the link.
    useEffect(() => saveState(state), [state]);
    useEffect(() => {
        if (location.search) navigate(PLAYGROUND_PATH, { replace: true });
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

    // The protocol the probe speaks right now — a client identity is bound to one.
    const effectiveProtocol: ProbeProtocol | '' = target?.kind === 'rule' ? ruleProtocolForScenario(target.scenario) : state.axes.protocol;

    const request = useMemo(() => buildProbeRequest(state), [state]);
    const requestKey = useMemo(() => JSON.stringify(request), [request]);
    const direct = isDirect(state);

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

    const setTarget = (next: PlaygroundState['target']) => {
        if (targetKey(next) === targetKey(state.target)) return;
        // New target, new baseline: overlays and hand edits were composed
        // against the old one.
        patch({ target: next, flags: {}, bodyOverrides: {}, headers: {}, client: null });
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
                title={t('playground.title', { defaultValue: 'Playground' })}
                subtitle={t('playground.subtitle', { defaultValue: 'One probe request with every knob resident: pick a target, shape the request, overlay flags, edit the payload. Nothing here is saved to a rule.' })}
                icon={<PlaygroundIcon sx={{ fontSize: 26 }} />}
                actions={
                    <Button variant="contained" startIcon={<RunIcon />} onClick={run} disabled={!request || running} sx={{ minWidth: 120 }} title={t('playground.runHint', { defaultValue: '⌘ / Ctrl + Enter' })}>
                        {running ? t('playground.running', { defaultValue: 'Running…' }) : t('playground.run', { defaultValue: 'Run' })}
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
                    <Panel title={t('playground.compose', { defaultValue: 'Compose' })} question={t('playground.composeQ', { defaultValue: 'what do I send?' })}>
                        <Stack spacing={1.5}>
                            <Box>
                                <Typography variant="caption" sx={{ color: 'text.secondary', fontWeight: 500, display: 'block', mb: 0.5 }}>
                                    {t('playground.target', { defaultValue: 'Target' })}
                                </Typography>
                                <TargetPicker catalog={catalog} value={state.target} onChange={setTarget} />
                            </Box>
                            <PlaygroundAxes
                                axes={state.axes}
                                onChange={(axes) => patch({ axes })}
                                availability={availability}
                                targetKind={target?.kind ?? null}
                                routing={state.routing}
                                onRoutingChange={(routing) => patch({ routing })}
                            />
                            <Box>
                                <Tooltip title={t('playground.clientHint', { defaultValue: "Send the request as a real client: TB's own client implementation for that app emits it, so the headers, session key and body conventions are whatever it sends — and TB's inbound-keyed middleware (user-agent precedence, clean_header, session affinity) fires as in production. Through TB and Anthropic protocol only." })}>
                                    <Typography variant="caption" sx={{ color: 'text.secondary', fontWeight: 500, display: 'block', mb: 0.5, cursor: 'help' }}>
                                        {t('playground.client', { defaultValue: 'Send as' })}
                                    </Typography>
                                </Tooltip>
                                <Select
                                    size="small"
                                    fullWidth
                                    value={state.client ?? ''}
                                    displayEmpty
                                    onChange={(e) => patch({ client: (e.target.value as ProbeClient) || null })}
                                    disabled={!target || direct}
                                    sx={{ fontSize: '0.8rem' }}
                                >
                                    <MenuItem value="" sx={{ fontSize: '0.8rem' }}>{t('playground.clientNone', { defaultValue: 'Probe itself (plain SDK request)' })}</MenuItem>
                                    {PROBE_CLIENTS.map((c) => {
                                        const mismatch = !!effectiveProtocol && c.protocol !== effectiveProtocol;
                                        return (
                                            <MenuItem key={c.id} value={c.id} disabled={mismatch} sx={{ fontSize: '0.8rem', display: 'block' }}>
                                                {t(`playground.clientLabel.${c.id}`, { defaultValue: c.id })}
                                                <Typography variant="caption" sx={{ display: 'block', color: 'text.disabled', whiteSpace: 'normal', maxWidth: 320 }}>
                                                    {mismatch
                                                        ? t('playground.clientProtocolMismatch', { protocol: c.protocol, defaultValue: 'needs the {{protocol}} protocol' })
                                                        : t(`playground.clientDesc.${c.id}`, { defaultValue: '' })}
                                                </Typography>
                                            </MenuItem>
                                        );
                                    })}
                                </Select>
                            </Box>
                        </Stack>
                    </Panel>
                    <Panel title={t('playground.plugins', { defaultValue: 'Plugins' })} question={t('playground.pluginsQ', { defaultValue: 'flag overlay · this request only' })}>
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

                {/* ② the turns · ③ what happened? */}
                <Stack spacing={2}>
                    <Panel title={t('playground.conversation', { defaultValue: 'Conversation' })} question={t('playground.conversationQ', { defaultValue: 'the turns this probe sends' })}>
                        <ConversationEditor
                            system={state.system}
                            onSystemChange={(system) => patch({ system })}
                            turns={state.turns}
                            onTurnsChange={(turns) => patch({ turns })}
                            onTemplate={(tpl) => { if (tpl.tool && !state.axes.tool) patch({ axes: { ...state.axes, tool: true } }); }}
                            visionOn={state.axes.vision !== 'none'}
                        />
                    </Panel>
                    <Panel title={t('playground.result', { defaultValue: 'Result' })} question={t('playground.resultQ', { defaultValue: 'what happened?' })}>
                        {running && <LinearProgress sx={{ height: 6, borderRadius: 3 }} />}
                        {!running && !shown && (
                            <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                                {t('playground.resultEmpty', { defaultValue: 'Not run yet — press Run to send exactly this request.' })}
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
                                                    {t('playground.ruleMismatch', { defaultValue: 'TB matched a different rule than the one you picked — this is what a real client would hit. Switch Scope to “Pinned rule” to force yours.' })}
                                                </Typography>
                                            ) : undefined
                                        }
                                        flagsExtra={
                                            shownOverlay.length > 0 ? (
                                                <Typography variant="caption" sx={{ display: 'block', color: 'primary.main', fontFamily: 'inherit', mt: 0.25 }}>
                                                    {t('playground.overlaySent', { flags: shownOverlay.map(([k, v]) => `${k}=${JSON.stringify(v)}`).join(', '), defaultValue: 'overlay sent: {{flags}}' })}
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
                    <Panel title={t('playground.payload', { defaultValue: 'Payload' })} question={t('playground.payloadQ', { defaultValue: 'what actually goes out' })}>
                        <PayloadPanel
                            request={request}
                            curl={curl}
                            loading={curlLoading}
                            direct={direct}
                            bodyOverrides={state.bodyOverrides}
                            headers={state.headers}
                            onBodyOverridesChange={(bodyOverrides) => patch({ bodyOverrides })}
                            onHeadersChange={(headers) => patch({ headers })}
                        />
                    </Panel>
                </Box>
            </Box>
        </Box>
    );
};

export default PlaygroundPage;
