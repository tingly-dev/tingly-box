import {cleanup, fireEvent, render, screen} from '@testing-library/react';
import {afterEach, describe, expect, it, vi} from 'vitest';
import Transcript from './Transcript';
import type {TranscriptBlock} from './model';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (_key: string, opts: {defaultValue: string} & Record<string, unknown>) => opts.defaultValue.replace(/\{\{(\w+)\}\}/g, (_m, name: string) => String(opts[name])),
    }),
}));
afterEach(cleanup);

const card = (payload: Record<string, unknown>): TranscriptBlock => ({
    type: 'command',
    message: {kind: 'command', content: 'compact', payload: {name: 'compact', ...payload}, timestamp: '2026-10-04T00:00:00Z'},
});
const show = (block: TranscriptBlock) => render(<Transcript blocks={[block]} working={false} onRespond={vi.fn()}/>);

describe('command card', () => {
    it('shows a compaction in progress', () => {
        show(card({state: 'running', trigger: 'manual'}));
        expect(screen.getByRole('status')).toHaveTextContent('Compacting conversation…');
    });

    it('shows the figures the agent reported and opens the summary on demand', () => {
        show(card({state: 'ok', trigger: 'manual', tokens_before: 120000, tokens_after: 18000, tokens_reported: true, duration_ms: 4200, summary: 'The summary text'}));
        const status = screen.getByRole('status');
        expect(status).toHaveTextContent('Conversation compacted');
        expect(status).toHaveTextContent('120.0k → 18.0k tokens · 4s');
        expect(screen.queryByText('The summary text')).not.toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', {name: 'Summary'}));
        expect(screen.getByText('The summary text')).toBeInTheDocument();
    });

    it('does not invent figures the agent did not report, and marks an automatic compaction', () => {
        show(card({state: 'ok', trigger: 'auto', tokens_before: 0, tokens_after: 0}));
        const status = screen.getByRole('status');
        expect(status).toHaveTextContent('Conversation compacted');
        expect(status).toHaveTextContent('automatic');
        expect(status).not.toHaveTextContent('tokens');
    });

    it('shows a fresh session distinctly from compaction', () => {
        show(card({name: 'new', state: 'ok'}));
        expect(screen.getByRole('status')).toHaveTextContent('New session started');
        expect(screen.getByRole('status')).not.toHaveTextContent('compact');
    });

    it('shows a failed new session with its reason', () => {
        show(card({name: 'new', state: 'failed', reason: 'Session is busy.'}));
        expect(screen.getByRole('status')).toHaveTextContent('Could not start a new session: Session is busy');
    });

    it('says why a compaction could not run', () => {
        show(card({state: 'failed', reason: 'Not enough messages to compact.'}));
        expect(screen.getByRole('status')).toHaveTextContent('Could not compact: Not enough messages to compact');
    });
});
