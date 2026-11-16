import {fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import type {BotSettings} from '@/types/bot';
import NotifyGuide from './NotifyGuide';
import {SAMPLE_MESSAGE, buildSample} from './notifySamples';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, o?: Record<string, unknown>) => {
            const text = typeof o?.defaultValue === 'string' ? o.defaultValue : key;
            return text.replace(/\{\{(\w+)\}\}/g, (_m, name) => String(o?.[name] ?? ''));
        },
    }),
}));

vi.mock('@/constants/platformGuides', () => ({
    PLATFORM_BRAND_ICONS: {},
    platformDisplayName: (id: string) => id.charAt(0).toUpperCase() + id.slice(1),
}));

vi.mock('@/utils/protocol', () => ({getApiBaseUrl: () => Promise.resolve('http://gw.test')}));

const apiMock = vi.hoisted(() => ({
    listBotDirectChats: vi.fn(),
    listBotGroups: vi.fn(),
    getBotGroup: vi.fn(),
    notifyBot: vi.fn(),
}));
vi.mock('@/services/api', () => ({api: apiMock}));

const allow = [
    {capability: 'notify', action: 'notify.receive', effect: 'allow'},
    {capability: 'notify', action: 'notify.reply', effect: 'allow'},
];

const bot = (over: Partial<BotSettings> = {}): BotSettings => ({
    uuid: 'bot-1',
    name: 'Ops bot',
    platform: 'telegram',
    enabled: true,
    capabilities: [{bot_uuid: 'bot-1', capability: 'notify', enabled: true}],
    ...over,
} as BotSettings);

const chat = (id: string, external: string, alias: string | undefined, permissions: unknown[]) => ({
    chat: {id, bot_uuid: 'bot-1', platform: 'telegram', external_chat_id: external, alias, blocked: false},
    permissions,
});

const clipboard = {writeText: vi.fn().mockResolvedValue(undefined)};

beforeEach(() => {
    vi.clearAllMocks();
    Object.assign(navigator, {clipboard});
    Object.defineProperty(window, 'isSecureContext', {value: true, configurable: true});
    apiMock.listBotDirectChats.mockResolvedValue({
        chats: [
            chat('chat-uuid-1', 'telegram:111', 'Maya', allow),
            chat('chat-uuid-2', 'telegram:222', undefined, []),
        ],
    });
    apiMock.listBotGroups.mockResolvedValue({groups: [{id: 'group-uuid-1'}]});
    apiMock.getBotGroup.mockResolvedValue({
        group: {id: 'group-uuid-1', bot_uuid: 'bot-1', platform: 'telegram', external_group_id: 'telegram:-9', name: 'Release Crew', blocked: false},
        capabilities: {notify: 'allow'},
        actors: [],
    });
    apiMock.notifyBot.mockResolvedValue({ok: true});
});

const codeText = (container: HTMLElement) => container.querySelector('pre')?.textContent ?? '';

describe('NotifyGuide', () => {
    it('defaults to the first target that can receive and shows its friendly label', async () => {
        const {container} = render(<NotifyGuide bots={[bot()]} />);
        const input = await screen.findByRole('combobox');
        await waitFor(() => expect((input as HTMLInputElement).value).toBe('Telegram · Maya · 111'));
        expect(codeText(container)).toContain('"id": "chat-uuid-1"');
        expect(codeText(container)).toContain('http://gw.test/api/v1/bots/bot-1/notify');
        expect(codeText(container)).toContain('Bearer $TINGLY_TOKEN');
    });

    it('fills the sample from the picked target, including its kind', async () => {
        const {container} = render(<NotifyGuide bots={[bot()]} />);
        const input = await screen.findByRole('combobox');
        await waitFor(() => expect((input as HTMLInputElement).value).not.toBe(''));
        fireEvent.mouseDown(input);
        fireEvent.click(await screen.findByRole('option', {name: /Release Crew/}));
        await waitFor(() => expect(codeText(container)).toContain('"id": "group-uuid-1"'));
        expect(codeText(container)).toContain('"kind": "group"');
    });

    it('offers the same request in every language tab', async () => {
        const {container} = render(<NotifyGuide bots={[bot()]} />);
        await waitFor(() => expect(codeText(container)).toContain('chat-uuid-1'));
        fireEvent.click(screen.getByRole('tab', {name: 'JSON'}));
        expect(JSON.parse(codeText(container))).toEqual({
            target: {kind: 'direct_chat', id: 'chat-uuid-1'},
            title: 'Hello from Tingly Box',
            body: 'This is a test notification from the API guide.',
            level: 'info',
        });
        fireEvent.click(screen.getByRole('tab', {name: 'JavaScript'}));
        expect(codeText(container)).toContain('fetch("http://gw.test/api/v1/bots/bot-1/notify"');
        fireEvent.click(screen.getByRole('tab', {name: 'Python'}));
        expect(codeText(container)).toContain('requests.post(');
        expect(codeText(container)).toContain('"id": "chat-uuid-1"');
    });

    it('copies exactly the sample on screen', async () => {
        const {container} = render(<NotifyGuide bots={[bot()]} />);
        await waitFor(() => expect(codeText(container)).toContain('chat-uuid-1'));
        fireEvent.click(screen.getByTitle('Copy code'));
        await waitFor(() => expect(clipboard.writeText).toHaveBeenCalled());
        expect(clipboard.writeText.mock.calls[0][0]).toBe(buildSample('curl', 'http://gw.test', 'bot-1', {
            target: {kind: 'direct_chat', id: 'chat-uuid-1'},
            ...SAMPLE_MESSAGE,
        }));
    });

    it('sends the displayed request on Send test and reports delivery', async () => {
        render(<NotifyGuide bots={[bot()]} />);
        const send = await screen.findByRole('button', {name: 'Send test'});
        await waitFor(() => expect(send).toBeEnabled());
        fireEvent.click(send);
        expect(await screen.findByText(/^Delivered · /)).toBeInTheDocument();
        expect(apiMock.notifyBot).toHaveBeenCalledWith('bot-1', {
            target: {kind: 'direct_chat', id: 'chat-uuid-1'},
            title: 'Hello from Tingly Box',
            body: 'This is a test notification from the API guide.',
            level: 'info',
        });
    });

    it('shows the error when Send test fails', async () => {
        apiMock.notifyBot.mockResolvedValue({error: 'target not reachable'});
        render(<NotifyGuide bots={[bot()]} />);
        const send = await screen.findByRole('button', {name: 'Send test'});
        await waitFor(() => expect(send).toBeEnabled());
        fireEvent.click(send);
        expect(await screen.findByText('Failed: target not reachable')).toBeInTheDocument();
    });

    it('does not offer Send test for a target that is not allowed to receive', async () => {
        apiMock.listBotDirectChats.mockResolvedValue({chats: [chat('chat-uuid-2', 'telegram:222', undefined, [])]});
        apiMock.listBotGroups.mockResolvedValue({groups: []});
        render(<NotifyGuide bots={[bot()]} />);
        await screen.findByText(/can’t receive notifications yet/);
        expect(screen.getByRole('button', {name: 'Send test'})).toBeDisabled();
    });

    it('groups targets by bot when several bots have Notify on', async () => {
        const second = bot({uuid: 'bot-2', name: 'Feishu bot', platform: 'feishu'});
        apiMock.listBotDirectChats.mockImplementation(async (uuid: string) => ({
            chats: [chat(`${uuid}-chat`, `${uuid}:1`, `Chat of ${uuid}`, allow)],
        }));
        apiMock.listBotGroups.mockResolvedValue({groups: []});
        render(<NotifyGuide bots={[bot(), second]} />);
        const input = await screen.findByRole('combobox');
        await waitFor(() => expect((input as HTMLInputElement).value).not.toBe(''));
        fireEvent.mouseDown(input);
        const list = await screen.findByRole('listbox');
        expect(within(list).getByText('Ops bot')).toBeInTheDocument();
        expect(within(list).getByText('Feishu bot')).toBeInTheDocument();
    });

    it('explains how a chat appears when the bot has none yet', async () => {
        apiMock.listBotDirectChats.mockResolvedValue({chats: []});
        apiMock.listBotGroups.mockResolvedValue({groups: []});
        render(<NotifyGuide bots={[bot()]} />);
        expect(await screen.findByText(/No chats yet\. .*message.*telegram/)).toBeInTheDocument();
        expect(screen.getByRole('button', {name: 'Send test'})).toBeDisabled();
    });

    it('says Notify is off instead of listing targets when the capability is disabled', async () => {
        render(<NotifyGuide bots={[bot({capabilities: [{bot_uuid: 'bot-1', capability: 'notify', enabled: false}]})]} />);
        expect(await screen.findByText(/Notify is off/)).toBeInTheDocument();
        expect(apiMock.listBotDirectChats).not.toHaveBeenCalled();
        expect(screen.getByRole('combobox')).toBeDisabled();
    });

    it('wraps code instead of clipping or scrolling it sideways', async () => {
        const {container} = render(<NotifyGuide bots={[bot()]} />);
        await waitFor(() => expect(codeText(container)).toContain('chat-uuid-1'));
        const pre = container.querySelector('pre') as HTMLElement;
        expect(pre.style.whiteSpace).toBe('pre-wrap');
        expect(pre.style.wordBreak).toBe('break-word');
        expect((pre.parentElement as HTMLElement).style.overflowX || getComputedStyle(pre.parentElement as HTMLElement).overflowX).toBe('hidden');
    });

    it('lists every request field with its requirement', async () => {
        render(<NotifyGuide bots={[bot()]} />);
        for (const field of ['target.kind', 'target.id', 'title', 'body', 'level']) {
            expect(await screen.findByText(field)).toBeInTheDocument();
        }
        expect(screen.getAllByText('required')).toHaveLength(3);
        expect(screen.getAllByText('optional')).toHaveLength(2);
    });
});
