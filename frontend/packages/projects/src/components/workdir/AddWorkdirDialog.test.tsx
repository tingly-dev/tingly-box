import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { AddWorkdirDialog } from './AddWorkdirDialog';

const m = vi.hoisted(() => ({ post: vi.fn(), unwrap: vi.fn() }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock('../../client', () => ({ api: { POST: m.post }, unwrap: m.unwrap }));
vi.mock('../common/DirectoryPicker', () => ({
    DirectoryPicker: ({ onClose, onSelect }: { onClose: () => void; onSelect: (p: string) => void }) => (
        <div><button onClick={() => onSelect('/work/repo')}>pick</button><button onClick={onClose}>dismiss</button></div>
    ),
}));
afterEach(() => { cleanup(); vi.resetAllMocks(); });

it('adds the project as soon as a directory is picked', async () => {
    m.unwrap.mockResolvedValue({ path: '/work/repo' });
    const added = vi.fn();
    render(<AddWorkdirDialog onClose={vi.fn()} onAdded={added} />);
    fireEvent.click(screen.getByText('directory.browse'));
    fireEvent.click(screen.getByText('pick'));
    await waitFor(() => expect(added).toHaveBeenCalledWith({ path: '/work/repo' }));
    expect(m.post).toHaveBeenCalledWith('/api/v1/projects', { body: { type: 'folder', path: '/work/repo' } });
});

it('adds nothing when the picker is dismissed', () => {
    render(<AddWorkdirDialog onClose={vi.fn()} onAdded={vi.fn()} />);
    fireEvent.click(screen.getByText('directory.browse'));
    fireEvent.click(screen.getByText('dismiss'));
    expect(m.post).not.toHaveBeenCalled();
});

it('shows the server error for a picked directory and allows retrying', async () => {
    m.unwrap.mockRejectedValueOnce(new Error('already added')).mockResolvedValueOnce({ path: '/work/repo' });
    const added = vi.fn();
    render(<AddWorkdirDialog onClose={vi.fn()} onAdded={added} />);
    fireEvent.click(screen.getByText('directory.browse'));
    fireEvent.click(screen.getByText('pick'));
    expect(await screen.findByText('already added')).toBeInTheDocument();
    expect(added).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'workdir.add' }));
    await waitFor(() => expect(added).toHaveBeenCalled());
});

it('still adds a typed path with the Add button', async () => {
    m.unwrap.mockResolvedValue({ path: '/typed' });
    const added = vi.fn();
    render(<AddWorkdirDialog onClose={vi.fn()} onAdded={added} />);
    fireEvent.change(screen.getByRole('textbox'), { target: { value: ' /typed ' } });
    fireEvent.click(screen.getByRole('button', { name: 'workdir.add' }));
    await waitFor(() => expect(added).toHaveBeenCalled());
    expect(m.post).toHaveBeenCalledWith('/api/v1/projects', { body: { type: 'folder', path: '/typed' } });
});
