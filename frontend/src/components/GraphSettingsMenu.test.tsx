import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import GraphSettingsMenu from './GraphSettingsMenu';

// No i18n instance in tests: t() returns the key, which is what we match on.
const base = {
    allowDeleteRule: true,
    active: true,
    allowToggleRule: true,
    saving: false,
    onDelete: vi.fn(),
    onToggleActive: vi.fn(),
};

const openMenu = () => fireEvent.click(screen.getAllByRole('button')[0]);

describe('GraphSettingsMenu ordering', () => {
    it('offers only "Pin to top" — moving is drag and drop now', () => {
        const onPin = vi.fn();
        render(<GraphSettingsMenu {...base} pin={{ onPin, canPin: true }} />);
        openMenu();
        expect(screen.queryByText('common.moveUp')).toBeNull();
        expect(screen.queryByText('common.moveDown')).toBeNull();
        fireEvent.click(screen.getByRole('menuitem', { name: 'common.pinToTop' }));
        expect(onPin).toHaveBeenCalledOnce();
    });

    it('disables "Pin to top" for the first rule', () => {
        render(<GraphSettingsMenu {...base} pin={{ onPin: vi.fn(), canPin: false }} />);
        openMenu();
        expect(screen.getByRole('menuitem', { name: 'common.pinToTop' })).toHaveAttribute('aria-disabled', 'true');
    });

    it('has no pin item when ordering is off', () => {
        render(<GraphSettingsMenu {...base} />);
        openMenu();
        expect(screen.queryByText('common.pinToTop')).toBeNull();
    });
});
