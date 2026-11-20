import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { PhoneShellContext } from '@tingly/ui/hooks/usePhoneShell';
import type { Provider } from '@/types/provider';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

const models: Record<string, string[]> = {
    'p-openai': ['gpt-5.6-sol', 'gpt-5.6-luna'],
    'p-anthropic': ['claude-sonnet-5', 'claude-opus-4-8'],
};
vi.mock('@/services/api', () => {
    const api = new Proxy({
        getProviderModelsByUUID: async (uuid: string) => ({ success: true, data: { models: models[uuid] ?? [] } }),
    } as Record<string, unknown>, {
        get: (target, key: string) => target[key] ?? (async () => ({ success: false })),
    });
    return { api, default: api, fetchUIAPI: async () => ({}) };
});

import ModelSelectDialog from './ModelSelectDialog';

const providers = [
    { uuid: 'p-openai', name: 'OpenAI', api_style: 'openai', auth_type: 'api_key', enabled: true },
    { uuid: 'p-anthropic', name: 'Anthropic', api_style: 'anthropic', auth_type: 'api_key', enabled: true },
] as unknown as Provider[];

const phone = (ui: ReactNode) => render(<PhoneShellContext.Provider value>{ui}</PhoneShellContext.Provider>);

afterEach(() => {
    cleanup();
    localStorage.clear();
});

describe('ModelSelectDialog in the phone shell', () => {
    it('starts on the provider list when nothing is chosen, then shows one provider’s models', async () => {
        const onSelected = vi.fn();
        phone(<ModelSelectDialog providers={providers} onSelected={onSelected} />);

        // Step one: the providers, no models yet.
        expect(screen.queryByText('gpt-5.6-sol')).not.toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: /Anthropic/ }));

        // Step two: that provider's models, one per row, and a way back.
        expect(await screen.findByText('claude-sonnet-5')).toBeInTheDocument();
        expect(screen.queryByText('OpenAI')).not.toBeInTheDocument();
        fireEvent.click(screen.getByText('claude-opus-4-8'));
        expect(onSelected).toHaveBeenCalledWith(expect.objectContaining({ model: 'claude-opus-4-8', provider: expect.objectContaining({ uuid: 'p-anthropic' }) }));

        fireEvent.click(screen.getByRole('button', { name: 'mobile.modelPicker.backToProviders' }));
        expect(screen.getByRole('button', { name: /OpenAI/ })).toBeInTheDocument();
        expect(screen.queryByText('claude-sonnet-5')).not.toBeInTheDocument();
    });

    it('opens on the chosen model, marked as selected', async () => {
        phone(<ModelSelectDialog providers={providers} selectedProvider="p-openai" selectedModel="gpt-5.6-luna" />);

        const row = (await screen.findByText('gpt-5.6-luna')).closest('.MuiCard-root') as HTMLElement;
        expect(row.querySelector('.tabler-icon-circle-check')).not.toBeNull();
        const other = screen.getByText('gpt-5.6-sol').closest('.MuiCard-root') as HTMLElement;
        expect(other.querySelector('.tabler-icon-circle-check')).toBeNull();
        expect(screen.getByRole('button', { name: 'mobile.modelPicker.backToProviders' })).toBeInTheDocument();
        // The search stays above the list.
        expect(screen.getByPlaceholderText('Search models...')).toBeInTheDocument();
    });

    it('keeps the desktop side-by-side layout outside the phone shell', async () => {
        render(<ModelSelectDialog providers={providers} selectedProvider="p-openai" selectedModel="gpt-5.6-luna" />);

        expect(await screen.findByText('gpt-5.6-luna')).toBeInTheDocument();
        expect(screen.getByText('Anthropic')).toBeInTheDocument();
        expect(screen.queryByRole('button', { name: 'mobile.modelPicker.backToProviders' })).not.toBeInTheDocument();
    });
});
