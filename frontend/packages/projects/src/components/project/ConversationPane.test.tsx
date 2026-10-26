import type { ComponentProps } from 'react';
import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { ThemeProvider } from '@mui/material/styles';
import createAppTheme from '@tingly/ui/theme';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Thread, ThreadEvent } from '../../client';
import { Markdown } from '../Markdown';
import ThreadMarkdown from '../../transcript/Markdown';
import Transcript from '../../transcript/Transcript';
import type { TranscriptBlock } from '../../transcript/model';
import { ConversationPane } from './ConversationPane';
import { ProjectsHostProvider } from '../../host/ProjectsHost';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string, options?: Record<string, unknown>) => key === 'im.source' ? `${options?.platform} · ${options?.chat}` : key }) }));
const composer = vi.hoisted(() => vi.fn());
vi.mock('../ThreadComposer', () => ({ ThreadComposer: (props: unknown) => { composer(props); return <div data-testid="composer" />; } }));

beforeEach(() => { localStorage.clear(); composer.mockClear(); });
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const event = (kind: string, content: string, extra: Partial<ThreadEvent> = {}): ThreadEvent => ({
    id: `${kind}-${content}`, kind, content, thread_id: 'coordinator', created_at: '', updated_at: '', ...extra,
});
const props = (messages: ThreadEvent[] = []): ComponentProps<typeof ConversationPane> => ({
    projectId: 'p', threadId: 'coordinator', messages, threads: new Map(), busy: false, disabled: false,
    onSend: vi.fn().mockResolvedValue(true), onOpenThread: vi.fn(), onStartSuggestions: vi.fn().mockResolvedValue(undefined),
});
const show = (p: ComponentProps<typeof ConversationPane>) => render(<MemoryRouter><ProjectsHostProvider host={{ paths: { list: '/projects', project: (id) => `/projects/${id}`, folder: () => '/folders' } }}><ConversationPane {...p} /></ProjectsHostProvider></MemoryRouter>);

describe('conversation presentation', () => {
    it.each(['light', 'dark', 'sunlit', 'claude', 'ds'] as const)('matches Thread message and Markdown styles in %s', (mode) => {
        const text = '# Heading\n\nParagraph with **emphasis** and `inline`.\n\n> Quote\n\n- List item\n\n| Name | Value |\n| --- | --- |\n| One | Two |';
        const blocks: TranscriptBlock[] = [{ type: 'user', message: { kind: 'user', content: 'Same user message', timestamp: '' } }];
        const view = render(<ThemeProvider theme={createAppTheme(mode)}><MemoryRouter>
            <section data-testid="project"><ConversationPane {...props([event('user', 'Same user message'), event('assistant', text)])} /></section>
            <section data-testid="thread"><Transcript blocks={blocks} working={false} onRespond={vi.fn()} /><ThreadMarkdown content={text} /></section>
        </MemoryRouter></ThemeProvider>);
        const project = within(screen.getByTestId('project'));
        const thread = within(screen.getByTestId('thread'));
        const a = project.getByText('Same user message');
        const b = thread.getByText('Same user message');
        for (const key of ['color', 'fontSize', 'lineHeight', 'whiteSpace', 'wordBreak'] as const) {
            expect(getComputedStyle(a)[key]).toBe(getComputedStyle(b)[key]);
        }
        for (const key of ['backgroundColor', 'maxWidth', 'padding', 'borderRadius'] as const) {
            expect(getComputedStyle(a.parentElement!)[key]).toBe(getComputedStyle(b.parentElement!)[key]);
        }
        expect(getComputedStyle(a).fontSize).toBe('14px');
        expect(getComputedStyle(a.parentElement!).maxWidth).toBe('80%');
        for (const label of ['Heading', 'Quote', 'List item', 'Name']) {
            for (const key of ['color', 'fontSize', 'lineHeight', 'fontWeight', 'marginBottom', 'paddingLeft', 'borderLeftColor'] as const) {
                expect(getComputedStyle(project.getByText(label))[key]).toBe(getComputedStyle(thread.getByText(label))[key]);
            }
        }
        expect(view.container.querySelectorAll('table')).toHaveLength(2);
    });

    it('keeps the existing typography for Markdown outside conversation replies', () => {
        render(<><section data-testid="detail"><Markdown text="# Detail" /></section><section data-testid="reply"><Markdown text="# Reply" conversation /></section></>);
        expect(getComputedStyle(within(screen.getByTestId('detail')).getByRole('heading')).fontSize).toBe('22.4px');
        expect(getComputedStyle(within(screen.getByTestId('reply')).getByRole('heading')).fontSize).toBe('20px');
    });

    it('keeps GFM, safe links, escaped HTML and copyable code in conversation Markdown', async () => {
        const writeText = vi.fn().mockResolvedValue(undefined);
        vi.stubGlobal('isSecureContext', true);
        Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
        const view = render(<Markdown conversation text={'~~removed~~ [Docs](https://example.com)\n\n<script>alert("unsafe")</script>\n\n```js\nconst value = 1;\n```'} />);
        expect(view.container.querySelector('del')).toHaveTextContent('removed');
        expect(screen.getByRole('link', { name: 'Docs' })).toHaveAttribute('target', '_blank');
        expect(screen.getByRole('link', { name: 'Docs' })).toHaveAttribute('rel', 'noreferrer');
        expect(view.container.querySelector('script')).toBeNull();
        await act(async () => fireEvent.click(screen.getByRole('button', { name: 'common.copy' })));
        expect(writeText).toHaveBeenCalledWith('const value = 1;');
    });

    it('preserves attachment-only messages, IM provenance, busy and error actions', () => {
        show({ ...props([
            event('user', '', { source: 'im', source_platform: 'telegram', source_chat_id: 'chat', payload: JSON.stringify({ attachments: [{ name: 'log.txt', path: 'uploads/log.txt', type: 'text/plain', size: 32 }] }) }),
            event('error', 'Connection failed'),
        ]), busy: true, chatTitle: () => 'Team chat' });
        expect(screen.getByText('log.txt')).toBeInTheDocument();
        expect(screen.getByText(/Team chat/)).toBeInTheDocument();
        expect(screen.getByRole('status')).toHaveTextContent('chat.thinking');
        expect(within(screen.getByRole('status')).getByRole('progressbar')).toBeInTheDocument();
        expect(screen.getByRole('alert')).toHaveTextContent('Connection failed');
        expect(screen.getByRole('link', { name: 'connection.open' })).toHaveAttribute('href', '/settings');
        expect(composer).toHaveBeenLastCalledWith(expect.objectContaining({ draftKey: 'coordinator', streaming: true, placeholder: 'conversation.queuePlaceholder' }));
    });

    it('keeps thread opening, suggestions, folded reports and their persisted visibility', async () => {
        const p = props([
            event('thread_started', '', { payload: JSON.stringify({ thread_ids: ['t'] }) }),
            event('suggestions', '', { id: 'suggest', payload: JSON.stringify({ suggestions: [{ title: 'Next task', task: 'Do this' }, { title: 'Other task', task: 'Then this' }] }) }),
            event('thread_update', 'Report details', { payload: JSON.stringify({ thread_id: 't', title: 'Task report', kind: 'finished', status: 'ready' }) }),
        ]);
        p.threads = new Map([['t', { id: 't', title: 'Live task', status: 'working', commits_ahead: 0 } as Thread]]);
        show(p);
        fireEvent.click(screen.getByRole('button', { name: /Live task/ }));
        expect(p.onOpenThread).toHaveBeenCalledWith('t');
        await act(async () => fireEvent.click(screen.getAllByRole('button', { name: 'conversation.start' })[0]));
        expect(p.onStartSuggestions).toHaveBeenCalledWith('suggest', [0]);
        await act(async () => fireEvent.click(screen.getByRole('button', { name: 'conversation.startAll' })));
        expect(p.onStartSuggestions).toHaveBeenCalledWith('suggest', []);
        expect(screen.queryByText('Report details')).toBeNull();
        fireEvent.click(screen.getByRole('button', { expanded: false }));
        expect(screen.getByText('Report details')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'conversation.hideReports' }));
        expect(screen.queryByText('Task report')).toBeNull();
        expect(localStorage.getItem('tingly-project.conversation.threadReports')).toBe('0');
        fireEvent.click(screen.getByRole('button', { name: 'conversation.showReports' }));
        expect(screen.getByText('Task report')).toBeInTheDocument();
    });

    it('leaves composer configuration, send callbacks and empty-state seeds intact', async () => {
        const p = { ...props(), disabled: true, model: 'chosen-model', onModelChange: vi.fn(), composerContext: <span>Controls</span> };
        show(p);
        fireEvent.click(screen.getByText('conversation.example1'));
        const configured = composer.mock.lastCall![0];
        expect(configured).toMatchObject({ disabled: true, model: 'chosen-model', onModelChange: p.onModelChange, context: p.composerContext, seed: 'conversation.example1' });
        await configured.onSend('Hello', []);
        expect(p.onSend).toHaveBeenCalledWith('Hello', []);
    });
});
