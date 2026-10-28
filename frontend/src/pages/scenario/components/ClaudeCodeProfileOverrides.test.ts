import { describe, expect, it } from 'vitest';
import { profileRuntimePreferences } from './ClaudeCodeProfileOverrides';

describe('profileRuntimePreferences', () => {
    it('omits routing-owned model slots while retaining runtime settings', () => {
        expect(profileRuntimePreferences({
            ANTHROPIC_MODEL: 'cc',
            ANTHROPIC_DEFAULT_HAIKU_MODEL: 'cc',
            ANTHROPIC_DEFAULT_SONNET_MODEL: 'cc',
            ANTHROPIC_DEFAULT_OPUS_MODEL: 'cc',
            ANTHROPIC_DEFAULT_FABLE_MODEL: 'cc',
            CLAUDE_CODE_SUBAGENT_MODEL: 'cc',
            CLAUDE_CODE_MAX_OUTPUT_TOKENS: '64000',
            API_TIMEOUT_MS: '',
        })).toEqual({ CLAUDE_CODE_MAX_OUTPUT_TOKENS: '64000', API_TIMEOUT_MS: '' });
    });
});
