import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import ConfirmPopover from './ConfirmPopover';

describe('ConfirmPopover', () => {
    it('stays closed without an anchor', () => {
        render(<ConfirmPopover anchorEl={null} title="Delete?" onClose={vi.fn()} onConfirm={vi.fn()} />);
        expect(screen.queryByRole('alertdialog')).toBeNull();
    });

    it('opens at the anchor with labelled cancel and confirm actions', () => {
        const anchor = document.body.appendChild(document.createElement('button'));
        const onClose = vi.fn();
        const onConfirm = vi.fn();
        render(
            <ConfirmPopover
                anchorEl={anchor}
                title="Delete rule?"
                description="This cannot be undone."
                confirmLabel="Delete"
                confirmColor="error"
                onClose={onClose}
                onConfirm={onConfirm}
            />,
        );
        expect(screen.getByRole('alertdialog', { name: 'Delete rule?' })).toHaveAccessibleDescription(
            'This cannot be undone.',
        );
        fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
        fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
        expect(onClose).toHaveBeenCalledOnce();
        expect(onConfirm).toHaveBeenCalledOnce();
    });
});
