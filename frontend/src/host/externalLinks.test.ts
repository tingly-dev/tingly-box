import { afterEach, describe, expect, it, vi } from 'vitest';
import { externalHref, routeExternalLinks } from './externalLinks';

const appLocation = { href: 'wails://localhost/agent', origin: 'wails://localhost' };

describe('externalHref', () => {
    it.each([
        ['https://github.com/tingly-dev/tingly-box', 'https://github.com/tingly-dev/tingly-box'],
        ['http://example.com/x', 'http://example.com/x'],
        ['mailto:box@tingly.dev', 'mailto:box@tingly.dev'],
        ['/dashboard', null], // in-app route
        ['wails://localhost/system', null], // same origin
        ['#section', null],
        ['javascript:void(0)', null],
        ['file:///etc/passwd', null],
    ])('%s → %s', (href, expected) => {
        expect(externalHref(href, appLocation)).toBe(expected);
    });
});

describe('routeExternalLinks', () => {
    let undo: (() => void) | undefined;
    afterEach(() => {
        undo?.();
        document.body.innerHTML = '';
    });

    function link(href: string, attrs: Record<string, string> = {}) {
        const a = document.createElement('a');
        a.href = href;
        Object.entries(attrs).forEach(([k, v]) => a.setAttribute(k, v));
        a.innerHTML = '<span>label</span>';
        document.body.appendChild(a);
        return a;
    }

    function click(target: Element, init: MouseEventInit = {}) {
        const event = new MouseEvent('click', { bubbles: true, cancelable: true, button: 0, ...init });
        target.dispatchEvent(event);
        return event;
    }

    it('opens external links in the OS browser instead of following them', () => {
        const open = vi.fn();
        undo = routeExternalLinks(window, open);
        const a = link('https://github.com/tingly-dev/tingly-box', { target: '_blank' });
        // Clicking the label inside the anchor still counts.
        const event = click(a.querySelector('span')!);
        expect(open).toHaveBeenCalledWith('https://github.com/tingly-dev/tingly-box');
        expect(event.defaultPrevented).toBe(true);
    });

    it('leaves in-app links to the router', () => {
        const open = vi.fn();
        undo = routeExternalLinks(window, open);
        const event = click(link('/dashboard'));
        expect(open).not.toHaveBeenCalled();
        expect(event.defaultPrevented).toBe(false);
    });

    it('respects a handler that already prevented the click', () => {
        const open = vi.fn();
        undo = routeExternalLinks(window, open);
        const a = link('https://example.com');
        a.addEventListener('click', (e) => e.preventDefault());
        click(a);
        expect(open).not.toHaveBeenCalled();
    });

    it('leaves download links alone', () => {
        const open = vi.fn();
        undo = routeExternalLinks(window, open);
        click(link('https://example.com/file.zip', { download: '' }));
        expect(open).not.toHaveBeenCalled();
    });

    it('routes window.open for external URLs and restores it on undo', () => {
        const original = window.open;
        const open = vi.fn();
        undo = routeExternalLinks(window, open);
        expect(window.open('https://example.com/docs', '_blank')).toBeNull();
        expect(open).toHaveBeenCalledWith('https://example.com/docs');
        undo();
        undo = undefined;
        expect(window.open).toBe(original);
    });
});
