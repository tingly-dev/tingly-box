import type { ProbeThinking, ProbeProtocol, ProbeResult, ProbeTargetType } from '@/types/probe';
import type { Provider } from '@/types/provider';

// ProbeAxes is the panel's orthogonal control state. Every axis is one knob:
// Shape (stream) × Tool × Thinking × Protocol × Scope (direct) — plus the
// optional message override. Axes are NOT persisted across dialog opens: a
// probe is a diagnostic whose defaults must be predictable, and hidden
// (collapsed-panel) state that silently sticks is worse than re-setting a
// knob. Reopening a stored result restores its axes from the result's
// request-echo fields instead — the visible state always matches the data.
export interface ProbeAxes {
    stream: boolean;
    tool: boolean;
    thinking: ProbeThinking;
    // '' means "no protocol override" — the backend resolves the target's
    // primary protocol (provider APIStyle, Codex OAuth → Responses).
    protocol: ProbeProtocol | '';
    direct: boolean;
}

export const DEFAULT_AXES: ProbeAxes = {
    stream: true, // Stream default — closest to production traffic
    tool: false,
    thinking: 'none',
    protocol: '',
    direct: false,
};

// resolveInitialAxes applies the open-time association priority:
//   1. explicit prop overrides (thinkingLevel prop)
//   2. the pre-computed initialResult — the visible state must match the
//      result the user is looking at; the backend echoes the request axes
//      (stream/tool/direct/protocol/thinking) so every axis is restorable
//   3. defaults (Stream / no tool / no thinking / provider's primary protocol / Through TB)
export function resolveInitialAxes(opts: {
    targetType: ProbeTargetType;
    thinkingLevel?: ProbeThinking;
    initialResult?: ProbeResult;
    provider?: Provider | null;
}): ProbeAxes {
    const axes: ProbeAxes = { ...DEFAULT_AXES };

    // Priority 2: a pre-loaded result wins — the toggles must describe the
    // request that produced it.
    const data = opts.initialResult?.data;
    if (data) {
        if (typeof data.stream === 'boolean') axes.stream = data.stream;
        if (typeof data.tool === 'boolean') axes.tool = data.tool;
        if (typeof data.direct === 'boolean') axes.direct = data.direct;
        if (data.protocol) axes.protocol = data.protocol;
        if (data.thinking) axes.thinking = data.thinking;
    }

    // Priority 1: explicit prop (kept for callers that know better).
    if (opts.thinkingLevel) axes.thinking = opts.thinkingLevel;

    // Protocol/scope availability clamp (e.g. '' protocol for google targets
    // is fine, but a result-echoed anthropic protocol must not stick onto a
    // provider that can't speak it).
    const avail = protocolAvailability(opts.provider ?? null);
    if (avail.locked) {
        axes.protocol = avail.default;
    } else if (axes.protocol && !avail.options.includes(axes.protocol)) {
        axes.protocol = '';
    }
    if (!scopeAvailable(opts.targetType)) {
        axes.direct = false;
    }

    return axes;
}

// scopeAvailable: only provider targets can bypass TB. Rule probes must
// traverse the middleware they exist to test.
export function scopeAvailable(targetType: ProbeTargetType): boolean {
    return targetType === 'provider';
}

export interface ProtocolAvailability {
    // Options offered on the Protocol axis, in display order.
    options: ProbeProtocol[];
    // The target's primary protocol (the smart default).
    default: ProbeProtocol | '';
    // Locked means the target speaks exactly one protocol — render the axis
    // disabled with that value. Disabled (options empty + locked with '')
    // means no protocol axis at all (Google's own SDK).
    locked: boolean;
}

// protocolAvailability reduces the Protocol axis per target. Brand-first
// labels (OpenAI Chat / OpenAI Responses / Anthropic) live in i18n; bare
// "Responses"/"Messages" assume SDK knowledge users don't have.
export function protocolAvailability(provider: Provider | null): ProtocolAvailability {
    if (!provider) {
        // Unknown target (still loading) — offer nothing, don't lock.
        return { options: [], default: '', locked: false };
    }
    if (provider.api_style === 'google') {
        return { options: [], default: '', locked: true };
    }
    const isCodexOAuth = provider.oauth_detail?.issuer === 'codex';
    if (provider.api_style === 'anthropic') {
        return { options: ['anthropic_v1'], default: 'anthropic_v1', locked: true };
    }
    const hasDualAnthropic = !!provider.api_base_anthropic;
    const options: ProbeProtocol[] = ['openai_chat', 'openai_responses'];
    if (hasDualAnthropic) options.push('anthropic_v1');
    const primary: ProbeProtocol = isCodexOAuth ? 'openai_responses' : 'openai_chat';
    return { options, default: primary, locked: false };
}
