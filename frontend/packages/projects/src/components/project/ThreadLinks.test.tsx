import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { ThreadLink } from '../../client';
import { ThreadLinkRows } from './ThreadLinks';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string, o?: { count?: number }) => (o?.count != null ? `${key}:${o.count}` : key) }) }));
afterEach(cleanup);

const link = (o: Partial<ThreadLink> = {}): ThreadLink => ({
    kind: 'pr', repo: 'acme/app', number: 12, title: 'Fix login', url: 'https://github.com/acme/app/pull/12', state: 'open',
    created_at: '2026-11-20T10:00:00Z', attempted_at: '', refreshed_at: '', ...o,
} as ThreadLink);

const rows = () => [...document.querySelectorAll('[data-link-row]')].map((r) => r.getAttribute('data-link-row'));

describe('ThreadLinkRows', () => {
    it('is nothing without a link', () => {
        const { container } = render(<ThreadLinkRows links={undefined} />);
        expect(container.innerHTML).toBe('');
    });

    it('lists every item, open PR first, each a link to GitHub with where it stands', () => {
        render(<ThreadLinkRows links={[
            link({ kind: 'issue', number: 3, title: 'Login broken', url: 'https://github.com/acme/app/issues/3' }),
            link({ number: 9, state: 'merged', created_at: '2026-11-21T10:00:00Z' }),
            link({ number: 12, ci: 'failing', merge_state: 'conflicts' }),
        ]} />);
        expect(rows()).toEqual(['pr-12', 'issue-3', 'pr-9']);
        expect(screen.getByText('#3 Login broken').closest('a')!.getAttribute('href')).toBe('https://github.com/acme/app/issues/3');
        expect(screen.getByText('links.checksFailing')).toBeTruthy();
        expect(screen.getByText('links.conflicts')).toBeTruthy();
        expect(screen.getByText('links.merged')).toBeTruthy();
    });

    it('does not fold four items: a fold would hide only one', () => {
        render(<ThreadLinkRows links={[1, 2, 3, 4].map((n) => link({ number: n }))} />);
        expect(rows()).toHaveLength(4);
        expect(screen.queryByText(/links\.more/)).toBeNull();
    });

    it('folds past three when that hides two or more, and opens in place', () => {
        render(<ThreadLinkRows links={[1, 2, 3, 4, 5].map((n) => link({ number: n }))} />);
        expect(rows()).toHaveLength(3);
        fireEvent.click(screen.getByText('links.more:2'));
        expect(rows()).toHaveLength(5);
        fireEvent.click(screen.getByText('links.less'));
        expect(rows()).toHaveLength(3);
    });

    it('does not open the row it sits in', () => {
        const onRow = vi.fn();
        render(<div onClick={onRow}><ThreadLinkRows links={[1, 2, 3, 4, 5].map((n) => link({ number: n }))} /></div>);
        fireEvent.click(screen.getByText('#1 Fix login'));
        fireEvent.click(screen.getByText('links.more:2'));
        expect(onRow).not.toHaveBeenCalled();
    });

    it('drops a link that is not https', () => {
        render(<ThreadLinkRows links={[link({ url: 'javascript:alert(1)' })]} />);
        expect(screen.getByText('#12 Fix login').getAttribute('href')).toBeNull();
    });
});
