import { fireEvent, render, screen, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { ProviderListContent } from './ConnectProviderDialog';

vi.mock('@/services/api.ts', () => ({
    default: {
        getProviderCatalogs: async () => ({
            success: true,
            data: (await import('../../../internal/catalog/providers.json')).default.providers,
        }),
    },
}));

describe('self-hosted catalog in Connect AI', () => {
    it.each([
        ['vllm', 'vLLM', 'vllm', 8000],
        ['sglang', 'SGLang', 'sglang', 30000],
        ['llama.cpp', 'llama.cpp', 'llamacpp', 9931],
        ['omlx', 'oMLX', 'omlx', 8000],
    ] as const)('finds %s and opens its local connection preset', async (query, label, id, port) => {
        const onSelect = vi.fn();
        render(<ProviderListContent query={query} onQueryChange={vi.fn()} onSelect={onSelect} />);
        const card = await screen.findByRole('button', { name: name => name.startsWith(`${label}:`) });
        expect(screen.getByRole('heading', { name: 'Self-hosted' })).toBeInTheDocument();
        expect(screen.queryByRole('heading', { name: 'API key providers' })).not.toBeInTheDocument();
        expect(within(card).getByRole('img')).toHaveAttribute('alt', label);
        fireEvent.click(card);
        expect(onSelect).toHaveBeenCalledWith({
            kind: 'local',
            provider: expect.objectContaining({
                id,
                region: 'self-hosted',
                baseUrlOpenAI: `http://localhost:${port}/v1`,
                baseUrlAnthropic: `http://localhost:${port}`,
            }),
        });
    });
});
