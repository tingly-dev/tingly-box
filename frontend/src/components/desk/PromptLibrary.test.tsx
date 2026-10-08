import {act, cleanup, fireEvent, render, screen} from '@testing-library/react';
import {afterEach, describe, expect, it, vi} from 'vitest';
import NewSessionView from './NewSessionView';
import {PROMPTS_KEY} from './useDeskPrompts';

vi.mock('react-i18next', () => ({useTranslation: () => ({t: (_key: string, opts: {defaultValue: string}) => opts.defaultValue})}));
vi.mock('./FolderPicker', () => ({default: ({value}: {value: string}) => <input aria-label="Folder" value={value} readOnly/>}));
vi.mock('./ProfileSelect', () => ({default: () => null}));
vi.mock('./ModelSelect', () => ({default: () => null}));
vi.mock('./PermissionModeSelect', () => ({default: () => null}));
afterEach(() => {cleanup(); sessionStorage.clear(); localStorage.clear();});

const open = (folder = '/p/one', create = vi.fn().mockResolvedValue(true)) => {
    render(<NewSessionView initialFolder={folder} recentFolders={[]} permissionModes={[]} onCreate={create}/>);
    return create;
};

describe('Prompt library', () => {
    it('runs a built-in prompt in the chosen folder with one click', () => {
        const create = open();
        fireEvent.click(screen.getAllByRole('button', {name: 'Run'})[0]);
        expect(create).toHaveBeenCalledWith('/p/one', expect.stringContaining('Review the uncommitted changes'), '', '', '');
    });

    it('disables Run until a folder is chosen', () => {
        open('');
        expect(screen.getAllByRole('button', {name: 'Run'})[0]).toBeDisabled();
    });

    it('Edit first puts the text in the composer instead of starting', () => {
        const create = open();
        fireEvent.click(screen.getAllByRole('button', {name: 'Edit first'})[0]);
        expect(screen.getByPlaceholderText('Describe a task…')).toHaveDisplayValue(/Review the uncommitted changes/);
        expect(create).not.toHaveBeenCalled();
    });

    it('saves the draft to the library and keeps it after remount', () => {
        open();
        fireEvent.change(screen.getByPlaceholderText('Describe a task…'), {target: {value: 'check the logs\nand summarize errors'}});
        act(() => {fireEvent.click(screen.getByRole('button', {name: 'Save to library'}));});
        expect(Object.values(JSON.parse(localStorage.getItem(PROMPTS_KEY)!))).toMatchObject([{name: 'check the logs', text: 'check the logs\nand summarize errors'}]);
        cleanup();
        sessionStorage.clear(); // drop the unsent draft; only the library should remember it
        open();
        expect(screen.getByText('check the logs', {selector: 'p'})).toBeInTheDocument();
    });
});
