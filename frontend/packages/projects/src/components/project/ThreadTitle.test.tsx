import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ThreadTitle } from './ThreadTitle';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

afterEach(cleanup);

const title = (text: string, onSave: (t: string) => Promise<boolean>) => render(<ThreadTitle title={text} onSave={onSave} />);

describe('ThreadTitle', () => {
    it('edits in place: Enter saves the trimmed title', async () => {
        const onSave = vi.fn(async () => true);
        title('Fix login', onSave);
        fireEvent.click(screen.getByRole('button', { name: 'thread.editTitle' }));
        const field = screen.getByRole('textbox', { name: 'thread.editTitle' }) as HTMLInputElement;
        expect(field.value).toBe('Fix login');
        fireEvent.change(field, { target: { value: '  Fix the login page  ' } });
        fireEvent.keyDown(field, { key: 'Enter' });
        await waitFor(() => expect(onSave).toHaveBeenCalledWith('Fix the login page'));
        await waitFor(() => expect(screen.getByText('Fix the login page')).toBeTruthy());
    });

    it('Escape cancels and keeps the old title', () => {
        const onSave = vi.fn(async () => true);
        title('Fix login', onSave);
        fireEvent.click(screen.getByRole('button', { name: 'thread.editTitle' }));
        const field = screen.getByRole('textbox', { name: 'thread.editTitle' });
        fireEvent.change(field, { target: { value: 'Renamed' } });
        fireEvent.keyDown(field, { key: 'Escape' });
        expect(screen.getByText('Fix login')).toBeTruthy();
        fireEvent.click(screen.getByRole('button', { name: 'thread.editTitle' }));
        // The edit starts from the kept title, not the abandoned one.
        expect((screen.getByRole('textbox', { name: 'thread.editTitle' }) as HTMLInputElement).value).toBe('Fix login');
        expect(onSave).not.toHaveBeenCalled();
    });

    it('clicking away saves, an empty title does not', () => {
        const onSave = vi.fn(async () => true);
        const { unmount } = title('Fix login', onSave);
        fireEvent.click(screen.getByRole('button', { name: 'thread.editTitle' }));
        const field = screen.getByRole('textbox', { name: 'thread.editTitle' });
        fireEvent.change(field, { target: { value: '   ' } });
        fireEvent.blur(field);
        expect(onSave).not.toHaveBeenCalled();
        expect(screen.getByText('Fix login')).toBeTruthy();
        unmount();
        onSave.mockClear();
        const second = title('Fix login', onSave);
        fireEvent.click(screen.getByRole('button', { name: 'thread.editTitle' }));
        fireEvent.change(screen.getByRole('textbox', { name: 'thread.editTitle' }), { target: { value: 'Renamed by blur' } });
        fireEvent.blur(screen.getByRole('textbox', { name: 'thread.editTitle' }));
        expect(onSave).toHaveBeenCalledWith('Renamed by blur');
        second.unmount();
    });

    it('a failed save keeps the old title', async () => {
        const onSave = vi.fn(async () => false);
        title('Fix login', onSave);
        fireEvent.click(screen.getByRole('button', { name: 'thread.editTitle' }));
        const field = screen.getByRole('textbox', { name: 'thread.editTitle' });
        fireEvent.change(field, { target: { value: 'Renamed' } });
        fireEvent.keyDown(field, { key: 'Enter' });
        await waitFor(() => expect(onSave).toHaveBeenCalled());
        expect(screen.getByText('Fix login')).toBeTruthy();
    });

    it('follows the thread when its title changes from elsewhere', () => {
        const onSave = vi.fn(async () => true);
        const { rerender } = title('Fix login', onSave);
        rerender(<ThreadTitle title="Renamed upstream" onSave={onSave} />);
        expect(screen.getByText('Renamed upstream')).toBeTruthy();
    });
});
