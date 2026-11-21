import { useEffect } from 'react';
import { PHONE_SHELL_ATTR, PHONE_SHELL_VALUE, PHONE_VIEWPORT_HEIGHT, PHONE_VIEWPORT_TOP } from '@tingly/ui/theme/phone';

const isTextField = (el: EventTarget | null): el is HTMLElement =>
    el instanceof HTMLElement && (el.tagName === 'TEXTAREA' || (el.tagName === 'INPUT' && !['checkbox', 'radio', 'button', 'submit', 'range', 'file'].includes((el as HTMLInputElement).type)));

/**
 * What the phone shell does to the document while it is mounted: marks the
 * body so the phone styles (`@tingly/ui/theme/phone`) apply — to dialogs too,
 * which render outside the shell — tracks the visible viewport so sheets stay
 * above the on-screen keyboard, and brings a focused field into view.
 */
export function usePhoneShellBody() {
    useEffect(() => {
        const { body, documentElement: root } = document;
        body.setAttribute(PHONE_SHELL_ATTR, PHONE_SHELL_VALUE);

        const vv = window.visualViewport;
        const sync = () => {
            if (!vv) return;
            root.style.setProperty(PHONE_VIEWPORT_HEIGHT, `${vv.height}px`);
            root.style.setProperty(PHONE_VIEWPORT_TOP, `${vv.offsetTop}px`);
        };
        sync();
        vv?.addEventListener('resize', sync);
        vv?.addEventListener('scroll', sync);

        // The keyboard takes a moment to open; then put the field mid-screen.
        let timer: number | undefined;
        const onFocus = (e: FocusEvent) => {
            if (!isTextField(e.target)) return;
            const field = e.target;
            window.clearTimeout(timer);
            timer = window.setTimeout(() => field.scrollIntoView?.({ block: 'center', behavior: 'smooth' }), 300);
        };
        document.addEventListener('focusin', onFocus);

        return () => {
            body.removeAttribute(PHONE_SHELL_ATTR);
            root.style.removeProperty(PHONE_VIEWPORT_HEIGHT);
            root.style.removeProperty(PHONE_VIEWPORT_TOP);
            vv?.removeEventListener('resize', sync);
            vv?.removeEventListener('scroll', sync);
            document.removeEventListener('focusin', onFocus);
            window.clearTimeout(timer);
        };
    }, []);
}
