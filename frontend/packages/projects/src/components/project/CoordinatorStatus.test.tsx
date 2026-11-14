import { cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Project, Thread } from '../../client';
import { CoordinatorStatus } from './CoordinatorStatus';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en' } }) }));

afterEach(() => {
    cleanup();
    localStorage.clear();
});

const project = { id: 'p', name: 'Pay', coordinator_model: 'sonnet', coordinator_effort: 'high' } as Project;
const coordinator = (o: Partial<Thread> = {}): Thread => ({
    id: 'main', status: 'idle', turns: 4, input_tokens: 90000, output_tokens: 6000, cache_read_tokens: 72000, cache_write_tokens: 3000,
    context_tokens: 57500, context_window: 200000, last_turn_ms: 4200, last_turn_at: new Date().toISOString(), reported_model: 'sonnet-4', session_id: '1c9d5e7a-0000-4000-8000-000000000000', ...o,
} as Thread);
const props = { project, queued: 0 };
const strip = () => screen.getByRole('region', { name: 'rail.coordinatorStatus' });
const open = () => fireEvent.click(screen.getByRole('button', { name: 'rail.coordinatorStatusShow' }));

describe('CoordinatorStatus', () => {
    it('is one line while folded: the state, the context fill, the turns, the model and the usage', () => {
        render(<CoordinatorStatus {...props} coordinator={coordinator({ status: 'working' })} />);
        const line = strip();
        expect(within(line).getByText('thread.status.working')).toBeInTheDocument();
        expect(within(line).getByText('29%')).toBeInTheDocument();
        expect(within(line).getByText('4')).toBeInTheDocument();
        expect(within(line).getByText('sonnet')).toBeInTheDocument();
        expect(within(line).getByText('96k')).toBeInTheDocument();
        // The cards wait behind the fold.
        expect(screen.queryByRole('region', { name: 'rail.status' })).not.toBeInTheDocument();
    });

    it('opens into the same Status card a thread has, and remembers it per browser', () => {
        render(<CoordinatorStatus {...props} coordinator={coordinator({ status: 'working' })} queued={2} />);
        open();
        expect(localStorage.getItem('tingly-project.coordinator.status')).toBe('1');
        const status = screen.getByRole('region', { name: 'rail.status' });
        expect(within(status).getByText('57.5k / 200k · 29%')).toBeInTheDocument();
        expect(within(status).getByRole('progressbar', { name: 'rail.context' })).toHaveAttribute('aria-valuenow', '29');
        expect(within(status).getByText('4.2s')).toBeInTheDocument();
        expect(within(status).getByText('2')).toBeInTheDocument();
        // A fresh mount reads the fold back.
        cleanup();
        render(<CoordinatorStatus {...props} coordinator={coordinator()} />);
        expect(screen.getByRole('region', { name: 'rail.status' })).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'rail.coordinatorStatusHide' }));
        expect(localStorage.getItem('tingly-project.coordinator.status')).toBe('0');
    });

    it('shows a recovery decision as waiting on you, and hides an empty queue', () => {
        render(<CoordinatorStatus {...props} coordinator={coordinator({ status: 'waiting' })} />);
        expect(within(strip()).getByText('thread.status.waiting')).toBeInTheDocument();
        open();
        expect(screen.queryByText('rail.queued')).not.toBeInTheDocument();
    });

    it('carries its model and its usage with the cache hit rate', () => {
        render(<CoordinatorStatus {...props} coordinator={coordinator()} />);
        open();
        const model = screen.getByRole('region', { name: 'rail.model' });
        expect(within(model).getByText('sonnet')).toBeInTheDocument();
        expect(within(model).getByText('sonnet-4')).toBeInTheDocument();
        expect(within(model).getByText('1c9d5e7a')).toBeInTheDocument();
        const usage = screen.getByRole('region', { name: 'rail.usage' });
        expect(within(usage).getByText('90k / 6k')).toBeInTheDocument();
        expect(within(usage).getByText('72k / 3k')).toBeInTheDocument();
        expect(within(usage).getByText('80%')).toBeInTheDocument();
    });

    it('still renders before the first turn, with nothing to count yet', () => {
        render(<CoordinatorStatus {...props} coordinator={coordinator({ turns: 0, input_tokens: 0, output_tokens: 0, context_tokens: 0, last_turn_ms: 0, last_turn_at: undefined, reported_model: undefined, session_id: undefined })} />);
        expect(within(strip()).getByText('thread.status.idle')).toBeInTheDocument();
        expect(within(strip()).queryByText('rail.usage')).not.toBeInTheDocument();
        open();
        expect(screen.getByText('rail.contextEmpty')).toBeInTheDocument();
        expect(screen.queryByRole('region', { name: 'rail.usage' })).not.toBeInTheDocument();
    });

    it('shows nothing until the coordinator is known', () => {
        render(<CoordinatorStatus {...props} />);
        expect(screen.queryByRole('region', { name: 'rail.coordinatorStatus' })).not.toBeInTheDocument();
    });

    it("shows the host's extra cards after Usage", () => {
        render(<CoordinatorStatus {...props} coordinator={coordinator()} extra={<section aria-label="host card" />} />);
        open();
        const usage = screen.getByRole('region', { name: 'rail.usage' });
        expect(usage.compareDocumentPosition(screen.getByRole('region', { name: 'host card' })) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    });
});
