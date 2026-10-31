import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Thread, ThreadEvent } from '../../client';
import { ThreadPanel } from './ThreadPanel';
import { readLocal } from '../../utils/storage';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string, opts?: { defaultValue?: string }) => opts?.defaultValue ?? key }) }));
const respond = vi.hoisted(() => vi.fn().mockResolvedValue(undefined));
vi.mock('./threadOps', () => ({ threadOps: () => ({ respond }) }));
vi.mock('../ThreadComposer', () => ({ ThreadComposer: () => <div>Composer</div> }));
vi.mock('./DiffDialog', () => ({ DiffDialog: () => null }));
vi.mock('./ComposerStrip', () => ({ ComposerStrip: () => null }));
vi.mock('../Markdown', () => ({ Markdown: ({ text }: { text: string }) => <div>{text}</div>, CodeBlock: () => null }));

beforeEach(() => { localStorage.clear(); respond.mockClear(); });
afterEach(cleanup);
const thread = { id: 't', title: 'Work', status: 'waiting', task: '', branch: '', input_tokens: 0, output_tokens: 0 } as Thread;
const event = (id: string, kind: string, content: string, extra: Partial<ThreadEvent> = {}): ThreadEvent => ({
    id, kind, content, thread_id: 't', created_at: '2026-10-25T00:00:00Z', updated_at: '', ...extra,
});
const events = [
    event('u', 'user', 'Check the code'),
    event('c', 'tool_use', 'Bash', { request_id: 'call', payload: JSON.stringify({ command: 'go test ./...' }) }),
    event('p', 'approval', 'Permission needed', { payload: JSON.stringify({ request_id: 'permission', state: 'pending', tool: 'Bash', input: { command: 'go test ./...' } }) }),
];
const props = { projectId: 'p', threadId: 't', thread, events, running: true, disabled: false, onBack: () => {} };

describe('thread message detail integration', () => {
    it.each([false, true])('filters paired calls but retains the actual response path (folder=%s)', async (folder) => {
        const requestId = `permission-${folder}`;
        const currentEvents = events.map((e) => e.kind === 'approval' ? { ...e, payload: e.payload!.replace('permission', requestId) } : e);
        const first = render(<ThreadPanel {...props} events={currentEvents} folder={folder}/>);
        expect(document.querySelector('[data-call-ids="call"]')).not.toBeNull();
        fireEvent.click(screen.getByRole('button', { name: 'messageDetail.concise' }));
        expect(document.querySelector('[data-call-ids="call"]')).toBeNull();
        expect(screen.getByText('Check the code')).toBeInTheDocument();
        expect(screen.getByText('go test ./...')).toBeInTheDocument();
        await act(async () => fireEvent.click(screen.getByRole('button', { name: 'Allow' })));
        expect(respond).toHaveBeenCalledWith(requestId, true, false, '', undefined);
        expect(readLocal('thread.messageDetail')).toBe('concise');
        first.unmount();
        render(<ThreadPanel {...props} folder={folder} threadId="another" thread={{ ...thread, id: 'another' }} events={[]}/>);
        expect(screen.getByRole('button', { name: 'messageDetail.concise' })).toHaveAttribute('aria-pressed', 'true');
        fireEvent.click(screen.getByRole('button', { name: 'messageDetail.detailed' }));
        expect(readLocal('thread.messageDetail')).toBe('detailed');
    });
});

it('distinguishes a reported model from a folder session’s configured model', () => {
    const view = render(<ThreadPanel {...props} thread={{ ...thread, model: 'chosen-alias', reported_model: 'reported-worker-model' }} />);
    expect(screen.getByText('threadSettings.lastModel: reported-worker-model')).toBeVisible();
    view.rerender(<ThreadPanel {...props} thread={{ ...thread, model: 'chosen-alias' }} />);
    expect(screen.getByText('threadSettings.lastModel: threadSettings.modelUnknown')).toBeVisible();
    view.unmount();
    const usage = event('usage', 'usage', '', { payload: JSON.stringify({ model: 'provider-model' }) });
    render(<ThreadPanel {...props} folder thread={{ ...thread, model: 'configured-alias', reported_model: 'provider-model' }} events={[usage]} />);
    expect(screen.getByText('threadSettings.lastModel: provider-model')).toBeVisible();
    expect(screen.queryByText('threadSettings.lastModel: configured-alias')).not.toBeInTheDocument();
});
