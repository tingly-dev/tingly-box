import { cleanup, render, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Thread } from '../../client';
import { ThreadPanel } from './ThreadPanel';
import { ProjectsHostProvider } from '../../host/ProjectsHost';

const m = vi.hoisted(() => ({ get: vi.fn((_url: string, _opts?: { params: { path: { id: string }; query: Record<string, unknown> } }) => Promise.resolve({ data: { events: [] }, response: new Response() })) }));
vi.mock('../../client', async (actual) => ({ ...(await actual<typeof import('../../client')>()), api: { GET: m.get } }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string, opts?: { defaultValue?: string }) => opts?.defaultValue ?? key, i18n: { language: 'en' } }) }));
vi.mock('./threadOps', () => ({ threadOps: () => ({ respond: vi.fn() }) }));
vi.mock('../ThreadComposer', () => ({ ThreadComposer: () => <div>Composer</div> }));
vi.mock('./DiffDialog', () => ({ DiffDialog: () => null }));
vi.mock('./ComposerStrip', () => ({ ComposerStrip: () => null }));
vi.mock('../Markdown', () => ({ Markdown: ({ text }: { text: string }) => <div>{text}</div>, CodeBlock: () => null }));

const host = { paths: { list: '/', project: () => '/', folder: () => '/' } };
const thread = { id: 't', title: 'Work', status: 'idle', task: '', branch: '', cwd: '/work/app', input_tokens: 0, output_tokens: 0, turns: 0 } as Thread;

afterEach(() => {
    cleanup();
    m.get.mockClear();
});

describe('the checklist history', () => {
    it('reads only the checklist tools, not the whole transcript, when older messages exist', async () => {
        render(
            <ProjectsHostProvider host={host}>
                <ThreadPanel projectId="p" threadId="t" thread={thread} events={[]} more running={false} disabled={false} onBack={() => {}} />
            </ProjectsHostProvider>,
        );
        await waitFor(() => expect(m.get).toHaveBeenCalled());
        const call = m.get.mock.calls.find(([url]) => url === '/api/v1/threads/{id}/events') as unknown as
            [string, { params: { path: { id: string }; query: Record<string, unknown> } }] | undefined;
        expect(call).toBeDefined();
        expect(call![1].params.query).toEqual({ tools: 'TodoWrite,TaskCreate,TaskUpdate,TaskList,TaskGet' });
    });
});
