import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { getResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { handlers } from '@/mocks/handlers';
import { flagsToApi } from './flagHelpers';
import FlagCatalogDialog from './FlagCatalogDialog';

afterEach(cleanup);

describe('thinking budget rule flag', () => {
    it('appears under Reasoning and saves the API flag', async () => {
        // Resolve the mock handler directly: a real fetch can leave the msw
        // interceptor when the dialog's import graph is loaded and hit whatever
        // listens on the jsdom origin.
        const response = await getResponse(handlers, new Request(`${window.location.origin}/api/v1/rule/flags/registry`));
        const { data } = await response!.json();
        expect(data).toContainEqual(expect.objectContaining({
            key: 'disable_thinking_budget', type: 'bool', category: 'reasoning',
        }));

        const onSave = vi.fn();
        render(<FlagCatalogDialog open registry={data} onClose={() => {}} onSave={onSave} />);
        fireEvent.click(screen.getByText('Reasoning'));
        expect(screen.getByText('Remove thinking budget')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('switch'));
        fireEvent.click(screen.getByRole('button', { name: 'Save' }));
        expect(flagsToApi(onSave.mock.calls[0][0])).toMatchObject({ disable_thinking_budget: true });
    });
});
