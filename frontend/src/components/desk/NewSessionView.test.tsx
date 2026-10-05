import {act, cleanup, fireEvent, render, screen} from '@testing-library/react';
import {afterEach, describe, expect, it, vi} from 'vitest';
import NewSessionView from './NewSessionView';

vi.mock('react-i18next', () => ({useTranslation: () => ({t: (_key: string, opts: {defaultValue: string}) => opts.defaultValue})}));
vi.mock('./FolderPicker', () => ({default: ({value, onChange}: {value: string; onChange: (path: string) => void}) => (
    <input aria-label="Folder" value={value} onChange={(e) => onChange(e.target.value)}/>
)}));
vi.mock('./ProfileSelect', () => ({default: ({value, onChange}: {value: string; onChange: (profile: string) => void}) => (
    <input aria-label="Profile" value={value} onChange={(e) => onChange(e.target.value)}/>
)}));
vi.mock('./ModelSelect', () => ({default: ({value, onChange}: {value: string; onChange: (model: string) => void}) => (
    <input aria-label="Model" value={value} onChange={(e) => onChange(e.target.value)}/>
)}));
vi.mock('./PermissionModeSelect', () => ({default: ({value, onChange}: {value: string; onChange: (mode: string) => void}) => (
    <input aria-label="Permissions" value={value} onChange={(e) => onChange(e.target.value)}/>
)}));
afterEach(() => { cleanup(); sessionStorage.clear(); });
const open = (folder = '/projects/one', create = vi.fn().mockResolvedValue(true)) => render(
    <NewSessionView initialFolder={folder} recentFolders={[]} permissionModes={[]} onCreate={create}/>,
);

describe('New local task drafts', () => {
    it('restores the prompt together with its folder and launch settings', () => {
        const first = open();
        fireEvent.change(screen.getByLabelText('Folder'), {target: {value: '/projects/edited'}});
        fireEvent.change(screen.getByLabelText('Profile'), {target: {value: 'work'}});
        fireEvent.change(screen.getByLabelText('Model'), {target: {value: 'sonnet'}});
        fireEvent.change(screen.getByLabelText('Permissions'), {target: {value: 'plan'}});
        fireEvent.change(screen.getByPlaceholderText('Describe a task…'), {target: {value: 'review this project'}});
        first.unmount();
        open();
        expect(screen.getByLabelText('Folder')).toHaveValue('/projects/edited');
        expect(screen.getByLabelText('Profile')).toHaveValue('work');
        expect(screen.getByLabelText('Model')).toHaveValue('sonnet');
        expect(screen.getByLabelText('Permissions')).toHaveValue('plan');
        expect(screen.getByPlaceholderText('Describe a task…')).toHaveValue('review this project');
    });

    it('keeps folder drafts independent and clears a tier when changing profiles', () => {
        const first = open();
        fireEvent.change(screen.getByLabelText('Model'), {target: {value: 'opus'}});
        fireEvent.change(screen.getByLabelText('Profile'), {target: {value: 'another'}});
        expect(screen.getByLabelText('Model')).toHaveValue('');
        fireEvent.change(screen.getByPlaceholderText('Describe a task…'), {target: {value: 'task one'}});
        first.unmount();
        open('/projects/two');
        expect(screen.getByLabelText('Folder')).toHaveValue('/projects/two');
        expect(screen.getByPlaceholderText('Describe a task…')).toHaveValue('');
    });

    it('removes an accepted prompt from saved drafts while keeping the chosen folder', async () => {
        const create = vi.fn().mockResolvedValue(true);
        const first = open('/projects/one', create);
        fireEvent.change(screen.getByPlaceholderText('Describe a task…'), {target: {value: 'task'}});
        await act(async () => fireEvent.click(screen.getByRole('button', {name: 'Send'})));
        expect(create).toHaveBeenCalledWith('/projects/one', 'task', '', '', '');
        first.unmount();
        open();
        expect(screen.getByPlaceholderText('Describe a task…')).toHaveValue('');
        expect(screen.getByLabelText('Folder')).toHaveValue('/projects/one');
    });
});


it('does not clear a newer draft when an accepted start finishes after leaving and returning', async () => {
    let finish!: (accepted: boolean) => void;
    const create = vi.fn(() => new Promise<boolean>((resolve) => {finish = resolve;}));
    const first = open('/projects/one', create);
    fireEvent.change(screen.getByPlaceholderText('Describe a task…'), {target: {value: 'original task'}});
    fireEvent.click(screen.getByRole('button', {name: 'Send'}));
    first.unmount();
    open('/projects/one');
    fireEvent.change(screen.getByPlaceholderText('Describe a task…'), {target: {value: 'next task after returning'}});
    await act(async () => finish(true));
    expect(screen.getByPlaceholderText('Describe a task…')).toHaveValue('next task after returning');
    expect(JSON.parse(sessionStorage.getItem('desk.newDraft:/projects/one')!).prompt).toBe('next task after returning');
});
