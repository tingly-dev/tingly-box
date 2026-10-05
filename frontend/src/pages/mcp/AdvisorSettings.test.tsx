import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import AdvisorSettings from './AdvisorSettings';
vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (_: string, o: { defaultValue: string }) => o.defaultValue }),
}));
vi.mock('@/services/api', () => ({
    api: {
        getProviders: vi
            .fn()
            .mockResolvedValue({
                success: true,
                data: [{ uuid: 'review-provider', name: 'Review provider', api_style: 'openai' }],
            }),
    },
}));
vi.mock('@/components/ModelSelectDialog', () => ({
    default: ({ onSelected }: { onSelected: (option: unknown) => void }) => (
        <button onClick={() => onSelected({ provider: { uuid: 'review-provider' }, model: 'review-model' })}>
            Select model fixture
        </button>
    ),
}));
describe('Advisor model configuration', () => {
    it('has no shared connection switch and saves only Advisor fields, preserving unrelated settings', async () => {
        const save = vi.fn().mockResolvedValue(undefined);
        render(
            <AdvisorSettings
                advisorSource={{
                    id: 'advisor',
                    enabled: false,
                    usage: { client: false, gateway: true },
                    tool_policies: { advisor: { enabled: false } },
                    advisor: { timeout_seconds: 9 },
                }}
                onSave={save}
            />
        );
        expect(screen.queryByRole('switch')).toBeNull();
        expect(screen.queryByText('Experimental')).toBeNull();
        fireEvent.click(screen.getByRole('button', { name: 'Choose consultation model' }));
        fireEvent.click(screen.getByRole('button', { name: 'Select model fixture' }));
        fireEvent.click(await screen.findByRole('button', { name: 'Save' }));
        await waitFor(() =>
            expect(save).toHaveBeenCalledWith({
                id: 'advisor',
                advisor: { timeout_seconds: 9, provider_uuid: 'review-provider', model: 'review-model' },
            })
        );
        expect(save.mock.calls[0][0]).not.toHaveProperty('enabled');
        expect(save.mock.calls[0][0]).not.toHaveProperty('tool_policies');
        expect(save.mock.calls[0][0]).not.toHaveProperty('usage');
    });
});
