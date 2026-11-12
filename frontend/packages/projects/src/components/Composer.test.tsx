import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Composer } from './Composer';
import { en } from '../i18n/en';
import { zh } from '../i18n/zh';

const { mobile } = vi.hoisted(() => ({ mobile: { value: false } }));
vi.mock('@mui/material', async (original) => ({ ...await original<typeof import('@mui/material')>(), useMediaQuery: () => mobile.value }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => ({ 'composer.sendShort': 'Send', 'composer.stop': 'Stop' })[key] ?? key }) }));
beforeEach(() => { mobile.value = false; });
afterEach(cleanup);

describe('Composer', () => {
    it('keeps secondary settings available without crowding the composer', () => {
        render(<Composer onSend={vi.fn()} contextCollapsible context={<span>Permission options</span>} />);
        const toggle = screen.getByRole('button', { name: 'composer.settings' });
        expect(toggle).toHaveAttribute('aria-expanded', 'false');
        expect(screen.queryByText('Permission options')).not.toBeInTheDocument();
        fireEvent.click(toggle);
        expect(toggle).toHaveAttribute('aria-expanded', 'true');
        expect(screen.getByText('Permission options')).toBeVisible();
    });

    it('clears at once and puts a refused message back while the box is still empty', async () => {
        const send = vi.fn().mockResolvedValueOnce(false);
        render(<Composer placeholder="Reply" onSend={send} />);
        const input = screen.getByPlaceholderText('Reply');
        fireEvent.change(input, { target: { value: 'hello' } });
        await act(async () => fireEvent.keyDown(input, { key: 'Enter' }));
        expect(send).toHaveBeenCalledExactlyOnceWith('hello', []);
        expect(input).toHaveValue('hello');
    });

    it('keeps text until sent, keeps what is typed meanwhile, and never sends twice', async () => {
        let finish!: (accepted: boolean) => void;
        const send = vi.fn(() => new Promise<boolean>((resolve) => { finish = resolve; }));
        render(<Composer placeholder="Reply" keepUntilSent onSend={send} />);
        const input = screen.getByPlaceholderText('Reply');
        fireEvent.change(input, { target: { value: 'first message' } });
        fireEvent.keyDown(input, { key: 'Enter' });
        fireEvent.keyDown(input, { key: 'Enter' });
        fireEvent.change(input, { target: { value: 'next draft' } });
        await act(async () => finish(true));
        expect(send).toHaveBeenCalledExactlyOnceWith('first message', []);
        expect(input).toHaveValue('next draft');
    });

    it('keeps a failed send and clears only an accepted unchanged draft', async () => {
        const send = vi.fn().mockResolvedValueOnce(false).mockResolvedValueOnce(true);
        render(<Composer placeholder="Reply" keepUntilSent onSend={send} />);
        const input = screen.getByPlaceholderText('Reply');
        fireEvent.change(input, { target: { value: 'keep me' } });
        await act(async () => fireEvent.click(screen.getByRole('button', { name: 'Send' })));
        expect(input).toHaveValue('keep me');
        await act(async () => fireEvent.click(screen.getByRole('button', { name: 'Send' })));
        expect(input).toHaveValue('');
    });

    it('does not send when Enter confirms an IME candidate or adds a line', () => {
        const send = vi.fn();
        render(<Composer placeholder="Reply" onSend={send} />);
        const input = screen.getByPlaceholderText('Reply');
        fireEvent.change(input, { target: { value: '中文' } });
        fireEvent.keyDown(input, { key: 'Enter', isComposing: true });
        fireEvent.keyDown(input, { key: 'Enter', shiftKey: true });
        expect(send).not.toHaveBeenCalled();
    });

    it('on a phone, Enter adds a line and Ctrl+Enter sends', async () => {
        mobile.value = true;
        const send = vi.fn().mockResolvedValue(true);
        render(<Composer placeholder="Reply" onSend={send} />);
        const input = screen.getByRole('textbox', { name: 'Reply' });
        fireEvent.change(input, { target: { value: 'Mobile text' } });
        fireEvent.keyDown(input, { key: 'Enter' });
        fireEvent.keyDown(input, { key: 'Enter', ctrlKey: true, isComposing: true });
        expect(send).not.toHaveBeenCalled();
        await act(async () => fireEvent.keyDown(input, { key: 'Enter', ctrlKey: true }));
        expect(send).toHaveBeenCalledExactlyOnceWith('Mobile text', []);
    });

    it('while a turn runs: Stop once at a time, and Send only when the host takes messages meanwhile', async () => {
        let finish!: () => void;
        const stop = vi.fn(() => new Promise<void>((resolve) => { finish = resolve; }));
        const view = render(<Composer placeholder="Reply" streaming onStop={stop} onSend={vi.fn()} />);
        const button = screen.getByRole('button', { name: 'Stop' });
        fireEvent.click(button);
        fireEvent.click(button);
        expect(stop).toHaveBeenCalledOnce();
        expect(button).toBeDisabled();
        await act(async () => finish());
        expect(button).toBeEnabled();
        fireEvent.change(screen.getByPlaceholderText('Reply'), { target: { value: 'later' } });
        expect(screen.queryByRole('button', { name: 'Send' })).not.toBeInTheDocument();
        view.rerender(<Composer placeholder="Reply" streaming sendWhileStreaming onStop={stop} onSend={vi.fn()} />);
        expect(screen.getByRole('button', { name: 'Send' })).toBeEnabled();
    });
});

describe('Composer send modes', () => {
    const running = (onSend = vi.fn().mockResolvedValue(true), sendModes = true) => {
        render(<Composer placeholder="Reply" streaming sendWhileStreaming sendModes={sendModes} onStop={vi.fn()} onSend={onSend} />);
        fireEvent.change(screen.getByPlaceholderText('Reply'), { target: { value: 'change course' } });
        return onSend;
    };

    it('sends with no mode by default, so the host joins the running turn', async () => {
        const send = running();
        await act(async () => fireEvent.keyDown(screen.getByPlaceholderText('Reply'), { key: 'Enter' }));
        expect(send).toHaveBeenCalledExactlyOnceWith('change course', []);
    });

    it('sends after the turn on Alt+Enter, and offers both choices in the menu', async () => {
        const send = running();
        await act(async () => fireEvent.keyDown(screen.getByPlaceholderText('Reply'), { key: 'Enter', altKey: true }));
        expect(send).toHaveBeenCalledExactlyOnceWith('change course', [], 'queue');
    });

    it('stops and sends from the menu', async () => {
        const send = running();
        fireEvent.click(screen.getByRole('button', { name: 'composer.sendOptions' }));
        expect(screen.getByText('composer.sendQueue')).toBeInTheDocument();
        await act(async () => fireEvent.click(screen.getByText('composer.sendStop')));
        expect(send).toHaveBeenCalledExactlyOnceWith('change course', [], 'stop_and_send');
    });

    it('shows no menu, and ignores Alt+Enter, when no turn runs or the host did not opt in', async () => {
        const send = running(undefined, false);
        expect(screen.queryByRole('button', { name: 'composer.sendOptions' })).not.toBeInTheDocument();
        await act(async () => fireEvent.keyDown(screen.getByPlaceholderText('Reply'), { key: 'Enter', altKey: true }));
        expect(send).toHaveBeenCalledExactlyOnceWith('change course', []);
    });
});

describe('Composer attachments', () => {
    const stored = (name: string) => ({ name, path: `uploads/x/${name}`, type: 'text/plain', size: 3 });

    it('turns a long paste into a text file and sends it with the message', async () => {
        const attach = vi.fn(async (files: File[]) => files.map((f) => stored(f.name)));
        const onSend = vi.fn(async () => true);
        render(<Composer attach={attach} onSend={onSend} />);
        const box = screen.getByRole('textbox');
        const long = Array.from({ length: 60 }, (_, i) => `line ${i}`).join('\n');
        fireEvent.paste(box, { clipboardData: { files: [], getData: () => long } });
        await act(async () => {});
        expect(attach).toHaveBeenCalledTimes(1);
        expect(attach.mock.calls[0][0][0].name).toBe('pasted-text.txt');
        expect((box as HTMLTextAreaElement).value).toBe('');
        // The file alone is enough to send.
        fireEvent.click(screen.getByRole('button', { name: 'Send' }));
        await act(async () => {});
        expect(onSend).toHaveBeenCalledWith('', [stored('pasted-text.txt')]);
    });

    it('leaves a short paste in the box and holds Send while a file uploads', async () => {
        let finish: (a: ReturnType<typeof stored>[]) => void = () => {};
        const attach = vi.fn(() => new Promise<ReturnType<typeof stored>[]>((r) => { finish = r; }));
        const onSend = vi.fn(async () => true);
        render(<Composer attach={attach} onSend={onSend} />);
        fireEvent.paste(screen.getByRole('textbox'), { clipboardData: { files: [], getData: () => 'short' } });
        expect(attach).not.toHaveBeenCalled();
        const file = new File(['abc'], 'notes.txt', { type: 'text/plain' });
        fireEvent.drop(screen.getByRole('textbox'), { dataTransfer: { files: [file], types: ['Files'] } });
        fireEvent.change(screen.getByRole('textbox'), { target: { value: 'see this' } });
        const send = screen.getByRole('button', { name: 'Send' });
        expect(send).toBeDisabled();
        await act(async () => finish([stored('notes.txt')]));
        expect(send).toBeEnabled();
        fireEvent.click(send);
        await act(async () => {});
        expect(onSend).toHaveBeenCalledWith('see this', [stored('notes.txt')]);
    });

    it('shows a failed upload and sends nothing', async () => {
        const attach = vi.fn(async () => { throw new Error('too big'); });
        render(<Composer attach={attach} onSend={vi.fn()} />);
        fireEvent.drop(screen.getByRole('textbox'), { dataTransfer: { files: [new File(['a'], 'a.bin')], types: ['Files'] } });
        await act(async () => {});
        expect(screen.getByRole('alert')).toHaveTextContent('too big');
        expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled();
    });

    it('disables Attach with its hint and ignores dropped files until it is allowed', () => {
        const attach = vi.fn().mockResolvedValue([]);
        render(<Composer placeholder="Reply" onSend={vi.fn()} attach={attach} attachBlocked="Choose a folder first" />);
        expect(screen.getByRole('button', { name: 'composer.attach' })).toBeDisabled();
        fireEvent.change(screen.getByTestId('composer-file-input'), { target: { files: [new File(['x'], 'a.txt')] } });
        expect(attach).not.toHaveBeenCalled();
    });
});

describe('Composer slash menu', () => {
    const setup = (props: Partial<Parameters<typeof Composer>[0]> = {}) => {
        const send = vi.fn().mockResolvedValue(true);
        render(<Composer placeholder="Reply" slashCommands onSend={send} {...props} />);
        return { send, input: screen.getByPlaceholderText('Reply') };
    };

    it('offers /compact when the message starts with a slash, and inserts it on Enter instead of sending', () => {
        const { send, input } = setup();
        fireEvent.change(input, { target: { value: '/' } });
        expect(screen.getByRole('option', { name: /compact/ })).toBeInTheDocument();
        fireEvent.keyDown(input, { key: 'Enter' });
        expect(input).toHaveValue('/compact ');
        expect(send).not.toHaveBeenCalled();
        expect(screen.queryByRole('option')).not.toBeInTheDocument();
    });

    it('explains that old messages remain viewable without automatically inheriting their context', () => {
        expect(en.composer.newDescription).toBe('Start a new session without automatically inheriting previous conversation context; previous messages remain viewable');
        expect(zh.composer.newDescription).toBe('开始新会话，不自动继承之前的对话上下文；仍可查看之前的消息');
    });

    it('offers and sends /new through the normal composer path', async () => {
        const { send, input } = setup();
        fireEvent.change(input, { target: { value: '/n' } });
        expect(screen.getByRole('option', { name: /new/ })).toHaveTextContent('composer.newDescription');
        fireEvent.keyDown(input, { key: 'Tab' });
        expect(input).toHaveValue('/new ');
        await act(async () => fireEvent.keyDown(input, { key: 'Enter' }));
        expect(send).toHaveBeenCalledExactlyOnceWith('/new', []);
    });

    it('narrows as the name is typed, completes with Tab and with a click', () => {
        const { input } = setup();
        fireEvent.change(input, { target: { value: '/co' } });
        fireEvent.keyDown(input, { key: 'Tab' });
        expect(input).toHaveValue('/compact ');

        fireEvent.change(input, { target: { value: '/x' } });
        expect(screen.queryByRole('option')).not.toBeInTheDocument();
        fireEvent.change(input, { target: { value: '/' } });
        fireEvent.mouseDown(screen.getByRole('option', { name: /compact/ }));
        expect(input).toHaveValue('/compact ');
    });

    it('sends a complete command on Enter, and closes with Escape', async () => {
        const { send, input } = setup();
        fireEvent.change(input, { target: { value: '/compact' } });
        expect(screen.queryByRole('option')).not.toBeInTheDocument();
        await act(async () => fireEvent.keyDown(input, { key: 'Enter' }));
        expect(send).toHaveBeenCalledExactlyOnceWith('/compact', []);

        fireEvent.change(input, { target: { value: '/com' } });
        fireEvent.keyDown(input, { key: 'Escape' });
        expect(screen.queryByRole('option')).not.toBeInTheDocument();
        fireEvent.change(input, { target: { value: '/comp' } });
        expect(screen.getByRole('option')).toBeInTheDocument();
    });

    it('stays out of the way of ordinary text and when not enabled', () => {
        const { input } = setup();
        fireEvent.change(input, { target: { value: 'compare /etc/hosts' } });
        expect(screen.queryByRole('option')).not.toBeInTheDocument();
        fireEvent.change(input, { target: { value: '/etc/hosts is broken' } });
        expect(screen.queryByRole('option')).not.toBeInTheDocument();
        cleanup();
        const off = setup({ slashCommands: false });
        fireEvent.change(off.input, { target: { value: '/' } });
        expect(screen.queryByRole('option')).not.toBeInTheDocument();
    });
});
