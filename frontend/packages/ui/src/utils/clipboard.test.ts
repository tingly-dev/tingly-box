import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { writeToClipboard } from './clipboard';

// jsdom has no Clipboard API and no working execCommand, so the fallback tests
// stub execCommand with a small model of what a browser copies.
describe('writeToClipboard', () => {
    const execCommand = document.execCommand;
    const cleanups: Array<() => void> = [];
    let clipboard: string | undefined;

    beforeEach(() => {
        vi.stubGlobal('isSecureContext', false);
        clipboard = undefined;
    });

    afterEach(() => {
        cleanups.splice(0).forEach((cleanup) => cleanup());
        vi.unstubAllGlobals();
        document.execCommand = execCommand;
        document.body.innerHTML = '';
    });

    // stubCopyCommand fires a copy event at the focused element (unless
    // fireEvent is false), then copies the event's data if a listener
    // cancelled it, or else the focused textarea's selection.
    function stubCopyCommand({ fireEvent = true, result = true } = {}) {
        const setData = vi.fn();
        document.execCommand = vi.fn(() => {
            if (!result) {
                return false;
            }
            const event = new Event('copy', { bubbles: true, cancelable: true });
            Object.defineProperty(event, 'clipboardData', { value: { setData } });
            if (fireEvent) {
                (document.activeElement ?? document.body).dispatchEvent(event);
            }
            const active = document.activeElement;
            if (event.defaultPrevented) {
                clipboard = setData.mock.lastCall?.[1];
            } else if (active instanceof HTMLTextAreaElement) {
                clipboard = active.value.slice(active.selectionStart, active.selectionEnd);
            } else {
                clipboard = '';
            }
            return true;
        });
        return setData;
    }

    // trapFocus models a MUI Dialog's focus trap: focus that leaves the dialog
    // is moved straight back to the dialog's button.
    function trapFocus() {
        const dialog = document.createElement('div');
        const button = document.createElement('button');
        dialog.appendChild(button);
        document.body.appendChild(dialog);
        button.focus();
        const onFocus = (event: FocusEvent) => {
            if (!dialog.contains(event.target as Node)) {
                button.focus();
            }
        };
        document.addEventListener('focus', onFocus, true);
        cleanups.push(() => document.removeEventListener('focus', onFocus, true));
        return button;
    }

    it('writes the text from the copy event', async () => {
        const setData = stubCopyCommand();

        await writeToClipboard('model = "x"\n');

        expect(setData).toHaveBeenCalledWith('text/plain', 'model = "x"\n');
        expect(clipboard).toBe('model = "x"\n');
        expect(document.body.querySelector('textarea')).toBeNull();
    });

    it('copies inside a dialog that takes focus back', async () => {
        const button = trapFocus();
        stubCopyCommand();

        await writeToClipboard('tb-key');

        expect(clipboard).toBe('tb-key');
        expect(document.activeElement).toBe(button);
    });

    it('copies the selection when no copy event fires', async () => {
        stubCopyCommand({ fireEvent: false });

        await writeToClipboard('a\tb');

        expect(clipboard).toBe('a\tb');
    });

    it('rejects when no copy event fires and the textarea lost focus', async () => {
        trapFocus();
        stubCopyCommand({ fireEvent: false });

        await expect(writeToClipboard('tb-key')).rejects.toThrow('Copy command was rejected');
        expect(document.body.querySelector('textarea')).toBeNull();
    });

    it('rejects when the copy command is refused and leaves later copies alone', async () => {
        stubCopyCommand({ result: false });

        await expect(writeToClipboard('tb-key')).rejects.toThrow('Copy command was rejected');
        const later = new Event('copy', { cancelable: true });
        Object.defineProperty(later, 'clipboardData', { value: { setData: vi.fn() } });
        document.dispatchEvent(later);
        expect(later.defaultPrevented).toBe(false);
    });

    it('uses the Clipboard API in a secure context', async () => {
        const writeText = vi.fn(async () => {});
        vi.stubGlobal('isSecureContext', true);
        vi.stubGlobal('navigator', { clipboard: { writeText } });
        stubCopyCommand();

        await writeToClipboard('copied');

        expect(writeText).toHaveBeenCalledWith('copied');
        expect(document.execCommand).not.toHaveBeenCalled();
    });
});
