import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { ChannelBinding } from '../client/channelBindings';
import { ProjectsHostProvider, type ImChat, type ProjectsHost } from '../host/ProjectsHost';
import ImSettingsPage from './ImSettingsPage';

const m = vi.hoisted(() => ({ list: vi.fn(), bind: vi.fn(), unbind: vi.fn(), get: vi.fn(), notify: vi.fn() }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string, o?: Record<string, unknown>) => (o ? `${key} ${JSON.stringify(o)}` : key) }) }));
vi.mock('../client/channelBindings', () => ({ listChannelBindings: m.list, bindChannel: m.bind, unbindChannel: m.unbind }));
vi.mock('../client', async (actual) => ({ ...(await actual<typeof import('../client')>()), api: { GET: m.get } }));

const chat = (chatId: string, title: string, over: Partial<ImChat> = {}): ImChat => ({
    botUuid: 'b1', platform: 'telegram', chatId, title, kind: 'direct', state: 'ok', ...over,
});

const host = (over: Partial<NonNullable<ProjectsHost['im']>> = {}): ProjectsHost => ({
    paths: { list: '/p', project: (id) => `/p/${id}`, folder: () => '/f', im: '/im' },
    notify: m.notify,
    im: {
        listBots: async () => [{ uuid: 'b1', name: 'My bot', platform: 'telegram', online: true }],
        listChats: async () => [chat('tg:1', 'yz'), chat('tg:2', 'New chat'), chat('tg:3', 'Muted', { state: 'blocked' })],
        ...over,
    },
});

const summary = (id: string, name: string, over: Record<string, unknown> = {}) => ({ id, name, type: 'project', main_thread_id: `m${id}`, threads: { waiting: 0, working: 0, ready: 0, idle: 0, resolved: 0 }, ...over });

const show = (h: ProjectsHost) => render(<MemoryRouter><ProjectsHostProvider host={h}><ImSettingsPage /></ProjectsHostProvider></MemoryRouter>);

beforeEach(() => {
    m.list.mockResolvedValue([{ id: 'cb1', platform: 'telegram', chat_id: 'tg:1', chat_title: 'yz', project_id: 'p1', thread_id: 'mp1', created_at: '', updated_at: '', verbose: false } satisfies ChannelBinding]);
    m.bind.mockResolvedValue({});
    m.get.mockResolvedValue({ data: { projects: [summary('p1', 'Alpha'), summary('p2', 'Docs'), summary('f1', 'notes', { type: 'folder', main_thread_id: undefined, path: '/home/me/notes' })] }, response: new Response() });
});
afterEach(() => {
    cleanup();
    for (const f of Object.values(m)) f.mockReset();
});

describe('ImSettingsPage', () => {
    it('lists each bot\'s chats with where they are bound and offers Move or Bind', async () => {
        show(host());
        expect(await screen.findByText('My bot')).toBeInTheDocument();
        const rows = await screen.findAllByRole('row');
        // header + three chats
        expect(rows).toHaveLength(4);
        expect(within(rows[1]).getByRole('link', { name: 'Alpha' })).toHaveAttribute('href', '/p/p1');
        expect(within(rows[1]).getByRole('button', { name: /im\.settings\.rebindLabel/ })).toBeEnabled();
        expect(within(rows[2]).getByText('im.settings.notBound')).toBeInTheDocument();
        expect(within(rows[2]).getByRole('button', { name: 'im.settings.bindAction' })).toBeEnabled();
        expect(within(rows[3]).getByRole('button', { name: 'im.settings.bindAction' })).toBeDisabled();
    });

    it('binds an unbound chat to the project picked', async () => {
        show(host());
        const rows = await screen.findAllByRole('row');
        fireEvent.click(within(rows[2]).getByRole('button', { name: 'im.settings.bindAction' }));
        const dialog = await screen.findByRole('dialog');
        fireEvent.click(await within(dialog).findByRole('radio', { name: /Docs/ }));
        fireEvent.click(within(dialog).getByRole('button', { name: 'im.bind.submit' }));
        await waitFor(() => expect(m.bind).toHaveBeenCalledWith({ platform: 'telegram', chat_id: 'tg:2', project_id: 'p2' }));
    });

    it('does not offer the project a chat is already bound to, nor a folder (those are bound by thread)', async () => {
        show(host());
        const rows = await screen.findAllByRole('row');
        fireEvent.click(within(rows[1]).getByRole('button', { name: /im\.settings\.rebindLabel/ }));
        const dialog = await screen.findByRole('dialog');
        const radios = await within(dialog).findAllByRole('radio');
        expect(radios.map((r) => r.textContent)).toEqual(['Docs']);
    });

    it('opens the host\'s access dialog for a bot', async () => {
        show(host({ accessDialog: ({ bot, onClose }) => <button onClick={onClose}>access:{bot.name}</button> }));
        fireEvent.click(await screen.findByRole('button', { name: 'im.settings.access' }));
        expect(screen.getByText('access:My bot')).toBeInTheDocument();
    });

    it('explains when there are no bots', async () => {
        show(host({ listBots: async () => [] }));
        expect(await screen.findByText('im.settings.emptyTitle')).toBeInTheDocument();
    });
});
