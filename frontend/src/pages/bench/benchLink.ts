import type { ProbeTargetType, ProbeThinking, ProbeVision, ProbeProtocol } from '@/types/probe';
import type { ProbeAxes } from '@/components/probe/probeConfig';

// benchLink: the URL contract of the Bench page. Lives in its own
// page-free module so the probe dialog (and any card menu) can build a deep
// link without pulling the lazy page chunk into the eager bundle
// (frontend/CLAUDE.md, code-splitting).
//
//   /bench?target=rule:{uuid}&scenario={scenario}
//   /bench?target=provider:{uuid}&model={model}
//   + optional knobs: stream=0|1 tool=0|1 direct=0|1 thinking= vision= protocol= message=
//
// Explicit URL intent beats the persisted workbench state (.design/bench.md §10).

export const BENCH_PATH = '/bench';

export type BenchTarget =
    | { kind: 'rule'; ruleUuid: string; scenario: string }
    | { kind: 'provider'; providerUuid: string; model: string };

export interface BenchLinkInput {
    targetType: ProbeTargetType;
    targetId: string;
    scenario?: string;
    model?: string;
    axes?: Partial<ProbeAxes>;
    message?: string;
}

export interface BenchLinkParams {
    target: BenchTarget | null;
    axes: Partial<ProbeAxes>;
    message?: string;
}

const THINKING_VALUES: ProbeThinking[] = ['none', 'low', 'medium', 'high', 'max'];
const VISION_VALUES: ProbeVision[] = ['none', 'user', 'tool'];
const PROTOCOL_VALUES: ProbeProtocol[] = ['openai_chat', 'openai_responses', 'anthropic_v1'];

export function benchDeepLink(input: BenchLinkInput): string {
    const q = new URLSearchParams();
    if (input.targetType === 'rule') {
        q.set('target', `rule:${input.targetId}`);
        if (input.scenario) q.set('scenario', input.scenario);
    } else if (input.targetType === 'provider') {
        q.set('target', `provider:${input.targetId}`);
        if (input.model) q.set('model', input.model);
    }
    const a = input.axes;
    if (a) {
        if (a.stream !== undefined) q.set('stream', a.stream ? '1' : '0');
        if (a.tool !== undefined) q.set('tool', a.tool ? '1' : '0');
        if (a.direct !== undefined) q.set('direct', a.direct ? '1' : '0');
        if (a.thinking) q.set('thinking', a.thinking);
        if (a.vision) q.set('vision', a.vision);
        if (a.protocol) q.set('protocol', a.protocol);
    }
    if (input.message) q.set('message', input.message);
    const qs = q.toString();
    return qs ? `${BENCH_PATH}?${qs}` : BENCH_PATH;
}

const flag = (v: string | null): boolean | undefined => (v === null ? undefined : v === '1' || v === 'true');

export function parseBenchLink(search: string): BenchLinkParams {
    const q = new URLSearchParams(search);
    let target: BenchTarget | null = null;
    const raw = q.get('target') || '';
    const sep = raw.indexOf(':');
    if (sep > 0) {
        const kind = raw.slice(0, sep);
        const id = raw.slice(sep + 1);
        if (kind === 'rule' && id) {
            target = { kind: 'rule', ruleUuid: id, scenario: q.get('scenario') || '' };
        } else if (kind === 'provider' && id) {
            target = { kind: 'provider', providerUuid: id, model: q.get('model') || '' };
        }
    }
    const axes: Partial<ProbeAxes> = {};
    const stream = flag(q.get('stream'));
    const tool = flag(q.get('tool'));
    const direct = flag(q.get('direct'));
    if (stream !== undefined) axes.stream = stream;
    if (tool !== undefined) axes.tool = tool;
    if (direct !== undefined) axes.direct = direct;
    const thinking = q.get('thinking') as ProbeThinking | null;
    if (thinking && THINKING_VALUES.includes(thinking)) axes.thinking = thinking;
    const vision = q.get('vision') as ProbeVision | null;
    if (vision && VISION_VALUES.includes(vision)) axes.vision = vision;
    const protocol = q.get('protocol') as ProbeProtocol | null;
    if (protocol && PROTOCOL_VALUES.includes(protocol)) axes.protocol = protocol;
    const message = q.get('message') || undefined;
    return { target, axes, message };
}
