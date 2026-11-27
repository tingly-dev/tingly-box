import {cleanup, fireEvent, render, screen} from '@testing-library/react';
import {afterEach, describe, expect, it, vi} from 'vitest';
import {PhoneShellContext} from '@tingly/ui/hooks/usePhoneShell';
import Transcript from './Transcript';
import type {TranscriptEntry} from './model';
import {buildTranscript} from './model';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        i18n: {language: 'en'},
        t: (key: string, opts?: {defaultValue?: string}) => opts?.defaultValue ?? key,
    }),
}));

const entries: TranscriptEntry[] = [
    {role: 'user', content: 'look at the pool', timestamp: '2026-11-19T08:00:00Z'},
    {kind: 'tool_use', content: 'Read', request_id: 't1', payload: {file_path: 'internal/db/pool.go'}, timestamp: '2026-11-19T08:00:02Z'},
    {kind: 'tool_result', content: 'MaxOpenConns: 10', request_id: 't1', timestamp: '2026-11-19T08:00:04Z'},
    {role: 'assistant', content: 'The pool is small.', timestamp: '2026-11-19T08:00:06Z'},
];

const view = (phone: boolean) => {
    const transcript = <Transcript blocks={buildTranscript(entries)} working={false} onRespond={vi.fn()}/>;
    render(phone ? <PhoneShellContext.Provider value>{transcript}</PhoneShellContext.Provider> : transcript);
};

afterEach(cleanup);

describe('the transcript on a phone', () => {
    it('keeps the desktop transcript as it was: the call is an inline row', () => {
        view(false);
        expect(screen.getByText('Read')).toBeInTheDocument();
        expect(screen.queryByRole('button', {name: /Process/})).toBeNull();
    });

    it('folds the run to one Process row and opens the per-call rows in a sheet', () => {
        view(true);
        expect(screen.queryByText('Read')).toBeNull();
        const process = screen.getByRole('button', {name: /Process/});
        fireEvent.click(process);
        expect(screen.getByText('Read')).toBeInTheDocument();
        expect(screen.getByText('This run')).toBeInTheDocument();
    });

    it('keeps a right-side copy action on each message, with the time in the meta row', () => {
        view(true);
        expect(screen.getAllByRole('button', {name: 'Copy message'})).toHaveLength(2);
        // The exact time is visible without hover on a phone.
        expect(screen.getAllByText(/08:00/).length).toBeGreaterThan(0);
    });
});
