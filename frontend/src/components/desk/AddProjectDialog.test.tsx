import {cleanup, fireEvent, render, screen} from '@testing-library/react';
import {afterEach, expect, it, vi} from 'vitest';
import AddProjectDialog from './AddProjectDialog';

vi.mock('react-i18next', () => ({useTranslation: () => ({t: (_key: string, opts: {defaultValue: string}) => opts.defaultValue})}));
afterEach(cleanup);

it('blocks relative paths and adds a normalized absolute path on form submission', () => {
    const add = vi.fn().mockReturnValue(true);
    render(<AddProjectDialog onAdd={add} onClose={vi.fn()}/>);
    const button = screen.getByRole('button', {name: 'Add project'});
    expect(button).toBeDisabled();
    fireEvent.change(screen.getByLabelText('Project directory'), {target: {value: '../app'}});
    expect(button).toBeDisabled();
    expect(screen.getByText(/Enter a full absolute path/)).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText('Project directory'), {target: {value: ' /projects/new app/ '}});
    fireEvent.submit(button.closest('form')!);
    expect(add).toHaveBeenCalledExactlyOnceWith('/projects/new app');
});

it('keeps the dialog and path when saving fails and permits retry', () => {
    const add = vi.fn().mockReturnValueOnce(false).mockReturnValueOnce(true);
    render(<AddProjectDialog onAdd={add} onClose={vi.fn()}/>);
    fireEvent.change(screen.getByLabelText('Project directory'), {target: {value: '/projects/app'}});
    fireEvent.click(screen.getByRole('button', {name: 'Add project'}));
    expect(screen.getByText(/Could not save the project/)).toBeInTheDocument();
    expect(screen.getByLabelText('Project directory')).toHaveValue('/projects/app');
    fireEvent.click(screen.getByRole('button', {name: 'Add project'}));
    expect(add).toHaveBeenCalledTimes(2);
});
