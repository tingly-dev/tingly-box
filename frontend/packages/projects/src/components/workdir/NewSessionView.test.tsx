import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { NewSessionView } from './NewSessionView';
import { ProjectsHostProvider } from '../../host/ProjectsHost';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock('../ModelPicker', () => ({ ModelPicker: (p: { value: string; disabled?: boolean; onChange: (v: string) => void }) => <button disabled={p.disabled} onClick={() => p.onChange('chosen-model')}>{p.value || 'choose-model'}</button> }));
const host = { paths: { list: '/', project: (id: string) => `/${id}`, folder: () => '/folder' } };
afterEach(cleanup);
// The prompt is a ThreadComposer draft now; keep runs independent of it.
beforeEach(() => localStorage.clear());

it('submits selected settings, locks pickers during creation and retains draft and choices on failure', async () => {
    let finish!: (value: boolean) => void;
    const create = vi.fn(() => new Promise<boolean>((r) => { finish = r; }));
    render(<ProjectsHostProvider host={host}><NewSessionView initialFolder="/repo" folders={['/repo']} permissionModes={['plan']} onCreate={create} onAttach={vi.fn()} /></ProjectsHostProvider>);
    fireEvent.click(screen.getByRole('button', { name: 'choose-model' }));
    fireEvent.mouseDown(screen.getByRole('combobox'));
    fireEvent.click(screen.getByRole('option', { name: 'session.mode.plan' }));
    const input = screen.getByPlaceholderText('session.promptPlaceholder');
    fireEvent.change(input, { target: { value: 'task' } });
    fireEvent.keyDown(input, { key: 'Enter' });
    expect(create).toHaveBeenCalledWith('/repo', 'task', 'plan', { profile: '', model: 'chosen-model' }, []);
    expect(screen.getByRole('button', { name: 'chosen-model' })).toBeDisabled();
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
    expect(create).toHaveBeenLastCalledWith('/repo', 'edited task', 'plan', { profile: '', model: 'chosen-model' }, []);
    await act(async () => finish(true));
    expect(input).toHaveValue('');
});

it('keeps the typed prompt across a remount, like every draft', () => {
    const first = render(<ProjectsHostProvider host={host}><NewSessionView initialFolder="/repo" folders={['/repo']} permissionModes={['plan']} onCreate={vi.fn()} onAttach={vi.fn()} /></ProjectsHostProvider>);
    fireEvent.change(screen.getByPlaceholderText('session.promptPlaceholder'), { target: { value: 'half typed' } });
    first.unmount();
    render(<ProjectsHostProvider host={host}><NewSessionView initialFolder="/other" folders={['/repo']} permissionModes={['plan']} onCreate={vi.fn()} onAttach={vi.fn()} /></ProjectsHostProvider>);
    expect(screen.getByPlaceholderText('session.promptPlaceholder')).toHaveValue('half typed');
});
