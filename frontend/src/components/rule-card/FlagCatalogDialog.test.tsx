import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { setupServer } from 'msw/node';
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import { handlers } from '@/mocks/handlers';
import { flagsToApi } from './flagHelpers';
import FlagCatalogDialog from './FlagCatalogDialog';

const server = setupServer(...handlers);

beforeAll(() => server.listen({ onUnhandledFrame: 'error' }));
afterEach(() => { cleanup(); server.resetHandlers(); });
afterAll(() => server.close());

describe('thinking budget rule flag', () => {
    it('appears under Reasoning and saves the API flag', async () => {
        const response = await fetch(`${window.location.origin}/api/v1/rule/flags/registry`);
        const { data } = await response.json();
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
