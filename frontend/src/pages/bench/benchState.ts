import { DEFAULT_AXES, type ProbeAxes } from '@/components/probe/probeConfig';
import type { ProbeProtocol, ProbeRequest, ProbeResult, ProbeRouting } from '@/types/probe';
import type { BenchTarget } from './benchLink';

// benchState: the workbench's whole configuration as one plain object —
// what the user composed (target, axes, the request itself), the flag
// overlay and the header edits. It is what gets persisted (the workbench
// resumes where you left it, .design/bench.md §10), what a run history
// chip restores, and what buildProbeRequest turns into the wire request.

// A raw client request: the body text as edited, in `protocol`'s shape.
// The probe fills the model (and Anthropic max_tokens) and sends it on that
// protocol's wire; the fixture knobs (tool / vision / thinking / message)
// do not apply.
export interface RawRequest {
    protocol: ProbeProtocol;
    body: string;
}

export interface BenchState {
    version: 1;
    target: BenchTarget | null;
    axes: ProbeAxes;
    // Rule targets: natural = TB matches the rule from the request model
    // (the production chain, default); pinned = force the chosen rule.
    routing: ProbeRouting;
    // Fixture mode: the single message override ('' = the probe's default).
    message: string;
    // Raw mode: the client request written by hand; null = fixture mode.
    raw: RawRequest | null;
    // Rule-flag overlay: registry (snake_case) key → value. Only keys present
    // are sent; "present" is the override, whatever the value.
    flags: Record<string, unknown>;
    // Header set/override; '' removes the header.
    headers: Record<string, string>;
}

export const STORAGE_KEY = 'tb.bench.state';

export const DEFAULT_STATE: BenchState = {
    version: 1,
    target: null,
    axes: { ...DEFAULT_AXES },
    routing: 'natural',
    message: '',
    raw: null,
    flags: {},
    headers: {},
};

const PROTOCOLS: ProbeProtocol[] = ['openai_chat', 'openai_responses', 'anthropic_v1'];

export function loadState(): BenchState | null {
    try {
        const stored = localStorage.getItem(STORAGE_KEY);
        if (!stored) return null;
        const parsed = JSON.parse(stored) as Partial<BenchState>;
        if (parsed?.version !== 1) return null;
        const raw = parsed.raw && PROTOCOLS.includes(parsed.raw.protocol) && typeof parsed.raw.body === 'string' ? parsed.raw : null;
        return {
            ...DEFAULT_STATE,
            ...parsed,
            axes: { ...DEFAULT_AXES, ...(parsed.axes ?? {}) },
            routing: parsed.routing === 'pinned' ? 'pinned' : 'natural',
            message: typeof parsed.message === 'string' ? parsed.message : '',
            raw,
            flags: parsed.flags ?? {},
            headers: parsed.headers ?? {},
        };
    } catch {
        return null;
    }
}

export function saveState(state: BenchState): void {
    try {
        localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
    } catch {
        // storage unavailable (private mode, quota) — the workbench still works, it just won't resume
    }
}

export function targetKey(t: BenchTarget | null): string {
    if (!t) return '';
    return t.kind === 'rule' ? `rule:${t.ruleUuid}` : `provider:${t.providerUuid}:${t.model}`;
}

export const isDirect = (state: BenchState): boolean =>
    state.target?.kind === 'provider' && state.axes.direct;

export const overlayCount = (state: BenchState): number => Object.keys(state.flags).length;

// parseRawBody turns the edited text into the JSON object the API takes, or
// says why it cannot — the payload panel shows that as build feedback.
export function parseRawBody(raw: RawRequest): { value?: Record<string, unknown>; error?: string } {
    try {
        const value = JSON.parse(raw.body);
        if (!value || typeof value !== 'object' || Array.isArray(value)) return { error: 'the request must be a JSON object' };
        return { value: value as Record<string, unknown> };
    } catch (e: any) {
        return { error: e?.message || 'invalid JSON' };
    }
}

export interface BuiltRequest {
    request: ProbeRequest | null;
    // Why no request could be built (no target, or a raw body that does not parse).
    error?: string;
}

// buildProbeRequest is the single request constructor for Run and the
// payload panel — the two can never disagree about what would be sent.
export function buildProbeRequest(state: BenchState): BuiltRequest {
    const { target, axes } = state;
    if (!target) return { request: null };
    const direct = isDirect(state);
    const req: ProbeRequest =
        target.kind === 'rule'
            ? {
                  target_type: 'rule',
                  scenario: target.scenario || 'openai',
                  rule_uuid: target.ruleUuid,
                  ...(state.routing === 'pinned' ? { routing: 'pinned' as const } : {}),
              }
            : {
                  target_type: 'provider',
                  provider_uuid: target.providerUuid,
                  model: target.model,
                  direct: axes.direct,
              };
    req.stream = axes.stream;
    if (state.raw) {
        const parsed = parseRawBody(state.raw);
        if (parsed.error) return { request: null, error: parsed.error };
        req.request = parsed.value;
        req.request_protocol = state.raw.protocol;
        // A raw request is sent on its own protocol; provider targets say so
        // explicitly, rule targets are bound to the scenario's family.
        if (target.kind === 'provider') req.protocol = state.raw.protocol;
    } else {
        if (target.kind === 'provider' && axes.protocol) req.protocol = axes.protocol;
        req.tool = axes.tool;
        req.thinking = axes.thinking;
        if (axes.vision !== 'none') req.vision = axes.vision;
        if (state.message.trim()) req.message = state.message;
    }
    if (!direct && Object.keys(state.flags).length) req.flags = { ...state.flags };
    if (Object.keys(state.headers).length) req.headers = { ...state.headers };
    return { request: req };
}

export interface RunRecord {
    id: string;
    at: number;
    result: ProbeResult;
    snapshot: BenchState;
    label: string;
}

// runLabel: the chip caption — what made this run different, not everything.
export function runLabel(state: BenchState): string {
    const parts: string[] = [state.axes.stream ? 'stream' : 'one-shot'];
    if (isDirect(state)) parts.push('direct');
    if (state.target?.kind === 'rule' && state.routing === 'pinned') parts.push('pinned');
    if (state.raw) parts.push(`raw ${state.raw.protocol}`);
    else {
        if (state.axes.tool) parts.push('tool');
        if (state.axes.vision !== 'none') parts.push('vision');
        if (state.axes.thinking !== 'none') parts.push(`think=${state.axes.thinking}`);
    }
    const n = isDirect(state) ? 0 : overlayCount(state);
    if (n) parts.push(`${n} flag${n > 1 ? 's' : ''}`);
    const edits = Object.keys(state.headers).length;
    if (edits) parts.push(`${edits} header${edits > 1 ? 's' : ''}`);
    return parts.join(' · ');
}

export const cloneState = (state: BenchState): BenchState => JSON.parse(JSON.stringify(state));
