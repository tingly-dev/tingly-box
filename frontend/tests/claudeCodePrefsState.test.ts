import assert from 'node:assert/strict';
import test from 'node:test';
import { derivePrefsFromRules } from '../src/pages/scenario/components/ClaudeCodeQuickConfig';
import { restoreAppliedClaudeCodePrefs } from '../src/pages/scenario/components/claudeCodePrefsState';

const unified = {
    ANTHROPIC_MODEL: 'tingly/cc',
    ANTHROPIC_DEFAULT_HAIKU_MODEL: 'tingly/cc',
    ANTHROPIC_DEFAULT_SONNET_MODEL: 'tingly/cc',
    ANTHROPIC_DEFAULT_OPUS_MODEL: 'tingly/cc',
    CLAUDE_CODE_SUBAGENT_MODEL: 'tingly/cc',
};

const separate = {
    ANTHROPIC_MODEL: 'tingly/cc-default',
    ANTHROPIC_DEFAULT_HAIKU_MODEL: 'tingly/cc-haiku',
    ANTHROPIC_DEFAULT_SONNET_MODEL: 'tingly/cc-sonnet',
    ANTHROPIC_DEFAULT_OPUS_MODEL: 'tingly/cc-opus',
    CLAUDE_CODE_SUBAGENT_MODEL: 'tingly/cc-subagent',
};

test('fills missing model slots from the current unified rules', () => {
    const restored = restoreAppliedClaudeCodePrefs({
        generated: unified,
        applied: { ANTHROPIC_MODEL: 'tingly/cc', CLAUDE_CODE_MAX_OUTPUT_TOKENS: '64000' },
    });

    assert.equal(restored.ANTHROPIC_DEFAULT_HAIKU_MODEL, 'tingly/cc');
    assert.equal(restored.ANTHROPIC_DEFAULT_OPUS_MODEL, 'tingly/cc');
    assert.equal(restored.CLAUDE_CODE_MAX_OUTPUT_TOKENS, '64000');
});

test('current rule-derived slots replace stale applied model strings', () => {
    const restored = restoreAppliedClaudeCodePrefs({
        generated: unified,
        applied: { ...unified, ANTHROPIC_DEFAULT_HAIKU_MODEL: 'custom/fast' },
    });

    assert.equal(restored.ANTHROPIC_DEFAULT_HAIKU_MODEL, 'tingly/cc');
});

test('regenerates every model slot after switching modes and retains other prefs', () => {
    const restored = restoreAppliedClaudeCodePrefs({
        generated: separate,
        applied: { ...unified, CLAUDE_CODE_MAX_OUTPUT_TOKENS: '64000' },
    });

    assert.deepEqual(
        Object.fromEntries(Object.keys(separate).map(key => [key, restored[key as keyof typeof restored]])),
        separate,
    );
    assert.equal(restored.CLAUDE_CODE_MAX_OUTPUT_TOKENS, '64000');
});

test('separate mode keeps each UUID-derived slot distinct', () => {
    const restored = restoreAppliedClaudeCodePrefs({
        generated: separate,
        applied: unified,
    });

    assert.equal(restored.ANTHROPIC_MODEL, 'tingly/cc-default');
    assert.equal(restored.ANTHROPIC_DEFAULT_HAIKU_MODEL, 'tingly/cc-haiku');
});

test('unified mode resolves the cc rule by UUID instead of array order or canonical name', () => {
    const prefs = derivePrefsFromRules({
        mode: 'unified',
        rules: [
            { uuid: 'unrelated', request_model: 'wrong/model' },
            {
                uuid: 'builtin:claude_code:cc',
                request_model: 'team/custom-route',
                flags: { context_1m: true },
            },
        ],
    });

    assert.equal(prefs.ANTHROPIC_MODEL, 'team/custom-route[1m]');
    assert.equal(prefs.ANTHROPIC_DEFAULT_HAIKU_MODEL, 'team/custom-route[1m]');
    assert.equal(prefs.CLAUDE_CODE_SUBAGENT_MODEL, 'team/custom-route[1m]');
});

test('separate mode resolves each custom request model by its tier rule UUID', () => {
    const prefs = derivePrefsFromRules({
        mode: 'separate',
        rules: [
            { uuid: 'builtin:claude_code:opus', request_model: 'routes/deep' },
            { uuid: 'builtin:claude_code:default', request_model: 'routes/default' },
            { uuid: 'builtin:claude_code:subagent', request_model: 'routes/agent' },
            { uuid: 'builtin:claude_code:sonnet', request_model: 'routes/main' },
            { uuid: 'builtin:claude_code:haiku', request_model: 'routes/fast' },
        ],
    });

    assert.equal(prefs.ANTHROPIC_MODEL, 'routes/default');
    assert.equal(prefs.ANTHROPIC_DEFAULT_HAIKU_MODEL, 'routes/fast');
    assert.equal(prefs.ANTHROPIC_DEFAULT_SONNET_MODEL, 'routes/main');
    assert.equal(prefs.ANTHROPIC_DEFAULT_OPUS_MODEL, 'routes/deep');
    assert.equal(prefs.CLAUDE_CODE_SUBAGENT_MODEL, 'routes/agent');
});
