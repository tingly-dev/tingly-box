import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { NewSessionView } from './NewSessionView';
import { ProjectsHostProvider } from '../../host/ProjectsHost';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock('../ModelPicker', () => ({ ModelPicker: (p: { value: string; disabled?: boolean; onChange: (v: string) => void }) => <button disabled={p.disabled} onClick={() => p.onChange('chosen-model')}>{p.value || 'choose-model'}</button> }));
const host = { paths: { list: '/', project: (id: string) => `/${id}`, folder: () => '/folder' } };
afterEach(cleanup);

it('submits selected settings, locks pickers during creation and retains draft and choices on failure', async () => {
    let finish!: (value: boolean) => void;
    const create = vi.fn(() => new Promise<boolean>((r) => { finish = r; }));
    render(<ProjectsHostProvider host={host}><NewSessionView initialFolder="/repo" folders={['/repo']} permissionModes={['plan']} onCreate={create} onAttach={vi.fn()} /></ProjectsHostProvider>);
    fireEvent.click(screen.getByRole('button', { name: 'choose-model' }));
    fireEvent.click(screen.getByRole('button', { name: 'effort.default' }));
    fireEvent.click(screen.getByRole('menuitem', { name: /effort.high/ }));
    fireEvent.mouseDown(screen.getByRole('combobox'));
    fireEvent.click(screen.getByRole('option', { name: 'session.mode.plan' }));
    const input = screen.getByPlaceholderText('session.promptPlaceholder');
    fireEvent.change(input, { target: { value: 'task' } });
    fireEvent.keyDown(input, { key: 'Enter' });
    expect(create).toHaveBeenCalledWith('/repo', 'task', 'plan', { profile: '', model: 'chosen-model' }, [], 'high');
    expect(screen.getByRole('button', { name: 'chosen-model' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'effort.high' })).toBeDisabled();
    expect(screen.getByRole('button', { name: /session.folder:/ })).toBeDisabled();
    expect(screen.getByRole('combobox')).toHaveAttribute('aria-disabled', 'true');
    expect(input).not.toBeDisabled();
    fireEvent.change(input, { target: { value: 'edited task' } });
    fireEvent.keyDown(input, { key: 'Enter' });
    expect(create).toHaveBeenCalledOnce();
    await act(async () => finish(false));
    expect(input).toHaveValue('edited task');
    expect(screen.getByRole('button', { name: 'chosen-model' })).not.toBeDisabled();
    fireEvent.keyDown(input, { key: 'Enter' });
    expect(create).toHaveBeenLastCalledWith('/repo', 'edited task', 'plan', { profile: '', model: 'chosen-model' }, [], 'high');
    await act(async () => finish(true));
    expect(input).toHaveValue('');
});

describe('new folder session attachments', () => {
    it('keeps the original owner if the folder list changes during upload', async () => {
        let finish!: (value: { name: string; path: string; type: string; size: number }[]) => void;
        const attach = vi.fn(() => new Promise<{ name: string; path: string; type: string; size: number }[]>((resolve) => { finish = resolve; }));
        const create = vi.fn().mockResolvedValue(false);
        const props = { permissionModes: [], onCreate: create, onAttach: attach };
        const { container, rerender } = render(<ProjectsHostProvider host={host}><NewSessionView {...props} folders={['/one', '/two']} /></ProjectsHostProvider>);
        fireEvent.change(container.querySelector('input[type="file"]')!, { target: { files: [new File(['x'], 'note.txt')] } });
        rerender(<ProjectsHostProvider host={host}><NewSessionView {...props} folders={['/two', '/one']} /></ProjectsHostProvider>);
        expect(screen.getByRole('button', { name: 'session.folder: one' })).toBeDisabled();
        await act(async () => finish([{ name: 'note.txt', path: 'uploads/123456789abc-note.txt', type: 'text/plain', size: 1 }]));
        await waitFor(() => expect(screen.getByText('note.txt')).toBeInTheDocument());
        fireEvent.keyDown(screen.getByPlaceholderText('session.promptPlaceholder'), { key: 'Enter' });
        await waitFor(() => expect(create).toHaveBeenCalledWith('/one', '', '', expect.anything(), expect.any(Array), ''));
    });

    it('unlocks the folder after a failed upload without losing the draft', async () => {
        const attach = vi.fn().mockRejectedValue(new Error('upload failed'));
        const { container } = render(<ProjectsHostProvider host={host}><NewSessionView initialFolder="/one" folders={['/one', '/two']} permissionModes={[]} onCreate={vi.fn()} onAttach={attach} /></ProjectsHostProvider>);
        const prompt = screen.getByPlaceholderText('session.promptPlaceholder');
        fireEvent.change(prompt, { target: { value: 'draft' } });
        fireEvent.change(container.querySelector('input[type="file"]')!, { target: { files: [new File(['x'], 'note.txt')] } });
        await screen.findByText('upload failed');
        expect(screen.getByRole('button', { name: 'session.folder: one' })).toBeEnabled();
        expect(prompt).toHaveValue('draft');
    });

    it('keeps the folder fixed while an upload is pending and until its attachment is removed', async () => {
        let finish!: (value: { name: string; path: string; type: string; size: number }[]) => void;
        const attach = vi.fn(() => new Promise<{ name: string; path: string; type: string; size: number }[]>((resolve) => { finish = resolve; }));
        const { container } = render(<ProjectsHostProvider host={host}><NewSessionView initialFolder="/one" folders={['/one', '/two']} permissionModes={[]} onCreate={vi.fn()} onAttach={attach} /></ProjectsHostProvider>);
        const input = container.querySelector('input[type="file"]')!;
        fireEvent.change(input, { target: { files: [new File(['content'], 'note.txt', { type: 'text/plain' })] } });
        expect(attach).toHaveBeenCalledWith('/one', expect.any(Array));
        expect(screen.getByRole('button', { name: 'session.folder: one' })).toBeDisabled();
        finish([{ name: 'note.txt', path: 'uploads/123456789abc-note.txt', type: 'text/plain', size: 7 }]);
        await waitFor(() => expect(screen.getByText('note.txt')).toBeInTheDocument());
        expect(screen.getByRole('button', { name: 'session.folder: one' })).toBeDisabled();
        fireEvent.click(screen.getByRole('button', { name: 'composer.removeAttachment' }));
        expect(screen.getByRole('button', { name: 'session.folder: one' })).toBeEnabled();
    });
});
