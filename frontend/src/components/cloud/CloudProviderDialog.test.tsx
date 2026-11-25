import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import CloudProviderDialog from './CloudProviderDialog';

const { getConfig, templates } = vi.hoisted(() => ({
    getConfig: vi.fn(async () => ({ data: { http_transport: { global_proxy_url: 'http://proxy.test' } } })),
    templates: {
        bedrock: { id: 'bedrock', name: 'Bedrock', auth_type: 'aws_sigv4', api_style: 'anthropic' },
        vertex: { id: 'vertex', name: 'Vertex', auth_type: 'gcp_sa', api_style: 'anthropic' },
    },
}));
vi.mock('@/services/serviceProviders', () => ({
    getServiceProvider: (id: keyof typeof templates) => templates[id] ?? null,
}));
vi.mock('@/components/ProviderIcon', () => ({ default: () => null }));
vi.mock('../../services/api', () => ({ api: { getConfig } }));
vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (key: string, options?: { defaultValue?: string }) => options?.defaultValue ?? key }),
}));

describe('CloudProviderDialog', () => {
    it('keeps edits within an opening and resets the form for a new or reopened template', async () => {
        const props = { onClose: vi.fn(), onSuccess: vi.fn() };
        const { rerender } = render(<CloudProviderDialog {...props} open presetId="bedrock" />);
        const name = () => screen.getByRole('textbox', { name: /Name/ });

        expect(name()).toHaveValue('Bedrock');
        fireEvent.change(name(), { target: { value: 'my provider' } });
        rerender(<CloudProviderDialog {...props} open presetId="bedrock" onClose={vi.fn()} />);
        expect(name()).toHaveValue('my provider');

        rerender(<CloudProviderDialog {...props} open presetId="vertex" />);
        expect(name()).toHaveValue('Vertex');
        fireEvent.change(name(), { target: { value: 'temporary' } });

        rerender(<CloudProviderDialog {...props} open={false} presetId={null} />);
        // Keep the exiting dialog mounted through its transition.
        expect(screen.getByDisplayValue('temporary')).toBeInTheDocument();
        rerender(<CloudProviderDialog {...props} open presetId="vertex" />);
        expect(name()).toHaveValue('Vertex');
        await waitFor(() => expect(getConfig).toHaveBeenCalled());
    });
});
