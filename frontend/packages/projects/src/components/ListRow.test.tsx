import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ListRow } from './ListRow';

afterEach(cleanup);

describe('ListRow', () => {
    it('lays out title, subtitle, meta and the trailing slot, and opens on click and Enter', () => {
        const onClick = vi.fn();
        render(<ListRow title="Fix the list" subtitle="waiting on a reply" meta="3m" end={<span>menu</span>} selected onClick={onClick} />);
        const row = screen.getByRole('button');
        expect(row.getAttribute('aria-current')).toBe('page');
        for (const text of ['Fix the list', 'waiting on a reply', '3m', 'menu']) expect(screen.getByText(text)).toBeTruthy();
        fireEvent.click(row);
        fireEvent.keyDown(row, { key: 'Enter' });
        expect(onClick).toHaveBeenCalledTimes(2);
    });

    it('is not a button without a handler, and passes data attributes through', () => {
        render(<ListRow title="Plain" data-subagent="c1" />);
        expect(screen.queryByRole('button')).toBeNull();
        expect(document.querySelector('[data-subagent="c1"]')).toBeTruthy();
    });
});
