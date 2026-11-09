import { describe, expect, it } from 'vitest';
import { CLAUDE_CODE_FIELD_STRUCT, derivePrefsFromRules } from './ClaudeCodeQuickConfig';
import { restoreAppliedClaudeCodePrefs } from './claudeCodePrefsState';

describe('CLAUDE_CODE_AUTO_MODE_SERVER', () => {
    const generated = derivePrefsFromRules({ rules: [] });

    it('defaults to off', () => {
        expect(generated.CLAUDE_CODE_AUTO_MODE_SERVER).toBe('0');
    });

    it('keeps the user\'s applied choice over the default', () => {
        const restored = restoreAppliedClaudeCodePrefs({
            generated,
            applied: { CLAUDE_CODE_AUTO_MODE_SERVER: '1' },
        });
        expect(restored.CLAUDE_CODE_AUTO_MODE_SERVER).toBe('1');
    });

    it('takes the default for prefs applied before the option existed', () => {
        const restored = restoreAppliedClaudeCodePrefs({
            generated,
            applied: { CLAUDE_CODE_MAX_OUTPUT_TOKENS: '64000' },
        });
        expect(restored.CLAUDE_CODE_AUTO_MODE_SERVER).toBe('0');
    });

    it('is a top-level row that switches off to an explicit 0', () => {
        const field = CLAUDE_CODE_FIELD_STRUCT.find(f => f.envName === 'CLAUDE_CODE_AUTO_MODE_SERVER');
        expect(field).toMatchObject({ group: 'behavior', kind: 'bool', offValue: '0' });
        expect(field?.advanced).toBeFalsy();
    });
});
