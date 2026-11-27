import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { ChannelBinding } from '../../client/channelBindings';
import { ProjectsHostProvider, type ImChat, type ProjectsHost } from '../../host/ProjectsHost';
import { ProjectImTags, useProjectIm } from './ProjectImBindings';

const m = vi.hoisted(() => ({ list: vi.fn(), bind: vi.fn(), unbind: vi.fn(), get: vi.fn(), notify: vi.fn() }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string, o?: Record<string, unknown>) => (o ? `${key} ${JSON.stringify(o)}` : key) }) }));
vi.mock('../../client/channelBindings', () => ({ listChannelBindings: m.list, bindChannel: m.bind, unbindChannel: m.unbind }));
vi.mock('../../client', async (actual) => ({ ...(await actual<typeof import('../../client')>()), api: { GET: m.get } }));

// A project's chats are bound to its main thread, named m<project>.
const binding = (id: string, chat_id: string, chat_title: string, project_id: string): ChannelBinding => ({
    id, platform: 'telegram', bot_uuid: 'b1', chat_id, chat_title, project_id, thread_id: `m${project_id}`, created_at: '', updated_at: '', verbose: false,
});

const summary = (id: string, name: string, over: Record<string, unknown> = {}) => ({ id, name, type: 'project', main_thread_id: `m${id}`, threads: { waiting: 0, working: 0, ready: 0, idle: 0, resolved: 0 }, ...over });

const chat = (chatId: string, title: string, over: Partial<ImChat> = {}): ImChat => ({
    botUuid: 'b1', platform: 'telegram', chatId, alias: title, title, kind: 'direct', state: 'ok', ...over,
});

const host = (chats: ImChat[], bots = [{ uuid: 'b1', name: 'Bot', platform: 'telegram', online: true }]): ProjectsHost => ({
    paths: { list: '/p', project: (id) => `/p/${id}`, folder: () => '/f', im: '/im' },
    notify: m.notify,
    im: { listBots: async () => bots, listChats: async () => chats },
});

function Harness({ folder = false }: { folder?: boolean }) {
    const im = useProjectIm(folder ? { projectId: 'f1', name: 'Folder', folder: true } : { threadId: 'mp1', projectId: 'p1', name: 'Alpha' });
    if (!im) return <div>no im</div>;
    return <><ProjectImTags im={im} />{im.dialog}</>;
}

const show = (h: ProjectsHost, folder = false) => render(
    <MemoryRouter><ProjectsHostProvider host={h}><Harness folder={folder} /></ProjectsHostProvider></MemoryRouter>,
);

beforeEach(() => {
    m.list.mockResolvedValue([
        binding('cb1', 'telegram:1', 'yz', 'p1'),
        binding('cb2', 'telegram:2', 'Team', 'p2'),
    ]);
    m.bind.mockImplementation(async (r: { chat_id: string }) => binding('new', r.chat_id, 'x', 'p1'));
    m.unbind.mockResolvedValue(undefined);
    m.get.mockResolvedValue({ data: { projects: [summary('p1', 'Alpha'), summary('p2', 'Docs')] }, response: new Response() });
});
afterEach(() => {
    cleanup();
    for (const f of Object.values(m)) f.mockReset();
});

describe('project IM bindings', () => {
    it('keeps a project binding visible when the chat focuses a project thread', async () => {
        m.list.mockResolvedValue([{ ...binding('focused', 'telegram:1', 'yz', 'p1'), thread_id: 'worker-1', target_project_id: 'p1' }]);
        show(host([chat('telegram:1', 'yz')]));
        expect(await screen.findByText('yz')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: /im\.tags\.bind/ }));
        expect(await screen.findByText('im.bind.noChatsTitle')).toBeInTheDocument();
    });
    it('allows moving a chat from a specific session to its folder', async () => {
        m.list.mockResolvedValue([{ ...binding('session', 'telegram:1', 'yz', 'f1'), thread_id: 'session1', target_project_id: 'f1', target_thread_id: 'session1' }]);
        show(host([chat('telegram:1', 'yz')]), true);
        fireEvent.click(await screen.findByRole('button', { name: /im\.tags\.bind/ }));
        fireEvent.click(await screen.findByRole('radio', { name: /yz/ }));
        fireEvent.click(screen.getByRole('button', { name: 'im.bind.submit' }));
        await waitFor(() => expect(m.bind).toHaveBeenCalledWith({ platform: 'telegram', chat_id: 'telegram:1', bot_uuid: 'b1', chat_title: expect.any(String), project_id: 'f1' }));
    });
    it('binds a chat to a folder before a session exists and shows pending bindings', async () => {
        m.list.mockResolvedValue([{ ...binding('pending', 'telegram:1', 'yz', 'f1'), thread_id: '', folder: '/work' }]);
        show(host([chat('telegram:1', 'yz'), chat('telegram:3', 'Free')]), true);
        expect(await screen.findByText('yz')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: /im\.tags\.bind/ }));
        fireEvent.click(await screen.findByRole('radio', { name: /Free/ }));
        fireEvent.click(screen.getByRole('button', { name: 'im.bind.submit' }));
        await waitFor(() => expect(m.bind).toHaveBeenCalledWith({ platform: 'telegram', chat_id: 'telegram:3', bot_uuid: 'b1', chat_title: expect.any(String), project_id: 'f1' }));
    });
    it('shows a bound chat by its alias and id, and marks one the host no longer has as deleted', async () => {
        m.list.mockResolvedValue([binding('cb1', 'telegram:1', 'yz', 'p1'), binding('cb2', 'telegram:2', 'Gone chat', 'p1')]);
        const h = host([]);
        h.im!.chatInfo = (_platform, chatId) => (chatId === 'telegram:2'
            ? { displayId: '2', group: false, deleted: true }
            : { alias: 'Alice (ops)', displayId: '1', group: false });
        show(h);
        const labels = await screen.findAllByTestId('entity-label');
        // Tags are compact: the alias gets the room, the full label (id included) is the tooltip.
        expect(labels[0]).toHaveTextContent('Alice (ops)');
        expect(labels[0]).toHaveAttribute('title', 'Telegram · Alice (ops) · 1');
        expect(labels[0]).not.toHaveTextContent('im.deleted');
        expect(labels[1]).toHaveTextContent('im.deleted');
        expect(labels[1]).toHaveTextContent('Gone chat');
    });
    it('lets the host\'s answer win over a stale saved title, and hides a title that is just the id', async () => {
        m.list.mockResolvedValue([binding('cb1', 'telegram:1', 'Old name', 'p1'), binding('cb2', 'telegram:2', '2', 'p1')]);
        const h = host([]);
        // The host knows chat 1 and says it has no alias now (cleared); chat 2 is unknown to it.
        h.im!.chatInfo = (_p, chatId) => (chatId === 'telegram:1' ? { displayId: '1', group: false } : undefined);
        show(h);
        const labels = await screen.findAllByTestId('entity-label');
        expect(labels[0]).toHaveAttribute('title', 'Telegram · 1');
        expect(labels[1]).toHaveAttribute('title', 'Telegram · 2');
    });
    it('shows only this project\'s chats and unbinds one with its ×', async () => {
        show(host([]));
        expect(await screen.findByText('yz')).toBeInTheDocument();
        expect(screen.queryByText('Team')).not.toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: /im\.tags\.unbind/ }));
        await waitFor(() => expect(m.unbind).toHaveBeenCalledWith('cb1'));
        expect(m.notify).toHaveBeenCalledWith(expect.stringContaining('im.bind.unbound'), 'success');
    });

    it('shows nothing when the host has no IM', () => {
        render(<MemoryRouter><ProjectsHostProvider host={{ paths: { list: '/p', project: (id) => `/p/${id}`, folder: () => '/f' } }}><Harness /></ProjectsHostProvider></MemoryRouter>);
        expect(screen.getByText('no im')).toBeInTheDocument();
        expect(m.list).not.toHaveBeenCalled();
    });

    it('binds a free chat, marks a chat bound elsewhere "will move", and leaves out chats already here', async () => {
        show(host([chat('telegram:1', 'yz'), chat('telegram:2', 'Team', { kind: 'group' }), chat('telegram:3', 'Free'), chat('telegram:4', 'Off', { state: 'blocked' })]));
        fireEvent.click(await screen.findByRole('button', { name: /im\.tags\.bind/ }));
        const dialog = await screen.findByRole('dialog');
        const radios = await within(dialog).findAllByRole('radio');
        // telegram:1 is bound here (in the header), so: Free, Off (disabled), then Team (bound elsewhere) last among enabled free-first order.
        expect(radios.map((r) => r.textContent)).toEqual([
            expect.stringContaining('Free'),
            expect.stringContaining('Off'),
            expect.stringContaining('Team'),
        ]);
        expect(radios[2].textContent).toContain('im.bind.rebind');
        expect(radios[2].textContent).toContain('Docs');
        expect(radios[1]).toBeDisabled();
        const submit = within(dialog).getByRole('button', { name: 'im.bind.submit' });
        expect(submit).toBeDisabled();
        fireEvent.click(radios[2]);
        fireEvent.click(submit);
        await waitFor(() => expect(m.bind).toHaveBeenCalledWith({ platform: 'telegram', chat_id: 'telegram:2', bot_uuid: 'b1', chat_title: expect.any(String), project_id: 'p1' }));
        expect(m.notify).toHaveBeenCalledWith(expect.stringContaining('im.bind.moved'), 'success');
    });

    it('allows selecting another bot for the same chat and thread', async () => {
        const bots = [{ uuid: 'b1', name: 'First bot', platform: 'telegram', online: true }, { uuid: 'b2', name: 'Second bot', platform: 'telegram', online: true }];
        const h = host([], bots);
        h.im!.listChats = async (bot) => [chat('telegram:1', 'yz', { botUuid: bot.uuid })];
        show(h);
        fireEvent.click(await screen.findByRole('button', { name: /im\.tags\.bind/ }));
        const dialog = await screen.findByRole('dialog');
        const other = await within(dialog).findByRole('radio', { name: /Second bot/ });
        fireEvent.click(other);
        fireEvent.click(within(dialog).getByRole('button', { name: 'im.bind.submit' }));
        await waitFor(() => expect(m.bind).toHaveBeenCalledWith({ platform: 'telegram', chat_id: 'telegram:1', bot_uuid: 'b2', chat_title: expect.any(String), project_id: 'p1' }));
    });

    it('says why there is nothing to bind', async () => {
        show(host([], []));
        fireEvent.click(await screen.findByRole('button', { name: /im\.tags\.bind/ }));
        expect(await screen.findByText('im.bind.noBotsTitle')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'im.bind.openSettings' })).toBeInTheDocument();
    });

    it('hides ids that cannot receive a message and keeps an unreadable one visible but disabled', async () => {
        show(host([
            chat('oc_group', 'Feishu group', { platform: 'feishu', kind: 'group' }),
            chat('od-4e6ac4d14bcd5071a37a39de902c7141', 'Department', { platform: 'feishu' }),
            chat('U012ABCDEF', 'A user', { platform: 'slack' }),
        ]));
        fireEvent.click(await screen.findByRole('button', { name: /im\.tags\.bind/ }));
        const dialog = await screen.findByRole('dialog');
        const radios = await within(dialog).findAllByRole('radio');
        // The department is not offered at all; the Slack user id stays, disabled, with the reason.
        expect(radios).toHaveLength(2);
        expect(radios[0].textContent).toContain('oc_group');
        expect(radios[1].textContent).toContain('U012ABCDEF');
        expect(radios[1].textContent).toContain('im.idReason.userNotChat');
        expect(radios[1]).toBeDisabled();
        expect(within(dialog).queryByText(/od-4e6ac4d14bcd5071a37a39de902c7141/)).not.toBeInTheDocument();
        expect(within(dialog).getByText(/im\.bind\.hidden/)).toBeInTheDocument();
    });

    it('says when every known entry is not a message target', async () => {
        show(host([
            chat('od-4e6ac4d14bcd5071a37a39de902c7141', 'Department', { platform: 'feishu' }),
            chat('cli_a1b2c3', 'App', { platform: 'feishu' }),
        ]));
        fireEvent.click(await screen.findByRole('button', { name: /im\.tags\.bind/ }));
        expect(await screen.findByText('im.bind.noSendableTitle')).toBeInTheDocument();
        expect(screen.getByText('im.bind.noSendableBody')).toBeInTheDocument();
        expect(screen.queryByRole('radio')).not.toBeInTheDocument();
    });

    it('keeps the dialog open and reports a failed bind', async () => {
        m.bind.mockRejectedValue(new Error('boom'));
        show(host([chat('telegram:3', 'Free')]));
        fireEvent.click(await screen.findByRole('button', { name: /im\.tags\.bind/ }));
        fireEvent.click(await screen.findByRole('radio'));
        fireEvent.click(screen.getByRole('button', { name: 'im.bind.submit' }));
        await waitFor(() => expect(m.notify).toHaveBeenCalledWith(expect.stringContaining('boom'), 'error'));
        expect(screen.getByRole('dialog')).toBeInTheDocument();
    });
});
