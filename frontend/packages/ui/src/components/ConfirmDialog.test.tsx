import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import ConfirmDialog from './ConfirmDialog';

describe('ConfirmDialog', () => {
    it('provides labelled cancel and confirm actions', () => {
        const onClose = vi.fn();
        const onConfirm = vi.fn();

        render(
            <ConfirmDialog
                open
                title="Delete location?"
                description="Files on disk will not be deleted."
                confirmLabel="Delete location"
                confirmColor="error"
                onClose={onClose}
                onConfirm={onConfirm}
            />,
        );

        expect(
            screen.getByRole('dialog', { name: 'Delete location?' }),
        ).toHaveAccessibleDescription('Files on disk will not be deleted.');

        fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
        fireEvent.click(screen.getByRole('button', { name: 'Delete location' }));

        expect(onClose).toHaveBeenCalledOnce();
        expect(onConfirm).toHaveBeenCalledOnce();
    });

    it('locks both actions while confirmation is in progress', () => {
        render(
            <ConfirmDialog
                open
                title="Delete location?"
                description="Deleting this location."
                confirmLabel="Delete location"
                confirmingLabel="Deleting…"
                loading
                onClose={vi.fn()}
                onConfirm={vi.fn()}
            />,
        );

        expect(screen.getByRole('dialog')).toHaveAttribute('aria-busy', 'true');
        expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled();
        expect(screen.getByRole('button', { name: 'Deleting…' })).toBeDisabled();
    });
});
