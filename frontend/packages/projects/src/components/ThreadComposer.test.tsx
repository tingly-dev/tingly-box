import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ThreadComposer } from './ThreadComposer';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => ({ 'composer.sendShort': 'Send' })[key] ?? key }) }));
afterEach(() => {
    cleanup();
    window.localStorage.clear();
});

describe('ThreadComposer', () => {
    it('keeps an unsent draft across a remount and drops it once sent', async () => {
        const send = vi.fn().mockResolvedValue(true);
        const first = render(<ThreadComposer draftKey="t1" placeholder="Reply" onSend={send} />);
        fireEvent.change(screen.getByPlaceholderText('Reply'), { target: { value: 'half written' } });
        first.unmount();

        render(<ThreadComposer draftKey="t1" placeholder="Reply" onSend={send} />);
        const input = screen.getByPlaceholderText('Reply');
        expect(input).toHaveValue('half written');
        await act(async () => fireEvent.keyDown(input, { key: 'Enter' }));
        expect(send).toHaveBeenCalledExactlyOnceWith('half written', []);
        expect(input).toHaveValue('');
        cleanup();

        render(<ThreadComposer draftKey="t1" placeholder="Reply" onSend={send} />);
        expect(screen.getByPlaceholderText('Reply')).toHaveValue('');
    });

    it('keeps drafts apart per conversation and lets a message queue while a turn runs', async () => {
        const send = vi.fn().mockResolvedValue(true);
        const a = render(<ThreadComposer draftKey="a" placeholder="Reply" streaming onSend={send} />);
        const input = screen.getByPlaceholderText('Reply');
        fireEvent.change(input, { target: { value: 'queued' } });
        await act(async () => fireEvent.keyDown(input, { key: 'Enter' }));
        expect(send).toHaveBeenCalledExactlyOnceWith('queued', []);
        a.unmount();

        render(<ThreadComposer draftKey="b" placeholder="Reply" onSend={send} />);
        expect(screen.getByPlaceholderText('Reply')).toHaveValue('');
    });
});
