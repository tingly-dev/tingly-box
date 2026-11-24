import { expect, test } from 'vitest';
import { derivePrefsFromRules } from './ClaudeCodeQuickConfig';
import { restoreAppliedClaudeCodePrefs } from './claudeCodePrefsState';

const defaultModels = {
    ANTHROPIC_MODEL: 'tingly/cc',
    ANTHROPIC_DEFAULT_HAIKU_MODEL: 'tingly/cc',
    ANTHROPIC_DEFAULT_SONNET_MODEL: 'tingly/cc',
    ANTHROPIC_DEFAULT_OPUS_MODEL: 'tingly/cc',
    ANTHROPIC_DEFAULT_FABLE_MODEL: 'tingly/cc',
    CLAUDE_CODE_SUBAGENT_MODEL: 'tingly/cc',
};

const splitModels = {
    ANTHROPIC_MODEL: 'routes/custom-default',
    ANTHROPIC_DEFAULT_HAIKU_MODEL: 'tingly/cc-haiku',
    ANTHROPIC_DEFAULT_SONNET_MODEL: 'tingly/cc-sonnet',
    ANTHROPIC_DEFAULT_OPUS_MODEL: 'tingly/cc-opus',
    ANTHROPIC_DEFAULT_FABLE_MODEL: 'tingly/cc-fable',
    CLAUDE_CODE_SUBAGENT_MODEL: 'tingly/cc-subagent',
};

test('fills missing model slots from the current default rule', () => {
    const restored = restoreAppliedClaudeCodePrefs({
        generated: defaultModels,
        applied: { ANTHROPIC_MODEL: 'tingly/cc', CLAUDE_CODE_MAX_OUTPUT_TOKENS: '64000' },
    });

    expect(restored.ANTHROPIC_DEFAULT_HAIKU_MODEL).toBe('tingly/cc');
    expect(restored.ANTHROPIC_DEFAULT_OPUS_MODEL).toBe('tingly/cc');
    expect(restored.CLAUDE_CODE_MAX_OUTPUT_TOKENS).toBe('64000');
});

test('current rule-derived slots replace stale applied model strings', () => {
    const restored = restoreAppliedClaudeCodePrefs({
        generated: defaultModels,
        applied: { ...defaultModels, ANTHROPIC_DEFAULT_HAIKU_MODEL: 'custom/fast' },
    });

    expect(restored.ANTHROPIC_DEFAULT_HAIKU_MODEL).toBe('tingly/cc');
});

test('refreshes model slots while retaining other preferences', () => {
    const restored = restoreAppliedClaudeCodePrefs({
        generated: splitModels,
        applied: { ...defaultModels, CLAUDE_CODE_MAX_OUTPUT_TOKENS: '64000' },
    });

    expect(restored).toMatchObject(splitModels);
    expect(restored.CLAUDE_CODE_MAX_OUTPUT_TOKENS).toBe('64000');
});

test('keeps regenerated slots distinct', () => {
    const restored = restoreAppliedClaudeCodePrefs({
        generated: splitModels,
        applied: defaultModels,
    });

    expect(restored.ANTHROPIC_MODEL).toBe('routes/custom-default');
    expect(restored.ANTHROPIC_DEFAULT_HAIKU_MODEL).toBe('tingly/cc-haiku');
});

test('resolves the default cc rule by UUID instead of array order or canonical name', () => {
    const prefs = derivePrefsFromRules({
        rules: [
            { uuid: 'unrelated', request_model: 'wrong/model' },
            {
                uuid: 'builtin:claude_code:cc',
                request_model: 'team/custom-route',
                flags: { context_1m: true },
            },
        ],
    });

    expect(prefs.ANTHROPIC_MODEL).toBe('team/custom-route[1m]');
    expect(prefs.ANTHROPIC_DEFAULT_HAIKU_MODEL).toBe('team/custom-route[1m]');
    expect(prefs.CLAUDE_CODE_SUBAGENT_MODEL).toBe('team/custom-route[1m]');
});

test('resolves enabled model slots by their rule UUIDs', () => {
    const prefs = derivePrefsFromRules({
        slots: ['haiku', 'sonnet', 'opus', 'subagent'],
        rules: [
            { uuid: 'builtin:claude_code:opus', request_model: 'routes/deep' },
            { uuid: 'builtin:claude_code:cc', request_model: 'routes/default' },
            { uuid: 'builtin:claude_code:subagent', request_model: 'routes/agent' },
            { uuid: 'builtin:claude_code:sonnet', request_model: 'routes/main' },
            { uuid: 'builtin:claude_code:haiku', request_model: 'routes/fast' },
        ],
    });

    expect(prefs.ANTHROPIC_MODEL).toBe('routes/default');
    expect(prefs.ANTHROPIC_DEFAULT_HAIKU_MODEL).toBe('routes/fast');
    expect(prefs.ANTHROPIC_DEFAULT_SONNET_MODEL).toBe('routes/main');
    expect(prefs.ANTHROPIC_DEFAULT_OPUS_MODEL).toBe('routes/deep');
    expect(prefs.CLAUDE_CODE_SUBAGENT_MODEL).toBe('routes/agent');
});

test('uses the default model for a split fable slot whose rule is missing or off', () => {
    const base = [
        { uuid: 'builtin:claude_code:cc', request_model: 'routes/default' },
        { uuid: 'builtin:claude_code:opus', request_model: 'routes/deep' },
    ];
    const missing = derivePrefsFromRules({ slots: ['fable'], rules: base });
    expect(missing.ANTHROPIC_DEFAULT_FABLE_MODEL).toBe('routes/default');

    const inactive = derivePrefsFromRules({
        slots: ['fable'],
        rules: [...base, { uuid: 'builtin:claude_code:fable', request_model: 'routes/fable', active: false }],
    });
    expect(inactive.ANTHROPIC_DEFAULT_FABLE_MODEL).toBe('routes/default');

    const active = derivePrefsFromRules({
        slots: ['fable'],
        rules: [...base, { uuid: 'builtin:claude_code:fable', request_model: 'routes/fable', active: true }],
    });
    expect(active.ANTHROPIC_DEFAULT_FABLE_MODEL).toBe('routes/fable');
});

test('uses the default rule for unsplit slots and split slots with inactive rules', () => {
    const prefs = derivePrefsFromRules({
        slots: ['subagent', 'opus'],
        rules: [
            { uuid: 'builtin:claude_code:cc', request_model: 'tingly/cc', active: true },
            { uuid: 'builtin:claude_code:subagent', request_model: 'tingly/cc-subagent', active: true, flags: { context_1m: true } },
            // Listed but switched off: falls back to the default rule.
            { uuid: 'builtin:claude_code:opus', request_model: 'tingly/cc-opus', active: false },
            // Active but not listed (e.g. after `agent apply`): ignored.
            { uuid: 'builtin:claude_code:haiku', request_model: 'tingly/cc-haiku', active: true },
        ],
    });

    expect(prefs.ANTHROPIC_MODEL).toBe('tingly/cc');
    expect(prefs.ANTHROPIC_DEFAULT_HAIKU_MODEL).toBe('tingly/cc');
    expect(prefs.ANTHROPIC_DEFAULT_OPUS_MODEL).toBe('tingly/cc');
    expect(prefs.CLAUDE_CODE_SUBAGENT_MODEL).toBe('tingly/cc-subagent[1m]');
    // The split slot's 1M widens the compact window, as the backend does.
    expect(prefs.CLAUDE_CODE_AUTO_COMPACT_WINDOW).toBe('1000000');
});
