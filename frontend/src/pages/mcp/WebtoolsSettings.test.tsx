import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import WebtoolsSettings from './WebtoolsSettings';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (_: string, o: { defaultValue: string }) => o.defaultValue }),
}));

describe('Server web tool configuration', () => {
    it('saves credentials without enabling tools or changing the MCP source', async () => {
        const save = vi.fn().mockResolvedValue(undefined);
        render(<WebtoolsSettings source={{ id: 'server-webtools', enabled: false, env: { OTHER: 'kept' } }} onSave={save} />);
        fireEvent.change(screen.getByLabelText('Serper API key'), { target: { value: ' configured-key ' } });
        fireEvent.click(screen.getByRole('button', { name: 'Save web tool settings' }));
        await waitFor(() => expect(save).toHaveBeenCalledWith({
            id: 'server-webtools', env: { OTHER: 'kept', SERPER_API_KEY: 'configured-key' },
        }));
        expect(screen.queryByRole('switch')).toBeNull();
    });

    it('removes a saved search key while preserving unrelated environment settings', async () => {
        const save = vi.fn().mockResolvedValue(undefined);
        render(<WebtoolsSettings source={{ id: 'server-webtools', env: { SERPER_API_KEY: 'old', OTHER: 'kept' } }} onSave={save} />);
        fireEvent.change(screen.getByLabelText('Serper API key'), { target: { value: '' } });
        fireEvent.click(screen.getByRole('button', { name: 'Save web tool settings' }));
        await waitFor(() => expect(save).toHaveBeenCalledWith({ id: 'server-webtools', env: { OTHER: 'kept' } }));
    });
});
