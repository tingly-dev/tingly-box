import { useState } from 'react';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { DisclosureRow } from './Disclosure';

afterEach(cleanup);

function Fixture() {
    const [open, setOpen] = useState(false);
    return <DisclosureRow open={open} onToggle={() => setOpen(!open)}><span>Details</span><button type="button">inner</button></DisclosureRow>;
}

describe('DisclosureRow', () => {
    it('toggles on click, Enter and Space and reports aria-expanded', () => {
        render(<Fixture />);
        const row = screen.getByRole('button', { name: /Details/ });
        expect(row).toHaveAttribute('aria-expanded', 'false');
        fireEvent.click(row);
        expect(row).toHaveAttribute('aria-expanded', 'true');
        fireEvent.keyDown(row, { key: 'Enter' });
        expect(row).toHaveAttribute('aria-expanded', 'false');
        fireEvent.keyDown(row, { key: ' ' });
        expect(row).toHaveAttribute('aria-expanded', 'true');
    });

    it('ignores keys pressed on a control inside the row', () => {
        render(<Fixture />);
        fireEvent.keyDown(screen.getByRole('button', { name: 'inner' }), { key: 'Enter' });
        expect(screen.getByRole('button', { name: /Details/ })).toHaveAttribute('aria-expanded', 'false');
    });

    it('is plain without onToggle: no button role, no aria-expanded', () => {
        const { container } = render(<DisclosureRow open={false}>Static</DisclosureRow>);
        expect(container.firstElementChild).not.toHaveAttribute('role');
        expect(container.firstElementChild).not.toHaveAttribute('aria-expanded');
    });
});
