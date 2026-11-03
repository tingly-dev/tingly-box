import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { ThreadSettingsPickers } from './ThreadSettingsPickers';
import { ProjectsHostProvider } from '../host/ProjectsHost';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock('./ModelPicker', () => ({ ModelPicker: (p: { value: string; defaultLabel: string; source: string; disabled?: boolean; onChange: (v: string) => void }) => <button disabled={p.disabled} onClick={() => p.onChange('custom')} data-source={p.source}>{p.value || p.defaultLabel}</button> }));
const host = { paths: { list: '/', project: (id: string) => `/${id}`, folder: () => '/folder' } };
afterEach(cleanup);

it('shows worker inheritance and never uses folder profile slots for a project worker', () => {
    const slot = vi.fn(() => <span>folder slot</span>);
    const launch = vi.fn();
    render(<ProjectsHostProvider host={{ ...host, session: { launchPickers: slot } }}>
        <ThreadSettingsPickers value={{ profile: '', model: '' }} permissionMode="" modes={['plan']} defaultModel="project-model" defaultPermissionMode="auto" onLaunchChange={launch} onPermissionChange={vi.fn()} />
    </ProjectsHostProvider>);
    expect(slot).not.toHaveBeenCalled();
    const model = screen.getByRole('button', { name: 'threadSettings.projectDefault · project-model' });
    expect(model).toHaveAttribute('data-source', 'projects');
    fireEvent.click(model);
    expect(launch).toHaveBeenCalledWith({ profile: '', model: 'custom' });
    expect(screen.getByRole('combobox')).toHaveTextContent('session.mode.auto');
});

it('uses host slots only for folders and passes their disabled state', () => {
    const slot = vi.fn(() => <span>folder slot</span>);
    render(<ProjectsHostProvider host={{ ...host, session: { launchPickers: slot } }}>
        <ThreadSettingsPickers folder disabled value={{ profile: 'work', model: 'opus' }} permissionMode="plan" modes={['plan']} onLaunchChange={vi.fn()} onPermissionChange={vi.fn()} />
    </ProjectsHostProvider>);
    expect(slot).toHaveBeenCalledWith(expect.objectContaining({ value: { profile: 'work', model: 'opus' }, disabled: true }));
    expect(screen.getByRole('combobox')).toHaveAttribute('aria-disabled', 'true');
});

it('provides a model picker for standalone folder threads and preserves their profile', () => {
    const launch = vi.fn();
    render(<ProjectsHostProvider host={host}>
        <ThreadSettingsPickers folder value={{ profile: 'saved', model: '' }} permissionMode="" modes={['plan', 'dontAsk']} onLaunchChange={launch} onPermissionChange={vi.fn()} />
    </ProjectsHostProvider>);
    fireEvent.click(screen.getByRole('button', { name: 'threadSettings.defaultModel' }));
    expect(launch).toHaveBeenCalledWith({ profile: 'saved', model: 'custom' });
    fireEvent.mouseDown(screen.getByRole('combobox'));
    expect(screen.getByRole('option', { name: 'session.mode.dontAsk' })).toBeInTheDocument();
    expect(screen.queryByRole('option', { name: 'session.mode.auto' })).not.toBeInTheDocument();
});
