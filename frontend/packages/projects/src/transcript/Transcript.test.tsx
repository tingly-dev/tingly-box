import {act, cleanup, fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import type {DraftStorage} from './context';
import {createMemoryDraftStorage} from './context';
import Transcript from './Transcript';
import type {TranscriptBlock, TranscriptEntry} from './model';
import {buildTranscript} from './model';

vi.mock('react-i18next', () => ({useTranslation: () => ({t: (_key: string, opts: {defaultValue: string}) => opts.defaultValue})}));
// A fresh store per test; within one, it outlives unmounts like the host's does.
let drafts: DraftStorage;
beforeEach(() => {drafts = createMemoryDraftStorage();});
afterEach(cleanup);
const request = (id = 'r1', kind = 'approval_request'): TranscriptBlock => ({type: 'request', message: {
    content: 'Bash', kind, request_id: id, payload: {command: 'first line\nsecond line'}, timestamp: '2026-10-04T00:00:00Z',
}});
const show = (respond: (id: string, approved: boolean, answer: string) => Promise<boolean>, block = request()) =>
    render(<Transcript blocks={[block]} pendingRequestId={block.type === 'request' ? block.message.request_id : undefined} working onRespond={respond} drafts={drafts}/>);

describe('pending requests', () => {
    it('shows the whole command being approved and never resubmits an accepted decision while polling catches up', async () => {
        let finish!: (accepted: boolean) => void;
        const respond = vi.fn(() => new Promise<boolean>((resolve) => {finish = resolve;}));
        show(respond);
        expect(screen.getByText(/second line/)).toBeInTheDocument();
        const allow = screen.getByRole('button', {name: 'Allow'});
        fireEvent.click(allow);
        fireEvent.click(allow);
        expect(respond).toHaveBeenCalledTimes(1);
        await act(async () => finish(true));
        expect(allow).toBeDisabled();
        expect(screen.getByRole('button', {name: 'Deny'})).toBeDisabled();
        fireEvent.click(allow);
        expect(respond).toHaveBeenCalledTimes(1);
    });

    it('keeps a failed question answer, allows retry and restores it after navigation', async () => {
        const respond = vi.fn().mockResolvedValueOnce(false).mockResolvedValueOnce(true);
        const view = show(respond, request('question', 'ask_request'));
        const answer = screen.getByRole('textbox', {name: 'Your answer'});
        expect(screen.getByRole('button', {name: 'Answer'})).toBeDisabled();
        expect(answer).not.toHaveFocus();
        fireEvent.change(answer, {target: {value: 'Keep this answer'}});
        await act(async () => fireEvent.keyDown(answer, {key: 'Enter', ctrlKey: true}));
        expect(answer).toHaveValue('Keep this answer');
        expect(screen.getByRole('alert')).toHaveTextContent('try again');
        view.unmount();
        show(respond, request('question', 'ask_request'));
        expect(screen.getByRole('textbox', {name: 'Your answer'})).toHaveValue('Keep this answer');
        await act(async () => fireEvent.click(screen.getByRole('button', {name: 'Answer'})));
        // With no structured questions, the summary stands in as the question.
        expect(respond).toHaveBeenLastCalledWith('question', true, '', undefined, {Bash: 'Keep this answer'});
        expect(drafts.read('questionDraft:question', (saved) => saved)).toMatchObject({status: 'accepted'});
    });

    const ask = (questions: unknown[]): TranscriptBlock => ({type: 'request', message: {
        content: 'Which database?', kind: 'ask_request', request_id: 'q', payload: {questions}, timestamp: '2026-10-04T00:00:00Z',
    }});
    const db = {question: 'Which database?', header: 'DB', options: [
        {label: 'Postgres (Recommended)', description: 'Matches production'}, {label: 'SQLite'},
    ]};

    it('answers a single choice with one click on an option, marking the suggested one', async () => {
        const respond = vi.fn().mockResolvedValue(true);
        show(respond, ask([db]));
        const postgres = screen.getByRole('radio', {name: /Postgres/});
        expect(within(postgres).getByText('Recommended')).toBeInTheDocument();
        expect(within(postgres).queryByText(/\(Recommended\)/)).toBeNull();
        expect(screen.getByText('Matches production')).toBeInTheDocument();
        await act(async () => fireEvent.click(postgres));
        expect(respond).toHaveBeenCalledWith('q', true, '', undefined, {'Which database?': 'Postgres (Recommended)'});
        expect(postgres).toHaveAttribute('aria-checked', 'true');
        expect(screen.getByRole('radio', {name: /SQLite/})).toBeDisabled();
    });

    it('waits for every question, joins several picks and lets typed words stand in', async () => {
        const respond = vi.fn().mockResolvedValue(true);
        show(respond, ask([db, {question: 'Which checks?', multiSelect: true, options: [{label: 'Lint'}, {label: 'Tests'}]}]));
        const send = screen.getByRole('button', {name: 'Answer'});
        fireEvent.click(screen.getByRole('radio', {name: /SQLite/}));
        expect(respond).not.toHaveBeenCalled();
        expect(send).toBeDisabled();
        fireEvent.click(screen.getByRole('checkbox', {name: /Lint/}));
        fireEvent.click(screen.getByRole('checkbox', {name: /Tests/}));
        const [own] = screen.getAllByRole('textbox', {name: 'Or answer in your own words'});
        fireEvent.change(own, {target: {value: 'DuckDB'}});
        expect(screen.getByRole('radio', {name: /SQLite/})).toHaveAttribute('aria-checked', 'false');
        await act(async () => fireEvent.click(send));
        expect(respond).toHaveBeenCalledWith('q', true, '', undefined, {'Which database?': 'DuckDB', 'Which checks?': 'Lint, Tests'});
    });

    it('declines with nothing chosen', async () => {
        const respond = vi.fn().mockResolvedValue(true);
        show(respond, ask([db, {question: 'Second?', options: [{label: 'A'}]}]));
        fireEvent.click(screen.getByRole('radio', {name: /SQLite/}));
        await act(async () => fireEvent.click(screen.getByRole('button', {name: 'Decline'})));
        expect(respond).toHaveBeenCalledWith('q', false, '', undefined, {});
    });

    it('does not carry a submitted decision into a new request in the same transcript slot', async () => {
        const respond = vi.fn().mockResolvedValue(true);
        const view = show(respond);
        await act(async () => fireEvent.click(screen.getByRole('button', {name: 'Allow'})));
        view.rerender(<Transcript blocks={[request('r2')]} pendingRequestId="r2" working onRespond={respond} drafts={drafts}/>);
        expect(screen.getByRole('button', {name: 'Allow'})).toBeEnabled();
    });
});


it('retains a pending and accepted decision when navigating away and back before the transcript updates', async () => {
    let finish!: (accepted: boolean) => void;
    const respond = vi.fn(() => new Promise<boolean>((resolve) => {finish = resolve;}));
    const first = show(respond);
    fireEvent.click(screen.getByRole('button', {name: 'Allow'}));
    first.unmount();
    show(respond);
    expect(screen.getByRole('button', {name: 'Allow'})).toBeDisabled();
    await act(async () => finish(true));
    expect(screen.getByRole('button', {name: 'Allow'})).toBeDisabled();
    expect(screen.getByRole('status')).toHaveTextContent('Response sent');
    expect(respond).toHaveBeenCalledOnce();
});

const msg = (m: Partial<TranscriptEntry>): TranscriptEntry => ({content: '', timestamp: '2026-10-04T00:00:00Z', ...m});
const respondNever = () => Promise.resolve(true);
const renderEntries = (entries: TranscriptEntry[], opts: {pending?: string; working?: boolean; expandAll?: boolean} = {}) =>
    render(<Transcript blocks={buildTranscript(entries)} pendingRequestId={opts.pending} working={opts.working ?? false} expandAll={opts.expandAll} onRespond={respondNever} drafts={drafts}/>);

describe('approval card', () => {
    it('shows the command, not the raw input, with the input behind Details', () => {
        renderEntries([
            msg({kind: 'tool_use', content: 'Bash', request_id: 't1', payload: {command: 'go test ./...', description: 'Run the tests'}}),
            msg({kind: 'approval_request', content: 'Bash', request_id: 'r1', payload: {command: 'go test ./...', description: 'Run the tests'}}),
        ], {pending: 'r1', working: true});
        const card = document.querySelector<HTMLElement>('[data-request-id="r1"]')!;
        expect(within(card).getByText('go test ./...')).toHaveAttribute('data-subject', 'command');
        expect(within(card).queryByText(/"description"/)).not.toBeInTheDocument();
        fireEvent.click(within(card).getByRole('button', {name: 'Details'}));
        expect(within(card).getByText(/"description": "Run the tests"/)).toBeInTheDocument();
        // The call it gates says nothing yet: the card is the call to action.
        expect(document.querySelector('[data-call-ids="t1"] [data-outcome]')).toBeNull();
    });

    it('names the file for an edit and the page for a fetch', () => {
        renderEntries([msg({kind: 'approval_request', content: 'Write', request_id: 'r1', payload: {file_path: '/repo/a.go', content: 'package a'}})], {pending: 'r1', working: true});
        expect(screen.getByText('/repo/a.go')).toHaveAttribute('data-subject', 'path');
        expect(screen.queryByText(/package a/)).not.toBeInTheDocument();
        cleanup();
        renderEntries([msg({kind: 'approval_request', content: 'WebFetch', request_id: 'r2', payload: {url: 'https://x.y/doc', prompt: 'read'}})], {pending: 'r2', working: true});
        expect(screen.getByText('https://x.y/doc')).toHaveAttribute('data-subject', 'url');
    });
});

describe('settled requests', () => {
    it('become a tag on the call they gated instead of a second line', () => {
        renderEntries([
            msg({kind: 'tool_use', content: 'Bash', request_id: 't1', payload: {command: 'make test'}}),
            msg({kind: 'approval_request', content: 'Bash', request_id: 'r1', payload: {command: 'make test'}}),
            msg({kind: 'approval_response', content: 'approved', request_id: 'r1'}),
            msg({kind: 'tool_result', content: 'ok', request_id: 't1'}),
            msg({kind: 'tool_use', content: 'WebFetch', request_id: 't2', payload: {url: 'https://x.y'}}),
            msg({kind: 'approval_request', content: 'WebFetch', request_id: 'r2', payload: {url: 'https://x.y'}}),
            msg({kind: 'approval_response', content: 'denied', request_id: 'r2'}),
            msg({kind: 'tool_result', content: 'denied by user', request_id: 't2', payload: {is_error: true}}),
            msg({kind: 'tool_use', content: 'Edit', request_id: 't3', payload: {file_path: 'a.go'}}),
            msg({kind: 'approval_request', content: 'Edit', request_id: 'r3', payload: {file_path: 'a.go'}}),
        ]);
        expect(screen.getAllByText('make test')).toHaveLength(1);
        const allowed = document.querySelector<HTMLElement>('[data-call-ids="t1"]')!;
        expect(allowed).toHaveAttribute('data-request-id', 'r1');
        expect(within(allowed).getByText('Allowed')).toBeInTheDocument();
        const denied = document.querySelector<HTMLElement>('[data-call-ids="t2"]')!;
        expect(within(denied).getByText('Denied')).toBeInTheDocument();
        // The denial says why it failed; it isn't marked failed on top.
        expect(within(denied).queryByText('failed')).not.toBeInTheDocument();
        // The turn ended without an answer.
        expect(within(document.querySelector<HTMLElement>('[data-call-ids="t3"]')!).getByText('Not answered')).toBeInTheDocument();
    });

    it('stay one quiet line when no call is theirs, and a question reads with its answer', () => {
        renderEntries([
            msg({kind: 'approval_request', content: 'Bash', request_id: 'r1', payload: {command: 'rm -rf dist'}}),
            msg({kind: 'approval_response', content: 'denied', request_id: 'r1'}),
            msg({kind: 'ask_request', content: 'Which branch?', request_id: 'q1'}),
            msg({kind: 'ask_response', content: 'main', request_id: 'q1'}),
        ]);
        const line = document.querySelector<HTMLElement>('[data-request-id="r1"]')!;
        expect(within(line).getByText('rm -rf dist')).toBeInTheDocument();
        expect(within(line).getByText('Denied')).toBeInTheDocument();
        expect(screen.queryByRole('button', {name: 'Allow'})).not.toBeInTheDocument();
        const question = document.querySelector<HTMLElement>('[data-request-id="q1"]')!;
        expect(question).toHaveTextContent('Which branch?');
        expect(question).toHaveTextContent('↳ main');
    });
});

describe('tool calls', () => {
    const run = [
        msg({role: 'user', content: 'fix it'}),
        msg({kind: 'thinking', content: 'look at main.go'}),
        msg({kind: 'thinking', content: 'then run the tests'}),
        msg({kind: 'tool_use', content: 'Read', request_id: 't1', payload: {file_path: 'main.go'}}),
        msg({kind: 'tool_use', content: 'Bash', request_id: 't2', payload: {command: 'go test ./...'}}),
        msg({kind: 'tool_result', content: 'package main', request_id: 't1'}),
        msg({kind: 'tool_result', content: 'FAIL main_test.go:12', request_id: 't2', payload: {is_error: true}}),
        msg({role: 'assistant', content: 'Found it.'}),
    ];

    it('get one line each, with thinking folded into one Thought line', () => {
        renderEntries(run);
        expect(screen.getAllByText('Thought')).toHaveLength(1);
        expect(screen.queryByText(/Used \d/)).not.toBeInTheDocument();
        const read = document.querySelector<HTMLElement>('[data-call-ids="t1"]')!;
        expect(read).toHaveTextContent('Readmain.go');
        const bash = document.querySelector<HTMLElement>('[data-call-ids="t2"]')!;
        expect(within(bash).getByText('failed')).toBeInTheDocument();
        expect(within(read).queryByText('failed')).not.toBeInTheDocument();
        expect(screen.queryByText('package main')).not.toBeInTheDocument();
    });

    it('open to their input and output one at a time', () => {
        renderEntries(run);
        const bash = document.querySelector<HTMLElement>('[data-call-ids="t2"]')!;
        fireEvent.click(within(bash).getByRole('button'));
        expect(within(bash).getByText(/"command": "go test/)).toBeInTheDocument();
        expect(within(bash).getByText('FAIL main_test.go:12')).toBeInTheDocument();
        expect(screen.queryByText('package main')).not.toBeInTheDocument();
        expect(screen.queryByText('look at main.go')).not.toBeInTheDocument();
    });

    it('all open with expand all, and each still closes on its own', async () => {
        renderEntries(run, {expandAll: true});
        expect(screen.getByText('package main')).toBeInTheDocument();
        expect(screen.getByText('FAIL main_test.go:12')).toBeInTheDocument();
        expect(screen.getByText('look at main.go')).toBeInTheDocument();
        expect(screen.getByText('then run the tests')).toBeInTheDocument();
        fireEvent.click(within(document.querySelector<HTMLElement>('[data-call-ids="t1"]')!).getAllByRole('button')[0]);
        await waitFor(() => expect(screen.queryByText('package main')).not.toBeInTheDocument());
        expect(screen.getByText('FAIL main_test.go:12')).toBeInTheDocument();
    });

    it('carry the live spinner on the last line while the turn runs', () => {
        renderEntries(run.slice(0, 5), {working: true});
        const bash = document.querySelector<HTMLElement>('[data-call-ids="t2"]')!;
        expect(bash.querySelector('.MuiCircularProgress-root')).not.toBeNull();
        expect(document.querySelector('[data-call-ids="t1"] .MuiCircularProgress-root')).toBeNull();
        expect(screen.queryByText('Working…')).not.toBeInTheDocument();
    });
});

describe('host extensions', () => {
    it('offers to remember an approval only when the host labels it', async () => {
        const respond = vi.fn().mockResolvedValue(true);
        const view = show(respond);
        expect(screen.getAllByRole('button')).toHaveLength(3); // Allow, Details, Deny
        view.unmount();
        render(<Transcript blocks={[request('r3')]} pendingRequestId="r3" working onRespond={respond} drafts={drafts} rememberLabel={(tool) => `Always allow ${tool}`}/>);
        await act(async () => fireEvent.click(screen.getByRole('button', {name: 'Always allow Bash'})));
        expect(respond).toHaveBeenLastCalledWith('r3', true, '', true);
    });

    it('names who sent a message when the host knows', () => {
        const message = (author?: string): TranscriptBlock => ({type: 'user', message: {role: 'user', content: 'Do it', author, timestamp: '2026-10-04T00:00:00Z'}});
        render(<Transcript blocks={[message('Coordinator'), message()]} working={false} onRespond={vi.fn()} drafts={drafts}/>);
        expect(screen.getAllByText('Coordinator')).toHaveLength(1);
        expect(screen.getAllByText('Do it')).toHaveLength(2);
    });
});
