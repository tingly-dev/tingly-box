import type { ProbeProtocol } from '@/types/probe';
import { BLANK_REQUEST } from './benchState';

// contentOptions: the protocol-scoped catalog for Compose's Content control.
// Content unifies what used to be two separate things — the preset/custom
// Request-mode gate and RequestEditor's "Change starting point" menu — into
// one list, because they were always the same kind of object at different
// granularity (.design/bench.md §1 "内容预设的两个粒度是同一种东西"): 'message'
// is the fragment-level composer (Tool/Vision/Thinking axes + Message field),
// everything else is a whole-body content preset. Picking 'message' clears
// `raw`; picking anything else replaces the whole raw body — a single body
// still has exactly one author (.design/bench.md §6), Content just chooses
// who that author is without a separate mode toggle to discover first.
//
// The catalog is keyed by protocol, not flat: a request's shape is the
// protocol's shape (Anthropic Messages / OpenAI Chat / OpenAI Responses
// disagree on almost every field), so "Mid-conversation system" only exists
// for anthropic_v1 and a Tool round-trip body is not interchangeable across
// protocols. Switching Protocol re-scopes which Content options exist.
export interface ContentTemplate {
    id: string;
    body: object;
}

// One id per template, shared by every protocol that has it — lets a
// protocol switch carry the same *kind* of content forward (see
// carryContentAcrossProtocol in BenchPage.tsx) instead of always collapsing
// back to Blank.
const TEMPLATES: Record<ProbeProtocol, ContentTemplate[]> = {
    anthropic_v1: [
        { id: 'multi', body: { messages: [
            { role: 'user', content: "What's in ./src?" },
            { role: 'assistant', content: 'Three files: main.go, helper.go, types.go.' },
            { role: 'user', content: 'Summarize helper.go in one line.' },
        ] } },
        { id: 'tool', body: { tools: [{ name: 'get_weather', description: 'Current weather for a city', input_schema: { type: 'object', properties: { city: { type: 'string' } }, required: ['city'] } }], messages: [
            { role: 'user', content: "What's the weather in Tokyo?" },
            { role: 'assistant', content: [{ type: 'tool_use', id: 'toolu_01', name: 'get_weather', input: { city: 'Tokyo' } }] },
            { role: 'user', content: [{ type: 'tool_result', tool_use_id: 'toolu_01', content: '24°C, clear' }] },
        ] } },
        { id: 'image', body: { messages: [
            { role: 'user', content: [{ type: 'text', text: 'What colour is this image?' }, { type: 'image', source: { type: 'url', url: 'https://upload.wikimedia.org/wikipedia/commons/thumb/4/47/PNG_transparency_demonstration_1.png/280px-PNG_transparency_demonstration_1.png' } }] },
        ] } },
        { id: 'midsys', body: { system: 'You are a test agent.', messages: [
            { role: 'user', content: "What's in ./src?" },
            { role: 'assistant', content: 'Three files: main.go, helper.go, types.go.' },
            { role: 'system', content: 'From now on answer only in JSON.' },
            { role: 'user', content: 'List them again.' },
        ] } },
    ],
    openai_chat: [
        { id: 'multi', body: { messages: [
            { role: 'system', content: 'You are a test agent.' },
            { role: 'user', content: "What's in ./src?" },
            { role: 'assistant', content: 'Three files: main.go, helper.go, types.go.' },
            { role: 'user', content: 'Summarize helper.go in one line.' },
        ] } },
        { id: 'tool', body: { tools: [{ type: 'function', function: { name: 'get_weather', parameters: { type: 'object', properties: { city: { type: 'string' } } } } }], messages: [
            { role: 'user', content: "What's the weather in Tokyo?" },
            { role: 'assistant', tool_calls: [{ id: 'call_01', type: 'function', function: { name: 'get_weather', arguments: '{"city":"Tokyo"}' } }] },
            { role: 'tool', tool_call_id: 'call_01', content: '24°C, clear' },
        ] } },
        { id: 'image', body: { messages: [
            { role: 'user', content: [{ type: 'text', text: 'What colour is this image?' }, { type: 'image_url', image_url: { url: 'https://upload.wikimedia.org/wikipedia/commons/thumb/4/47/PNG_transparency_demonstration_1.png/280px-PNG_transparency_demonstration_1.png' } }] },
        ] } },
    ],
    openai_responses: [
        { id: 'multi', body: { instructions: 'You are a test agent.', input: [
            { role: 'user', content: "What's in ./src?" },
            { role: 'assistant', content: 'Three files: main.go, helper.go, types.go.' },
            { role: 'user', content: 'Summarize helper.go in one line.' },
        ] } },
        { id: 'tool', body: { tools: [{ type: 'function', name: 'get_weather', parameters: { type: 'object', properties: { city: { type: 'string' } } } }], input: [
            { role: 'user', content: "What's the weather in Tokyo?" },
            { type: 'function_call', call_id: 'call_01', name: 'get_weather', arguments: '{"city":"Tokyo"}' },
            { type: 'function_call_output', call_id: 'call_01', output: '24°C, clear' },
        ] } },
        { id: 'image', body: { input: [
            { role: 'user', content: [{ type: 'input_text', text: 'What colour is this image?' }, { type: 'input_image', image_url: 'https://upload.wikimedia.org/wikipedia/commons/thumb/4/47/PNG_transparency_demonstration_1.png/280px-PNG_transparency_demonstration_1.png' }] },
        ] } },
    ],
};

export const BLANK_ID = 'blank';
export const MESSAGE_ID = 'message';

// templatesForProtocol: Blank first (the universal empty starting point),
// then whatever whole-body templates this protocol has — the Content menu's
// options below the 'message' row.
export function templatesForProtocol(protocol: ProbeProtocol): ContentTemplate[] {
    return [{ id: BLANK_ID, body: BLANK_REQUEST[protocol] }, ...(TEMPLATES[protocol] ?? [])];
}

// matchTemplateId: which template (if any) `body` currently equals, ignoring
// JSON formatting — used to label the Content button with something better
// than "Custom" right after picking a template, and to carry the same kind
// of content across a Protocol switch (BenchPage.tsx). A hand-edited body no
// longer matches anything, which is expected: it fell back to being its own,
// unnamed thing the moment it was edited.
export function matchTemplateId(protocol: ProbeProtocol, body: string): string | null {
    let parsed: unknown;
    try {
        parsed = JSON.parse(body);
    } catch {
        return null;
    }
    const serialized = JSON.stringify(parsed);
    for (const tpl of templatesForProtocol(protocol)) {
        if (JSON.stringify(tpl.body) === serialized) return tpl.id;
    }
    return null;
}
