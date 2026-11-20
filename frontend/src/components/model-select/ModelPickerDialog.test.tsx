import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { PhoneShellContext } from '@tingly/ui/hooks/usePhoneShell';
import { ModelPickerDialog } from './ModelPickerDialog';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

afterEach(cleanup);

describe('ModelPickerDialog', () => {
    it('is a full-screen sheet in the phone shell, closed by its button or the system Back', () => {
        const onClose = vi.fn();
        const pushState = vi.spyOn(window.history, 'pushState');
        render(
            <PhoneShellContext.Provider value>
                <ModelPickerDialog open onClose={onClose} title="Choose Model"><p>picker</p></ModelPickerDialog>
            </PhoneShellContext.Provider>,
        );

        expect(document.querySelector('.MuiDialog-paperFullScreen')).not.toBeNull();
        expect(screen.getByRole('heading', { name: 'Choose Model' })).toBeInTheDocument();
        // Opening took a history entry, so Back closes the sheet rather than the page.
        expect(pushState).toHaveBeenCalledTimes(1);
        act(() => { window.dispatchEvent(new PopStateEvent('popstate')); });
        expect(onClose).toHaveBeenCalledTimes(1);

        fireEvent.click(screen.getByRole('button', { name: 'common.close' }));
        expect(onClose).toHaveBeenCalledTimes(2);
        pushState.mockRestore();
    });

    it('stays the large centred dialog on the desktop, with no history entry', () => {
        const pushState = vi.spyOn(window.history, 'pushState');
        render(<ModelPickerDialog open onClose={vi.fn()} title="Choose Model"><p>picker</p></ModelPickerDialog>);

        expect(document.querySelector('.MuiDialog-paperFullScreen')).toBeNull();
        expect(document.querySelector('.MuiDialog-paperWidthLg')).not.toBeNull();
        expect(screen.queryByRole('button', { name: 'common.close' })).not.toBeInTheDocument();
        expect(pushState).not.toHaveBeenCalled();
        pushState.mockRestore();
    });
});
