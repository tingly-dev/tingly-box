import type { ClaudeCodePrefs } from './ClaudeCodeQuickConfig';

const MODEL_SLOT_KEYS = [
    'ANTHROPIC_MODEL',
    'ANTHROPIC_DEFAULT_HAIKU_MODEL',
    'ANTHROPIC_DEFAULT_SONNET_MODEL',
    'ANTHROPIC_DEFAULT_OPUS_MODEL',
    'CLAUDE_CODE_SUBAGENT_MODEL',
] as const satisfies readonly (keyof ClaudeCodePrefs)[];

interface RestoreAppliedPrefsInput {
    generated: ClaudeCodePrefs;
    applied?: ClaudeCodePrefs;
}

// Applied settings persist user-tunable values, but model slots are routing
// artifacts and always come from the current mode's well-known rule UUIDs.
// This prevents stale model strings from masking a renamed request_model.
export const restoreAppliedClaudeCodePrefs = ({
    generated,
    applied = {},
}: RestoreAppliedPrefsInput): ClaudeCodePrefs => {
    const restored: ClaudeCodePrefs = { ...generated, ...applied };
    for (const key of MODEL_SLOT_KEYS) {
        const generatedValue = generated[key];
        if (generatedValue === undefined) {
            delete restored[key];
        } else {
            restored[key] = generatedValue;
        }
    }
    return restored;
};
