import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import MCPToolCard from './MCPToolCard';
import type { MCPCatalogTool } from './types';

const search: MCPCatalogTool = {
    source_id: 'server-webtools', name: 'mcp_web_search', normalized_name: 'search',
    enabled: true, usage: { client: false, gateway: true }, implementation: 'mcp',
    backend: 'deepseek', ready: true,
};

describe('DeepSeek search readiness', () => {
    it('allows testing without a separate Serper key', () => {
        render(<MCPToolCard tool={search} source={{ id: 'server-webtools' }}
            enabled={true} busy={false} onChange={vi.fn()} onTest={vi.fn()} />);
        expect(screen.getByRole('button', { name: 'Test tool' })).toBeEnabled();
        expect(screen.queryByText(/Search needs/)).not.toBeInTheDocument();
    });
    it('respects unavailable backend readiness', () => {
        render(<MCPToolCard tool={{ ...search, ready: false }} source={{ id: 'server-webtools' }}
            enabled={true} busy={false} onChange={vi.fn()} onTest={vi.fn()} />);
        expect(screen.getByRole('button', { name: 'Test tool' })).toBeDisabled();
    });
});
