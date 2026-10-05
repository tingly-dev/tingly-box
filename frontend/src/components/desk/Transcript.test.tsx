import {act, cleanup, fireEvent, render, screen} from '@testing-library/react';
import {afterEach, describe, expect, it, vi} from 'vitest';
import Transcript from './Transcript';
import type {TranscriptBlock} from './deskUtils';

vi.mock('react-i18next', () => ({useTranslation: () => ({t: (_key: string, opts: {defaultValue: string}) => opts.defaultValue})}));
afterEach(() => {cleanup(); sessionStorage.clear();});
const request = (id = 'r1', kind = 'approval_request'): TranscriptBlock => ({type: 'request', message: {
    content: 'Bash', kind, request_id: id, payload: {command: 'first line\nsecond line'}, timestamp: '2026-10-04T00:00:00Z',
}});
const show = (respond: (id: string, approved: boolean, answer: string) => Promise<boolean>, block = request()) =>
    render(<Transcript blocks={[block]} pendingRequestId={block.type === 'request' ? block.message.request_id : undefined} working onRespond={respond}/>);

describe('Desk pending requests', () => {
    it('shows full approval input and never resubmits an accepted decision while polling catches up', async () => {
        let finish!: (accepted: boolean) => void;
        const respond = vi.fn(() => new Promise<boolean>((resolve) => {finish = resolve;}));
        show(respond);
        expect(screen.getByText(/second line/)).toBeInTheDocument();
        const allow = screen.getByRole('button', {name: 'Allow'});
        fireEvent.click(allow);
        fireEvent.click(allow);
        expect(respond).toHaveBeenCalledTimes(1);
        await act(async () => finish(true));
        expect(allow).toBeDisabled();
        expect(screen.getByRole('button', {name: 'Deny'})).toBeDisabled();
        fireEvent.click(allow);
        expect(respond).toHaveBeenCalledTimes(1);
    });

    it('keeps a failed question answer, allows retry and restores it after navigation', async () => {
        const respond = vi.fn().mockResolvedValueOnce(false).mockResolvedValueOnce(true);
        const view = show(respond, request('question', 'ask_request'));
        const answer = screen.getByRole('textbox', {name: 'Your answer'});
        expect(screen.getByRole('button', {name: 'Answer'})).toBeDisabled();
        expect(answer).not.toHaveFocus();
        fireEvent.change(answer, {target: {value: 'Keep this answer'}});
        await act(async () => fireEvent.keyDown(answer, {key: 'Enter', ctrlKey: true}));
        expect(answer).toHaveValue('Keep this answer');
        expect(screen.getByRole('alert')).toHaveTextContent('try again');
        view.unmount();
        show(respond, request('question', 'ask_request'));
        expect(screen.getByRole('textbox', {name: 'Your answer'})).toHaveValue('Keep this answer');
        await act(async () => fireEvent.click(screen.getByRole('button', {name: 'Answer'})));
        expect(respond).toHaveBeenLastCalledWith('question', true, 'Keep this answer');
        expect(JSON.parse(sessionStorage.getItem('desk.requestDraft:question')!).answer).toBe('');
    });

    it('does not carry a submitted decision into a new request in the same transcript slot', async () => {
        const respond = vi.fn().mockResolvedValue(true);
        const view = show(respond);
        await act(async () => fireEvent.click(screen.getByRole('button', {name: 'Allow'})));
        view.rerender(<Transcript blocks={[request('r2')]} pendingRequestId="r2" working onRespond={respond}/>);
        expect(screen.getByRole('button', {name: 'Allow'})).toBeEnabled();
    });
});


it('retains a pending and accepted decision when navigating away and back before the transcript updates', async () => {
    let finish!: (accepted: boolean) => void;
    const respond = vi.fn(() => new Promise<boolean>((resolve) => {finish = resolve;}));
    const first = show(respond);
    fireEvent.click(screen.getByRole('button', {name: 'Allow'}));
    first.unmount();
    show(respond);
    expect(screen.getByRole('button', {name: 'Allow'})).toBeDisabled();
    await act(async () => finish(true));
    expect(screen.getByRole('button', {name: 'Allow'})).toBeDisabled();
    expect(screen.getByRole('status')).toHaveTextContent('Response sent');
    expect(respond).toHaveBeenCalledOnce();
});
