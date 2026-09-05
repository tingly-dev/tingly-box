import { DEFAULT_AXES, type ProbeAxes } from '@/components/probe/probeConfig';
import type { ProbeClient, ProbeMessageRole, ProbeRequest, ProbeResult, ProbeRouting } from '@/types/probe';
import type { PlaygroundTarget } from './playgroundLink';

// playgroundState: the workbench's whole configuration as one plain object —
// what the user composed (target, axes, conversation), the flag overlay and
// the hand edits to the payload. It is what gets persisted (the workbench
// resumes where you left it, .design/playground.md §10), what a run history
// chip restores, and what buildProbeRequest turns into the wire request.

export interface PlaygroundTurn {
    role: ProbeMessageRole;
    text: string;
}

export interface PlaygroundState {
    version: 1;
    target: PlaygroundTarget | null;
    axes: ProbeAxes;
    // Rule targets: natural = TB matches the rule from the request model
    // (the production chain, default); pinned = force the chosen rule.
    routing: ProbeRouting;
    // Send the loopback request as a real client (TB's own client
    // implementation emits it); null = the probe itself.
    client: ProbeClient | null;
    system: string;
    turns: PlaygroundTurn[];
    // Rule-flag overlay: registry (snake_case) key → value. Only keys present
    // are sent; "present" is the override, whatever the value.
    flags: Record<string, unknown>;
    // Hand edits to the payload: top-level body key → value, or null to
    // delete the key. Applied by the backend after every builder and flag.
    bodyOverrides: Record<string, unknown>;
    // Header set/override; '' removes the header.
    headers: Record<string, string>;
}

export const STORAGE_KEY = 'tb.playground.state';

export const DEFAULT_STATE: PlaygroundState = {
    version: 1,
    target: null,
    axes: { ...DEFAULT_AXES },
    routing: 'natural',
    client: null,
    system: '',
    turns: [],
    flags: {},
    bodyOverrides: {},
    headers: {},
};

export function loadState(): PlaygroundState | null {
    try {
        const raw = localStorage.getItem(STORAGE_KEY);
        if (!raw) return null;
        const parsed = JSON.parse(raw) as Partial<PlaygroundState>;
        if (parsed?.version !== 1) return null;
        return {
            ...DEFAULT_STATE,
            ...parsed,
            axes: { ...DEFAULT_AXES, ...(parsed.axes ?? {}) },
            routing: parsed.routing === 'pinned' ? 'pinned' : 'natural',
            client: parsed.client === 'claude_code' ? 'claude_code' : null,
            turns: Array.isArray(parsed.turns) ? parsed.turns : [],
            flags: parsed.flags ?? {},
            bodyOverrides: parsed.bodyOverrides ?? {},
            headers: parsed.headers ?? {},
        };
    } catch {
        return null;
    }
}

export function saveState(state: PlaygroundState): void {
    try {
        localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
    } catch {
        // storage unavailable (private mode, quota) — the workbench still works, it just won't resume
    }
}

export function targetKey(t: PlaygroundTarget | null): string {
    if (!t) return '';
    return t.kind === 'rule' ? `rule:${t.ruleUuid}` : `provider:${t.providerUuid}:${t.model}`;
}

export const isDirect = (state: PlaygroundState): boolean =>
    state.target?.kind === 'provider' && state.axes.direct;

export const overlayCount = (state: PlaygroundState): number => Object.keys(state.flags).length;

// buildProbeRequest is the single request constructor for Run and the
// payload panel — the two can never disagree about what would be sent.
// Returns null until a target is picked.
export function buildProbeRequest(state: PlaygroundState): ProbeRequest | null {
    const { target, axes } = state;
    if (!target) return null;
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
                  ...(axes.protocol ? { protocol: axes.protocol } : {}),
              };
    req.stream = axes.stream;
    req.tool = axes.tool;
    req.thinking = axes.thinking;
    if (axes.vision !== 'none') req.vision = axes.vision;
    if (state.system.trim()) req.system = state.system;
    if (state.client && !isDirect(state)) req.client = state.client;
    if (state.turns.length) req.messages = state.turns.map((t) => ({ role: t.role, text: t.text }));
    if (!isDirect(state) && Object.keys(state.flags).length) req.flags = { ...state.flags };
    if (Object.keys(state.bodyOverrides).length) req.body_overrides = { ...state.bodyOverrides };
    if (Object.keys(state.headers).length) req.headers = { ...state.headers };
    return req;
}

export interface RunRecord {
    id: string;
    at: number;
    result: ProbeResult;
    snapshot: PlaygroundState;
    label: string;
}

// runLabel: the chip caption — what made this run different, not everything.
export function runLabel(state: PlaygroundState): string {
    const parts: string[] = [state.axes.stream ? 'stream' : 'one-shot'];
    if (isDirect(state)) parts.push('direct');
    if (state.target?.kind === 'rule' && state.routing === 'pinned') parts.push('pinned');
    if (state.client && !isDirect(state)) parts.push(`as ${state.client}`);
    if (state.axes.tool) parts.push('tool');
    if (state.axes.vision !== 'none') parts.push('vision');
    if (state.axes.thinking !== 'none') parts.push(`think=${state.axes.thinking}`);
    const n = isDirect(state) ? 0 : overlayCount(state);
    if (n) parts.push(`${n} flag${n > 1 ? 's' : ''}`);
    const edits = Object.keys(state.bodyOverrides).length + Object.keys(state.headers).length;
    if (edits) parts.push(`${edits} edit${edits > 1 ? 's' : ''}`);
    if (state.turns.length) parts.push(`${state.turns.length} turn${state.turns.length > 1 ? 's' : ''}`);
    return parts.join(' · ');
}

export const cloneState = (state: PlaygroundState): PlaygroundState => JSON.parse(JSON.stringify(state));
