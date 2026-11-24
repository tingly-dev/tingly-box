import { createElement } from 'react';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import i18n from '@/i18n';
import { api } from '@/services/api';
import ClaudeCodeProfileOverrides, { profileRuntimePreferences } from './ClaudeCodeProfileOverrides';

vi.mock('@/services/api', () => ({
    api: {
        getClaudeCodeProfileConfig: vi.fn(),
        updateClaudeCodeProfileConfig: vi.fn(),
    },
}));
vi.mock('@/hooks/useNotify', () => ({ useNotify: () => ({ success: vi.fn() }) }));

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
            MCP_TIMEOUT: '1000',
        }, new Set(['ANTHROPIC_MODEL', 'CLAUDE_CODE_MAX_OUTPUT_TOKENS', 'API_TIMEOUT_MS'])))
            .toEqual({ CLAUDE_CODE_MAX_OUTPUT_TOKENS: '64000', API_TIMEOUT_MS: '' });
    });
});

describe('ClaudeCodeProfileOverrides', () => {
    it('saves and removes an override equal to the inherited value', async () => {
        await i18n.changeLanguage('en');
        const response = (overrideKeys: string[]) => ({
            success: true,
            data: { overrideKeys, inheritedDefaultMode: 'auto', defaultMode: 'auto', hasOverrides: overrideKeys.length > 0 },
        });
        vi.mocked(api.getClaudeCodeProfileConfig).mockResolvedValue(response([]));
        const update = vi.mocked(api.updateClaudeCodeProfileConfig);
        update.mockResolvedValueOnce(response(['defaultMode'])).mockResolvedValueOnce(response([]));

        render(createElement(ClaudeCodeProfileOverrides, { profileId: 'p1' }));
        await waitFor(() => expect(screen.getByRole('button', { name: 'Add' })).toBeEnabled());
        fireEvent.click(screen.getByRole('button', { name: 'Add' }));
        fireEvent.click(await screen.findByText('permissions.defaultMode'));
        fireEvent.click(await screen.findByRole('button', { name: 'Save' }));

        expect(update).toHaveBeenNthCalledWith(1, 'claude_code', 'p1', {}, ['defaultMode'], 'auto');
        const remove = await screen.findByRole('button', { name: 'Remove override and restore inheritance' });
        await waitFor(() => expect(remove).toBeEnabled());
        fireEvent.click(remove);
        fireEvent.click(screen.getByRole('button', { name: 'Save' }));
        expect(update).toHaveBeenNthCalledWith(2, 'claude_code', 'p1', {}, [], undefined);
        await waitFor(() => expect(screen.queryByRole('button', { name: 'Save' })).toBeNull());
    }, 20_000);
});
