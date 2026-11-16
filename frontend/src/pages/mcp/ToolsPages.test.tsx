import { fireEvent, render, screen, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import MCPServerToolsPage from './MCPServerToolsPage';
import MCPToolsPage from './MCPToolsPage';

const mocks = vi.hoisted(() => ({
    reload: vi.fn(),
    run: vi.fn(),
    loading: false,
    checking: false,
    busy: false,
}));
vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (_: string, options: { defaultValue: string }) => options.defaultValue }),
}));
vi.mock('./useMCPWorkspace', () => ({
    useMCPWorkspace: () => ({
        ...mocks,
        config: { sources: [] },
        routing: { sources: [] },
        enabled: true,
    }),
}));
vi.mock('./MCPSourceDrawer', () => ({ default: () => null }));
vi.mock('./MCPWorkspaceAlerts', () => ({ default: () => null }));
vi.mock('./MCPSourceEditor', () => ({ default: () => null }));

beforeEach(() => {
    vi.clearAllMocks();
    mocks.loading = false;
    mocks.checking = false;
    mocks.busy = false;
});

describe.each([
    { title: 'Server Tools', Page: MCPServerToolsPage },
    { title: 'MCP Tools', Page: MCPToolsPage },
])('$title page layout', ({ title, Page }) => {
    it('uses the shared page heading and keeps refresh wired to the workspace', () => {
        render(<MemoryRouter><Page /></MemoryRouter>);
        expect(screen.getByRole('heading', { level: 1, name: title })).toHaveClass('MuiTypography-h3');
        fireEvent.click(screen.getByRole('button', { name: 'Refresh connections' }));
        expect(mocks.run).toHaveBeenCalledWith(mocks.reload);
    });
    it('keeps refresh disabled while checking', () => {
        mocks.checking = true;
        render(<MemoryRouter><Page /></MemoryRouter>);
        expect(screen.getByRole('button', { name: 'Refresh connections' })).toBeDisabled();
    });
    it('does not show content before configuration is loaded', () => {
        mocks.loading = true;
        render(<MemoryRouter><Page /></MemoryRouter>);
        expect(screen.queryByRole('heading', { level: 1 })).not.toBeInTheDocument();
    });
});

it('still opens and validates the MCP connection dialog', () => {
    render(<MemoryRouter><MCPToolsPage /></MemoryRouter>);
    fireEvent.click(screen.getByRole('button', { name: 'Connect tools' }));
    const dialog = screen.getByRole('dialog', { name: 'Connect tools' });
    expect(within(dialog).getByRole('textbox', { name: 'Connection name' })).toBeRequired();
    expect(within(dialog).getByRole('button', { name: 'Connect and inspect tools' })).toBeDisabled();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }));
});
