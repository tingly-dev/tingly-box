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
