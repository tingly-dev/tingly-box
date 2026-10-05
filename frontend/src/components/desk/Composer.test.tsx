import {act, cleanup, fireEvent, render, screen} from '@testing-library/react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import Composer from './Composer';

const {mobile} = vi.hoisted(() => ({mobile: {value: false}}));
vi.mock('@mui/material', async (original) => ({...await original<typeof import('@mui/material')>(), useMediaQuery: () => mobile.value}));
beforeEach(() => {mobile.value = false;});
vi.mock('react-i18next', () => ({useTranslation: () => ({t: (_key: string, opts: {defaultValue: string}) => opts.defaultValue})}));
afterEach(cleanup);

describe('Desk composer', () => {
    it('keeps text typed while a send is in flight, and prevents duplicate sends', async () => {
        let finish!: (accepted: boolean) => void;
        const send = vi.fn(() => new Promise<boolean>((resolve) => { finish = resolve; }));
        render(<Composer placeholder="Reply" onSubmit={send}/>);
        const input = screen.getByPlaceholderText('Reply');
        fireEvent.change(input, {target: {value: 'first message'}});
        fireEvent.keyDown(input, {key: 'Enter'});
        fireEvent.keyDown(input, {key: 'Enter'});
        fireEvent.change(input, {target: {value: 'next draft'}});
        await act(async () => finish(true));
        expect(send).toHaveBeenCalledExactlyOnceWith('first message');
        expect(input).toHaveValue('next draft');
    });

    it('keeps a failed send and clears only an accepted unchanged draft', async () => {
        const send = vi.fn().mockResolvedValueOnce(false).mockResolvedValueOnce(true);
        render(<Composer placeholder="Reply" onSubmit={send}/>);
        const input = screen.getByPlaceholderText('Reply');
        fireEvent.change(input, {target: {value: 'keep me'}});
        await act(async () => fireEvent.click(screen.getByRole('button', {name: 'Send'})));
        expect(input).toHaveValue('keep me');
        await act(async () => fireEvent.click(screen.getByRole('button', {name: 'Send'})));
        expect(input).toHaveValue('');
    });

    it('does not submit when Enter confirms an IME candidate or adds a line', () => {
        const send = vi.fn();
        render(<Composer placeholder="Reply" onSubmit={send}/>);
        const input = screen.getByPlaceholderText('Reply');
        fireEvent.change(input, {target: {value: '中文'}});
        fireEvent.keyDown(input, {key: 'Enter', isComposing: true});
        fireEvent.keyDown(input, {key: 'Enter', shiftKey: true});
        expect(send).not.toHaveBeenCalled();
    });
});


it('mobile Enter adds a line and Ctrl+Enter submits without swallowing IME confirmation', async () => {
    mobile.value = true;
    const send = vi.fn().mockResolvedValue(true);
    render(<Composer placeholder="Reply" onSubmit={send}/>);
    const input = screen.getByRole('textbox', {name: 'Reply'});
    fireEvent.change(input, {target: {value: 'Mobile text'}});
    fireEvent.keyDown(input, {key: 'Enter'});
    fireEvent.keyDown(input, {key: 'Enter', ctrlKey: true, isComposing: true});
    expect(send).not.toHaveBeenCalled();
    await act(async () => fireEvent.keyDown(input, {key: 'Enter', ctrlKey: true}));
    expect(send).toHaveBeenCalledExactlyOnceWith('Mobile text');
});

it('prevents repeated stop requests while the first is pending', async () => {
    let finish!: () => void;
    const stop = vi.fn(() => new Promise<void>((resolve) => {finish = resolve;}));
    render(<Composer placeholder="Reply" onSubmit={vi.fn()} onStop={stop}/>);
    const button = screen.getByRole('button', {name: 'Stop'});
    fireEvent.click(button);
    fireEvent.click(button);
    expect(stop).toHaveBeenCalledOnce();
    expect(button).toBeDisabled();
    await act(async () => finish());
    expect(button).toBeEnabled();
});
