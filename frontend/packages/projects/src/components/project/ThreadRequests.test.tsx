import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api, type Thread, type ThreadEvent } from '../../client';
import { HostNotifyProvider } from '../../hooks/useNotify';
import { ThreadRequestsProvider, pendingRequestOf, useThreadRequestsStore, type ThreadRequests } from '../../hooks/threadRequests';
import { CLEARED_CLOSE_MS, ThreadRequestSubtitle, ThreadRequestsStrip } from './ThreadRequests';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, o?: { defaultValue?: string; count?: number; tool?: string; title?: string }) =>
            (o?.defaultValue ?? key).replace('{{tool}}', o?.tool ?? '').replace('{{count}}', String(o?.count ?? '')).replace('{{title}}', o?.title ?? ''),
        i18n: { language: 'en' },
    }),
}));

afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
});

const thread = (o: Partial<Thread> = {}): Thread => ({ id: 't1', project_id: 'p1', title: 'Fix login', status: 'waiting', wait_reason: 'Approve: Bash', running_tasks: 0, ...o } as Thread);

const approval = (id: string, command: string, state = 'pending', thread_id = 't1'): ThreadEvent => ({
    id: `e-${id}`, thread_id, kind: 'approval', content: command,
    payload: JSON.stringify({ request_id: id, tool: 'Bash', input: { command }, state }),
    created_at: `2026-11-13T10:00:0${id.length}Z`,
} as ThreadEvent);

const question = (id: string): ThreadEvent => ({
    id: `e-${id}`, thread_id: 't1', kind: 'ask', content: 'Which database?',
    payload: JSON.stringify({ request_id: id, tool: 'AskUserQuestion', state: 'pending', input: { questions: [{ question: 'Which database?', header: 'DB', multiSelect: false, options: [{ label: 'sqlite', description: 'file' }, { label: 'postgres', description: 'server' }] }] } }),
    created_at: '2026-11-13T10:00:00Z',
} as ThreadEvent);

const ok = (data: unknown) => ({ data, response: new Response('{}', { status: 200 }) });
const refused = (status: number, error: string) => ({ error: { error }, response: new Response('{}', { status }) });

let store: ThreadRequests;
const Probe = ({ threads, onOpen }: { threads: Thread[]; onOpen: (id: string) => void }) => {
    store = useThreadRequestsStore(threads);
    return (
        <ThreadRequestsProvider value={store}>
            {threads.map((th) => (
                <div key={th.id} data-testid={`card-${th.id}`} onClick={() => onOpen(th.id)}>
                    <div data-testid="subtitle"><ThreadRequestSubtitle thread={th} /></div>
                    <ThreadRequestsStrip thread={th} onOpen={onOpen} />
                </div>
            ))}
        </ThreadRequestsProvider>
    );
};

const notify = vi.fn();
const show = (requests: ThreadEvent[], threads: Thread[] = [thread()], onOpen: (id: string) => void = vi.fn()) => {
    const get = vi.spyOn(api, 'GET').mockResolvedValue(ok({ requests }) as never);
    const post = vi.spyOn(api, 'POST');
    render(<HostNotifyProvider notify={notify}><Probe threads={threads} onOpen={onOpen} /></HostNotifyProvider>);
    return { get, post, onOpen };
};
const strip = () => screen.findByRole('button', { name: /thread\.requests\./ });

beforeEach(() => notify.mockReset());

describe('pendingRequestOf', () => {
    it('reads a pending approval or question and nothing that is settled', () => {
        const a = pendingRequestOf(approval('r1', 'npm test'));
        expect(a).toMatchObject({ id: 'r1', kind: 'approval', entry: { kind: 'approval_request', content: 'Bash' } });
        expect(pendingRequestOf(question('q1'))).toMatchObject({ id: 'q1', kind: 'ask' });
        expect(pendingRequestOf(approval('r1', 'npm test', 'allowed'))).toBeNull();
        expect(pendingRequestOf(approval('r1', 'npm test', 'expired'))).toBeNull();
        expect(pendingRequestOf({ ...approval('r1', 'x'), kind: 'message' } as ThreadEvent)).toBeNull();
        expect(pendingRequestOf({ ...approval('r1', 'x'), payload: 'not json' })).toBeNull();
    });
});

describe('a waiting thread card', () => {
    it('shows what is asked and offers to review it', async () => {
        const { get } = show([approval('r1', 'npm test')]);
        const open = await strip();
        expect(get).toHaveBeenCalledWith('/api/v1/threads/{id}/requests', { params: { path: { id: 't1' } } });
        expect(screen.getByTestId('subtitle')).toHaveTextContent('Bash');
        expect(screen.getByTestId('subtitle')).toHaveTextContent('npm test');
        expect(open).toHaveAttribute('aria-haspopup', 'dialog');
        expect(open).toHaveAttribute('aria-expanded', 'false');
        expect(open).toHaveAccessibleName('thread.requests.approval · thread.requests.review');
    });

    it('keeps its usual subtitle and no row when nothing answerable is pending', async () => {
        const { get } = show([]);
        await waitFor(() => expect(get).toHaveBeenCalled());
        expect(screen.queryByRole('button')).toBeNull();
    });

    it('does not ask for threads that do not wait on the user', async () => {
        const { get } = show([approval('r1', 'npm test')], [thread({ status: 'working' }), thread({ id: 't2', status: 'failed' })]);
        await Promise.resolve();
        expect(get).not.toHaveBeenCalled();
        expect(screen.queryByRole('button')).toBeNull();
    });

    it('lists several requests oldest first and counts them', async () => {
        show([approval('r1', 'npm test'), approval('r22', 'rm -rf build')]);
        const open = await strip();
        expect(open).toHaveAccessibleName(/thread\.requests\.many/);
        expect(screen.getByTestId('subtitle')).toHaveTextContent('npm test');
        expect(screen.getByTestId('subtitle')).toHaveTextContent('thread.requests.more');
        fireEvent.click(open);
        const dialog = await screen.findByRole('dialog');
        const commands = within(dialog).getAllByText(/npm test|rm -rf build/).map((n) => n.textContent);
        expect(commands.findIndex((c) => c?.includes('npm test'))).toBeLessThan(commands.findIndex((c) => c?.includes('rm -rf build')));
        expect(within(dialog).getAllByRole('button', { name: 'Allow' })).toHaveLength(2);
    });

    it('opening the review does not open the thread; the link inside does', async () => {
        const { onOpen } = show([approval('r1', 'npm test')]);
        fireEvent.click(await strip());
        const dialog = await screen.findByRole('dialog');
        expect(within(dialog).getByText('npm test')).toBeInTheDocument();
        fireEvent.click(within(dialog).getByRole('button', { name: 'Allow' }).parentElement!);
        expect(onOpen).not.toHaveBeenCalled();
        fireEvent.click(within(dialog).getByRole('button', { name: /thread\.requests\.openThread/ }));
        expect(onOpen).toHaveBeenCalledTimes(1);
        expect(onOpen).toHaveBeenCalledWith('t1');
    });

    it('allows a request from the card and sends the same answer the thread page does', async () => {
        const { post } = show([approval('ra', 'npm test')]);
        post.mockResolvedValue(ok(thread({ status: 'working' })) as never);
        fireEvent.click(await strip());
        const dialog = await screen.findByRole('dialog');
        fireEvent.click(within(dialog).getByRole('button', { name: 'Allow' }));
        await waitFor(() => expect(post).toHaveBeenCalledWith('/api/v1/threads/{id}/respond', {
            params: { path: { id: 't1' } }, body: { request_id: 'ra', allow: true, scope: 'once', answer: undefined, answers: undefined },
        }));
        expect(await within(dialog).findByRole('status')).toHaveTextContent('Response sent');
    });

    it('denies from the card', async () => {
        const { post } = show([approval('rd', 'npm test')]);
        post.mockResolvedValue(ok(thread({ status: 'working' })) as never);
        fireEvent.click(await strip());
        fireEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Deny' }));
        await waitFor(() => expect(post).toHaveBeenCalledWith('/api/v1/threads/{id}/respond', expect.objectContaining({ body: expect.objectContaining({ request_id: 'rd', allow: false }) })));
    });

    it('answers a question with its option', async () => {
        const { post } = show([question('qa')]);
        post.mockResolvedValue(ok(thread({ status: 'working' })) as never);
        expect(await strip()).toHaveAccessibleName(/thread\.requests\.question/);
        expect(screen.getByTestId('subtitle')).toHaveTextContent('Which database?');
        fireEvent.click(await strip());
        fireEvent.click(within(await screen.findByRole('dialog')).getByRole('radio', { name: /postgres/ }));
        await waitFor(() => expect(post).toHaveBeenCalledWith('/api/v1/threads/{id}/respond', expect.objectContaining({
            body: expect.objectContaining({ request_id: 'qa', allow: true, answers: { 'Which database?': 'postgres' } }),
        })));
    });

    it('follows an answer given elsewhere: the card leaves the popover and says nothing is waiting', async () => {
        show([approval('rx', 'npm test')]);
        fireEvent.click(await strip());
        const dialog = await screen.findByRole('dialog');
        act(() => store.observe(approval('rx', 'npm test', 'allowed')));
        expect(await within(dialog).findByText('thread.requests.cleared')).toBeInTheDocument();
        expect(within(dialog).queryByRole('button', { name: 'Allow' })).toBeNull();
        await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull(), { timeout: CLEARED_CLOSE_MS + 1500 });
    });

    it('a request answered elsewhere but still shown is told so when answered here (409)', async () => {
        const { post, get } = show([approval('r409', 'npm test')]);
        post.mockResolvedValue(refused(409, 'no pending request with that id') as never);
        fireEvent.click(await strip());
        const dialog = await screen.findByRole('dialog');
        get.mockResolvedValue(ok({ requests: [] }) as never);
        fireEvent.click(within(dialog).getByRole('button', { name: 'Allow' }));
        expect(await within(dialog).findByText('thread.requests.settled')).toBeInTheDocument();
        expect(within(dialog).queryByRole('button', { name: 'Allow' })).toBeNull();
        expect(notify).not.toHaveBeenCalled();
    });

    it('reports a refused answer and keeps the request to try again', async () => {
        const { post } = show([approval('r500', 'npm test')]);
        post.mockResolvedValue(refused(500, 'agent unreachable') as never);
        fireEvent.click(await strip());
        const dialog = await screen.findByRole('dialog');
        fireEvent.click(within(dialog).getByRole('button', { name: 'Allow' }));
        await waitFor(() => expect(notify).toHaveBeenCalledWith('agent unreachable', 'error'));
        expect(await within(dialog).findByText(/Response was not accepted/)).toBeInTheDocument();
        expect(within(dialog).getByRole('button', { name: 'Allow' })).toBeEnabled();
    });

    it('picks up a request raised after the thread started waiting', async () => {
        show([approval('r1', 'npm test')]);
        await strip();
        act(() => store.observe(approval('r22', 'make deploy')));
        expect(await screen.findByRole('button', { name: /thread\.requests\.many/ })).toBeInTheDocument();
    });

    it('drops a thread\'s requests once it stops waiting', async () => {
        const get = vi.spyOn(api, 'GET').mockResolvedValue(ok({ requests: [approval('r1', 'npm test')] }) as never);
        const onOpen = vi.fn();
        const { rerender } = render(<Probe threads={[thread()]} onOpen={onOpen} />);
        await strip();
        rerender(<Probe threads={[thread({ status: 'working' })]} onOpen={onOpen} />);
        await waitFor(() => expect(screen.queryByRole('button')).toBeNull());
        expect(get).toHaveBeenCalledTimes(1);
    });

    it('opens the requests of the thread a notification points at', async () => {
        show([approval('r1', 'npm test')]);
        await strip();
        act(() => store.review('t1'));
        expect(await screen.findByRole('dialog')).toBeInTheDocument();
        expect(store.reviewId).toBeNull();
    });

    it('reads again after the stream reconnects', async () => {
        const { get } = show([approval('r1', 'npm test')]);
        await strip();
        act(() => store.refresh());
        await waitFor(() => expect(get).toHaveBeenCalledTimes(2));
    });
});
