import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { DirectoryPicker } from './DirectoryPicker';

const m = vi.hoisted(() => ({ fetch: vi.fn(), t: (key: string) => key }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: m.t }) }));
vi.mock('../../client', () => ({ apiFetch: m.fetch }));
afterEach(() => { cleanup(); vi.resetAllMocks(); });

it('starts at home, navigates and returns the selected directory', async () => {
    m.fetch.mockResolvedValueOnce(new Response(JSON.stringify({ home: '/home/me', path: '.', directories: ['work'] })))
        .mockResolvedValueOnce(new Response(JSON.stringify({ home: '/home/me', path: 'work', directories: [] })));
    const select = vi.fn();
    render(<DirectoryPicker onClose={vi.fn()} onSelect={select} />);
    fireEvent.click(await screen.findByText('work'));
    await screen.findByText('/home/me/work');
    fireEvent.click(screen.getByRole('button', { name: 'directory.select' }));
    expect(select).toHaveBeenCalledWith('/home/me/work');
    expect(JSON.parse(m.fetch.mock.calls[0][1].body)).toEqual({ path: '.' });
});

it('explains a rejected request and allows retrying', async () => {
    m.fetch.mockResolvedValueOnce(new Response('', { status: 403 }))
        .mockResolvedValueOnce(new Response(JSON.stringify({ home: '/home/me', path: '.', directories: [] })));
    render(<DirectoryPicker onClose={vi.fn()} onSelect={vi.fn()} />);
    expect(await screen.findByText('directory.blocked')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'directory.retry' }));
    await waitFor(() => expect(screen.getByText('/home/me')).toBeInTheDocument());
});

it('returns home after a directory becomes unavailable', async () => {
    m.fetch.mockResolvedValueOnce(new Response(JSON.stringify({ home: '/home/me', path: '.', directories: ['work'] })))
        .mockResolvedValueOnce(new Response('', { status: 400 }))
        .mockResolvedValueOnce(new Response(JSON.stringify({ home: '/home/me', path: '.', directories: ['work'] })));
    render(<DirectoryPicker onClose={vi.fn()} onSelect={vi.fn()} />);
    expect(screen.getByRole('button', { name: 'directory.parent' })).toBeDisabled();
    fireEvent.click(await screen.findByText('work'));
    await screen.findByText('directory.failed');
    expect(screen.getByRole('button', { name: 'directory.select' })).toBeDisabled();
    fireEvent.click(screen.getAllByRole('button', { name: 'directory.home' })[0]);
    await screen.findByText('/home/me');
    expect(JSON.parse(m.fetch.mock.calls[2][1].body)).toEqual({ path: '.' });
});

it('goes to the parent directory and preserves backslashes in Unix names', async () => {
    m.fetch.mockResolvedValueOnce(new Response(JSON.stringify({ home: '/home/me', path: '.', directories: ['a\\b'] })))
        .mockResolvedValueOnce(new Response(JSON.stringify({ home: '/home/me', path: 'a\\b', directories: [] })))
        .mockResolvedValueOnce(new Response(JSON.stringify({ home: '/home/me', path: '.', directories: ['a\\b'] })));
    const select = vi.fn();
    render(<DirectoryPicker onClose={vi.fn()} onSelect={select} />);
    fireEvent.click(await screen.findByText('a\\b'));
    await screen.findByText('/home/me/a\\b');
    fireEvent.click(screen.getByRole('button', { name: 'directory.select' }));
    expect(select).toHaveBeenCalledWith('/home/me/a\\b');
    fireEvent.click(screen.getByRole('button', { name: 'directory.parent' }));
    await screen.findByText('/home/me');
});

it('returns a Windows path and lets a breadcrumb navigate to an ancestor', async () => {
    m.fetch.mockResolvedValueOnce(new Response(JSON.stringify({ home: 'C:\\Users\\me', path: '.', directories: ['work'] })))
        .mockResolvedValueOnce(new Response(JSON.stringify({ home: 'C:\\Users\\me', path: 'work', directories: ['app'] })))
        .mockResolvedValueOnce(new Response(JSON.stringify({ home: 'C:\\Users\\me', path: 'work\\app', directories: [] })))
        .mockResolvedValueOnce(new Response(JSON.stringify({ home: 'C:\\Users\\me', path: 'work', directories: ['app'] })));
    const select = vi.fn();
    render(<DirectoryPicker onClose={vi.fn()} onSelect={select} />);
    fireEvent.click(await screen.findByText('work'));
    fireEvent.click(await screen.findByText('app'));
    await screen.findByText('C:\\Users\\me\\work\\app');
    fireEvent.click(screen.getByRole('button', { name: 'directory.select' }));
    expect(select).toHaveBeenCalledWith('C:\\Users\\me\\work\\app');
    fireEvent.click(screen.getByRole('button', { name: 'work' }));
    await screen.findByText('C:\\Users\\me\\work');
    expect(JSON.parse(m.fetch.mock.calls[3][1].body)).toEqual({ path: 'work' });
});

it('ignores a late response after returning home', async () => {
    let finish!: (response: Response) => void;
    m.fetch.mockResolvedValueOnce(new Response(JSON.stringify({ home: '/home/me', path: '.', directories: ['work'] })))
        .mockImplementationOnce(() => new Promise<Response>((resolve) => { finish = resolve; }))
        .mockResolvedValueOnce(new Response(JSON.stringify({ home: '/home/me', path: '.', directories: [] })));
    render(<DirectoryPicker onClose={vi.fn()} onSelect={vi.fn()} />);
    fireEvent.click(await screen.findByText('work'));
    fireEvent.click(screen.getAllByRole('button', { name: 'directory.home' })[0]);
    await screen.findByText('/home/me');
    finish(new Response(JSON.stringify({ home: '/home/me', path: 'work', directories: ['stale'] })));
    await waitFor(() => expect(screen.queryByText('stale')).not.toBeInTheDocument());
});
