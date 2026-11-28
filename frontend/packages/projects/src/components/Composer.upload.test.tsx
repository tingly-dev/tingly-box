import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { Composer } from './Composer';
import { MAX_UPLOAD_BYTES } from '../utils/uploadLimit';

// The real message, so the test can read the file, the limit and how far over
// it is — not just that some alert appeared.
vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, opts?: Record<string, unknown>) => key === 'composer.uploadTooLarge'
            ? `${opts?.name} is ${opts?.size}, over the ${opts?.limit} limit by ${opts?.over}`
            : ({ 'composer.sendShort': 'Send', 'composer.stop': 'Stop' } as Record<string, string>)[key] ?? key,
    }),
}));
const { mediaQuery } = vi.hoisted(() => ({ mediaQuery: { mobile: false } }));
vi.mock('@mui/material', async (original) => ({ ...await original<typeof import('@mui/material')>(), useMediaQuery: () => mediaQuery.mobile }));

afterEach(() => { cleanup(); mediaQuery.mobile = false; });

// Only name/size/type are read, so a real 200 MB File is not needed.
const file = (name: string, size: number, type = 'application/octet-stream'): File => ({ name, size, type }) as unknown as File;
const pick = (files: File[]) => fireEvent.change(screen.getByTestId('composer-file-input'), { target: { files } });

describe('Composer upload precheck', () => {
    it('refuses an oversized file before uploading, names it and the limit, and still uploads the rest', async () => {
        const ok = file('ok.txt', 3, 'text/plain');
        const attach = vi.fn().mockResolvedValue([{ name: 'ok.txt', path: 'uploads/abc-ok.txt', type: 'text/plain', size: 3 }]);
        render(<Composer placeholder="Reply" onSend={vi.fn()} attach={attach} />);

        await act(async () => pick([file('big.bin', MAX_UPLOAD_BYTES + 2048), ok]));

        await waitFor(() => expect(attach).toHaveBeenCalledOnce());
        expect(attach).toHaveBeenCalledWith([ok]);
        const alert = screen.getByRole('alert');
        expect(alert).toHaveTextContent('big.bin is 200 MB, over the 200 MB limit by 2 KB');
        // The refused file never became a chip.
        expect(screen.queryByText('big.bin')).not.toBeInTheDocument();
    });

    it('sends nothing when every picked file is oversized', async () => {
        const attach = vi.fn();
        render(<Composer placeholder="Reply" onSend={vi.fn()} attach={attach} />);

        await act(async () => pick([file('huge.bin', MAX_UPLOAD_BYTES + 1)]));

        expect(attach).not.toHaveBeenCalled();
        expect(screen.getByRole('alert')).toHaveTextContent('huge.bin');
        expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled();
    });
});
