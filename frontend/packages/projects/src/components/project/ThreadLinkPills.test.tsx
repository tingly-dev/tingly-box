import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Thread, ThreadLink } from '../../client';
import { linkTrouble, ThreadLinkPills } from './ThreadLinkPills';
import { ThreadBadges } from './threadStatus';
import { ThreadCard } from '../workspace/StatusColumns';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string, o?: { count?: number }) => (o?.count != null ? `${key}:${o.count}` : key), i18n: { language: 'en' } }) }));
afterEach(cleanup);

const link = (o: Partial<ThreadLink> = {}): ThreadLink => ({
    kind: 'pr', repo: 'acme/app', number: 12, title: 'Fix login', url: 'https://github.com/acme/app/pull/12', state: 'open',
    created_at: '2026-11-20T10:00:00Z', attempted_at: '', refreshed_at: '', ...o,
} as ThreadLink);

const thread = (o: Partial<Thread> = {}): Thread => ({
    id: 't', project_id: 'p', title: 'Fix login', task: '', status: 'idle', origin: 'user', agent: 'claude-code', seq: 1, cwd: '/work/app',
    commits_ahead: 0, files_changed: 0, uncommitted: false, turns: 0, input_tokens: 0, output_tokens: 0, created_at: '', updated_at: '',
    ...o,
} as Thread);

describe('ThreadLinkPills', () => {
    it('is nothing without a link', () => {
        const { container } = render(<ThreadLinkPills links={undefined} />);
        expect(container.innerHTML).toBe('');
    });

    it('shows the open PR first as a link to GitHub, the rest behind +N', () => {
        render(<ThreadLinkPills links={[
            link({ kind: 'issue', number: 3, url: 'https://github.com/acme/app/issues/3' }),
            link({ number: 9, state: 'merged', created_at: '2026-11-21T10:00:00Z' }),
            link({ number: 12 }),
        ]} />);
        const pills = document.querySelectorAll('[data-link-pill]');
        expect(pills).toHaveLength(1);
        expect(pills[0].getAttribute('data-link-pill')).toBe('pr-12');
        expect(pills[0].getAttribute('href')).toBe('https://github.com/acme/app/pull/12');
        expect(pills[0].textContent).toBe('#12');
        expect(screen.getByText('+2')).toBeTruthy();
    });

    it('lists every item from the +N pill', async () => {
        render(<ThreadLinkPills links={[link({ number: 12 }), link({ kind: 'issue', number: 3, title: 'Login broken' })]} />);
        fireEvent.click(document.querySelector('[data-link-more]')!);
        expect(await screen.findByText('#3 Login broken')).toBeTruthy();
        expect(screen.getByText('#12 Fix login')).toBeTruthy();
    });

    it('does not open the row it sits in', () => {
        const onRow = vi.fn();
        render(<div onClick={onRow}><ThreadLinkPills links={[link(), link({ number: 13 })]} /></div>);
        fireEvent.click(document.querySelector('[data-link-pill]')!);
        fireEvent.click(document.querySelector('[data-link-more]')!);
        expect(onRow).not.toHaveBeenCalled();
    });

    it('flags failing checks and conflicts on an open item only', () => {
        expect(linkTrouble(link({ ci: 'failing' }))).toBe(true);
        expect(linkTrouble(link({ merge_state: 'conflicts' }))).toBe(true);
        expect(linkTrouble(link({ ci: 'pending' }))).toBe(false);
        expect(linkTrouble(link({ ci: 'failing', state: 'merged' }))).toBe(false);
        render(<ThreadLinkPills links={[link({ ci: 'failing' })]} />);
        expect(document.querySelector('[data-link-pill]')!.getAttribute('aria-label')).toBe('acme/app#12 Fix login · links.open · links.checksFailing');
    });

    it('drops a link that is not https', () => {
        render(<ThreadLinkPills links={[link({ url: 'javascript:alert(1)' })]} />);
        expect(document.querySelector('[data-link-pill]')!.getAttribute('href')).toBeNull();
    });
});

describe('ThreadBadges with links', () => {
    it('shows the links before the work on a card, and nowhere else', () => {
        const th = thread({ links: [link()], branch: 'tp/x', commits_ahead: 2 });
        const { rerender } = render(<ThreadBadges thread={th} links />);
        expect(document.querySelector('[data-link-pill]')).toBeTruthy();
        expect(screen.getByText('thread.commits:2')).toBeTruthy();
        rerender(<ThreadBadges thread={th} />);
        expect(document.querySelector('[data-link-pill]')).toBeNull();
    });

    it('shows the links of a thread without commits', () => {
        render(<ThreadBadges thread={thread({ links: [link()] })} links />);
        expect(document.querySelector('[data-link-pill]')).toBeTruthy();
    });
});

describe('the PR of the thread\'s own branch', () => {
    it('stands in for the commits pill while open', () => {
        render(<ThreadBadges thread={thread({ links: [link({ head: 'tp/x' })], branch: 'tp/x', commits_ahead: 2 })} links />);
        expect(screen.queryByText('thread.commits:2')).toBeNull();
    });

    it('stands in for the merged pill once merged', () => {
        render(<ThreadBadges thread={thread({ links: [link({ head: 'tp/x', state: 'merged' })], branch: 'tp/x', merged_at: '2026-11-21T10:00:00Z' })} links />);
        expect(screen.queryByText('thread.mergedBadge')).toBeNull();
    });

    it('leaves the commits pill to a PR of another branch', () => {
        render(<ThreadBadges thread={thread({ links: [link({ head: 'other' })], branch: 'tp/x', commits_ahead: 2 })} links />);
        expect(screen.getByText('thread.commits:2')).toBeTruthy();
    });

    it('keeps the commits pill where the links are not shown', () => {
        render(<ThreadBadges thread={thread({ links: [link({ head: 'tp/x' })], branch: 'tp/x', commits_ahead: 2 })} />);
        expect(screen.getByText('thread.commits:2')).toBeTruthy();
    });
});

describe('a board card', () => {
    it('shows its number and its links as pills', () => {
        render(<ThreadCard thread={thread({ seq: 4, links: [link(), link({ kind: 'issue', number: 3 })] })} onOpen={() => {}} />);
        expect(document.querySelector('[data-thread-number]')?.textContent).toBe('#4');
        expect(document.querySelector('[data-link-pill]')?.getAttribute('data-link-pill')).toBe('pr-12');
        expect(screen.getByText('+1')).toBeTruthy();
    });
});
