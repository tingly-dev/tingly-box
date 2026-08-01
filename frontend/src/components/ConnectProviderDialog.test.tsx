import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import ConnectProviderDialog, { ProviderListContent } from './ConnectProviderDialog';

describe('ConnectProviderDialog', () => {
    it('exposes a labelled dialog and close action', () => {
        const onClose = vi.fn();

        render(
            <ConnectProviderDialog
                open
                onClose={onClose}
                onSelect={vi.fn()}
            />,
        );

        expect(screen.getByRole('dialog', { name: 'Connect AI' })).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'Close Connect AI' }));
        expect(onClose).toHaveBeenCalledOnce();
    });

    it('renders provider choices as native keyboard-operable buttons', () => {
        const onSelect = vi.fn();

        render(
            <ProviderListContent
                onSelect={onSelect}
                query=""
                onQueryChange={vi.fn()}
            />,
        );

        const customEndpoint = screen.getByRole('button', {
            name: /Custom endpoint: Not listed\? Bring your own URL/,
        });
        expect(customEndpoint.tagName).toBe('BUTTON');

        fireEvent.keyDown(customEndpoint, { key: 'Enter' });
        expect(onSelect).toHaveBeenCalledWith({ kind: 'custom' });
        expect(screen.getByRole('textbox', { name: 'Search providers' })).toBeInTheDocument();
    });

    it('renders the Paste & detect card as a keyboard-operable button emitting kind:paste', () => {
        const onSelect = vi.fn();

        render(
            <ProviderListContent
                onSelect={onSelect}
                query=""
                onQueryChange={vi.fn()}
            />,
        );

        const pasteCard = screen.getByRole('button', {
            name: /Paste & detect: Paste a \.env, curl, or JSON/,
        });
        expect(pasteCard.tagName).toBe('BUTTON');

        fireEvent.click(pasteCard);
        expect(onSelect).toHaveBeenCalledWith({ kind: 'paste' });

        // keyboard operability: Enter and Space both fire
        fireEvent.keyDown(pasteCard, { key: 'Enter' });
        fireEvent.keyDown(pasteCard, { key: ' ' });
        expect(onSelect).toHaveBeenCalledTimes(3);
    });

    it('renders the Import card emitting kind:import', () => {
        const onSelect = vi.fn();

        render(
            <ProviderListContent
                onSelect={onSelect}
                query=""
                onQueryChange={vi.fn()}
            />,
        );

        fireEvent.click(screen.getByRole('button', { name: /Import: From file or clipboard/ }));
        expect(onSelect).toHaveBeenCalledWith({ kind: 'import' });
    });
});
