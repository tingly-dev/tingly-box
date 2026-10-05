import {act, cleanup, fireEvent, render, screen, within, waitFor} from '@testing-library/react';
import type {ComponentProps} from 'react';
import {afterEach, describe, expect, it, vi} from 'vitest';
import SessionView from './SessionView';

vi.mock('react-i18next', () => ({useTranslation: () => ({t: (_key: string, opts: {defaultValue: string}) => opts.defaultValue})}));
vi.mock('./ProfileSelect', () => ({default: ({disabled, onChange}: {disabled: boolean; onChange: (value: string) => void}) => <button disabled={disabled} onClick={() => onChange('profile')}>Change profile</button>}));
vi.mock('./ModelSelect', () => ({default: ({disabled, onChange}: {disabled: boolean; onChange: (value: string) => void}) => <button disabled={disabled} onClick={() => onChange('model')}>Change model</button>}));
vi.mock('./PermissionModeSelect', () => ({default: ({disabled, onChange}: {disabled: boolean; onChange: (value: string) => void}) => <button disabled={disabled} onClick={() => onChange('mode')}>Change permissions</button>}));
vi.mock('./StatusLine', () => ({default: () => null}));
vi.mock('./Transcript', () => ({default: () => null}));
vi.mock('./BackgroundTasksPanel', () => ({default: () => null}));
afterEach(() => {cleanup(); sessionStorage.clear();});

const props = (): ComponentProps<typeof SessionView> => ({
    session: {id: 'a', status: 'completed', project: '/work', request: 'Test session', profile: '', model: '', permission_mode: '', background_tasks: [], awaiting_input: false, response: '', created_at: '', last_activity: ''},
    messages: [], permissionModes: [], draft: 'Saved draft', queued: [], onSend: vi.fn().mockResolvedValue(true), onRespond: vi.fn().mockResolvedValue(true),
    onInterrupt: vi.fn().mockResolvedValue(undefined), onArchive: vi.fn().mockResolvedValue(true), onPermissionModeChange: vi.fn().mockResolvedValue(undefined),
    onProfileChange: vi.fn().mockResolvedValue(undefined), onModelChange: vi.fn().mockResolvedValue(undefined), onUnqueue: vi.fn(), onSendQueuedNow: vi.fn(),
    onDraftAccepted: vi.fn(), onDraftChange: vi.fn(), onHandoff: vi.fn().mockResolvedValue(null), onRefresh: vi.fn(),
});

describe('Desk session actions', () => {
    it('serializes launch settings and disables sending until they are saved', async () => {
        let finish!: () => void;
        const config = props();
        config.onProfileChange = vi.fn(() => new Promise<void>((resolve) => {finish = resolve;}));
        render(<SessionView {...config}/>);
        fireEvent.click(screen.getByRole('button', {name: 'Change profile'}));
        expect(screen.getByRole('button', {name: 'Change model'})).toBeDisabled();
        expect(screen.getByRole('button', {name: 'Change permissions'})).toBeDisabled();
        expect(screen.getByRole('button', {name: 'Send'})).toBeDisabled();
        fireEvent.click(screen.getByRole('button', {name: 'Change model'}));
        expect(config.onModelChange).not.toHaveBeenCalled();
        await act(async () => finish());
        expect(screen.getByRole('button', {name: 'Change model'})).toBeEnabled();
        expect(screen.getByRole('button', {name: 'Send'})).toBeEnabled();
    });

    it('confirms archiving even an idle session, blocks duplicate requests and allows retry on failure', async () => {
        let finish!: (accepted: boolean) => void;
        const config = props();
        config.onArchive = vi.fn(() => new Promise<boolean>((resolve) => {finish = resolve;}));
        render(<SessionView {...config}/>);
        fireEvent.click(screen.getByRole('button', {name: 'Archive'}));
        const dialog = screen.getByRole('dialog', {name: 'Archive this session?'});
        expect(config.onArchive).not.toHaveBeenCalled();
        fireEvent.click(within(dialog).getByRole('button', {name: 'Archive'}));
        fireEvent.click(within(dialog).getByRole('button', {name: 'Working…'}));
        expect(config.onArchive).toHaveBeenCalledOnce();
        await act(async () => finish(false));
        expect(within(dialog).getByRole('button', {name: 'Archive'})).toBeEnabled();
        fireEvent.click(within(dialog).getByRole('button', {name: 'Archive'}));
        await act(async () => finish(true));
        expect(config.onArchive).toHaveBeenCalledTimes(2);
        await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    });

    it('keeps archived queued text and drafts accessible', () => {
        const config = props();
        config.session.status = 'closed';
        config.queued = ['Unsent follow-up'];
        render(<SessionView {...config}/>);
        expect(screen.getByText('Saved draft')).toBeInTheDocument();
        expect(screen.getByText('Unsent follow-up')).toBeInTheDocument();
        expect(screen.getByRole('button', {name: 'Copy'})).toBeInTheDocument();
        expect(screen.queryByRole('button', {name: 'Send now'})).not.toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', {name: 'Take back into the input'}));
        expect(config.onUnqueue).toHaveBeenCalledWith(0);
    });
});
