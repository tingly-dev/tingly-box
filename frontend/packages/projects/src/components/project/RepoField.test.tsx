import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { RepoField, type RepoEntry } from './RepoField';

const m = vi.hoisted(() => ({ post: vi.fn() }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en' } }) }));
vi.mock('../../client', async (actual) => ({ ...await actual<typeof import('../../client')>(), api: { POST: m.post } }));
// RepoField drives the picker only through onSelect; the real one is covered by its own tests.
vi.mock('../common/DirectoryPicker', () => ({
    DirectoryPicker: ({ onSelect }: { onSelect: (p: string) => void }) => (
        <button onClick={() => onSelect('/picked/repo')}>stub-pick</button>
    ),
}));

afterEach(() => { cleanup(); vi.clearAllMocks(); });

const inspectOk = () => m.post.mockResolvedValue({ data: { ok: true, root: '/picked/repo', branch: 'main', dirty: false }, response: new Response() });

const renderField = (value: RepoEntry[] = []) => {
    const change = vi.fn();
    render(<RepoField value={value} onChange={change} />);
    const input = screen.getByPlaceholderText('repo.placeholder') as HTMLInputElement;
    return { change, input, browse: () => fireEvent.click(screen.getByRole('button', { name: 'directory.browse' })) };
};

it('adds the repo directly on a directory pick, without pressing Add', async () => {
    inspectOk();
    const { change, input, browse } = renderField();
    browse();
    fireEvent.click(screen.getByRole('button', { name: 'stub-pick' }));
    await waitFor(() => expect(m.post).toHaveBeenCalledWith('/api/v1/projects/repos/inspect', { body: { path: '/picked/repo' } }));
    await waitFor(() => expect(change).toHaveBeenCalledWith([{ root: '/picked/repo', branch: 'main', dirty: false }]));
    // The picked path never sits in the field: a successful pick leaves it empty.
    expect(input.value).toBe('');
    expect(screen.getByRole('button', { name: 'repo.add' })).toBeDisabled();
});

it('keeps the Add button for a typed path', async () => {
    inspectOk();
    const { change, input } = renderField();
    fireEvent.change(input, { target: { value: '/typed/repo' } });
    fireEvent.click(screen.getByRole('button', { name: 'repo.add' }));
    await waitFor(() => expect(m.post).toHaveBeenCalledWith('/api/v1/projects/repos/inspect', { body: { path: '/typed/repo' } }));
    await waitFor(() => expect(change).toHaveBeenCalledWith([{ root: '/picked/repo', branch: 'main', dirty: false }]));
    expect(input.value).toBe('');
});

it('on a failed inspect the picked path stays in the field with the error', async () => {
    m.post.mockResolvedValue({ data: { ok: false, error: 'not a git repository' }, response: new Response() });
    const { change, input, browse } = renderField();
    browse();
    fireEvent.click(screen.getByRole('button', { name: 'stub-pick' }));
    await waitFor(() => expect(input.value).toBe('/picked/repo'));
    expect(screen.getByText('not a git repository')).toBeTruthy();
    expect(change).not.toHaveBeenCalled();
});
