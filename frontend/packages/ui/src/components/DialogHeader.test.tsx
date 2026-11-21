import { fireEvent, render, screen } from '@testing-library/react';
import { Dialog } from '@mui/material';
import { describe, expect, it, vi } from 'vitest';
import DialogHeader from './DialogHeader';

describe('DialogHeader', () => {
    it('keeps the close action out of the dialog name', () => {
        const onClose = vi.fn();

        render(
            <Dialog open aria-labelledby="dialog-title">
                <DialogHeader
                    title="Add Skill Location"
                    titleId="dialog-title"
                    closeLabel="Close add skill location"
                    onClose={onClose}
                />
            </Dialog>,
        );

        expect(
            screen.getByRole('dialog', { name: 'Add Skill Location' }),
        ).toBeInTheDocument();

        fireEvent.click(
            screen.getByRole('button', { name: 'Close add skill location' }),
        );
        expect(onClose).toHaveBeenCalledOnce();
    });
});
