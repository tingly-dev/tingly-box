import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const { phone } = vi.hoisted(() => ({ phone: { value: false } }));
vi.mock('@tingly/ui/hooks/useIsPhone', () => ({ useIsPhone: () => phone.value }));

// The pref lives in module state, so each test gets a fresh module.
const load = async () => import('./uiShell');

beforeEach(() => {
    vi.resetModules();
    localStorage.clear();
    window.history.replaceState({}, '', '/');
    phone.value = false;
});
afterEach(cleanup);

describe('resolveShell', () => {
    it('keeps auto on the desktop shell until the phone shell is switched on', async () => {
        const { resolveShell, MOBILE_AUTO } = await load();
        expect(resolveShell('auto', true)).toBe(MOBILE_AUTO ? 'mobile' : 'desktop');
        expect(resolveShell('auto', false)).toBe('desktop');
    });

    it('lets an explicit choice win over the screen', async () => {
        const { resolveShell } = await load();
        expect(resolveShell('mobile', false)).toBe('mobile');
        expect(resolveShell('desktop', true)).toBe('desktop');
    });
});

describe('useUiShell', () => {
    it('defaults to auto', async () => {
        const { useUiShell } = await load();
        const { result } = renderHook(() => useUiShell());
        expect(result.current).toEqual({ shell: 'desktop', pref: 'auto' });
    });

    it('takes ?ui= and remembers it', async () => {
        window.history.replaceState({}, '', '/?ui=mobile');
        const { useUiShell } = await load();
        const { result } = renderHook(() => useUiShell());
        expect(result.current).toEqual({ shell: 'mobile', pref: 'mobile' });
        expect(localStorage.getItem('ui.shell')).toBe('mobile');
    });

    it('reads the stored choice and ignores junk', async () => {
        localStorage.setItem('ui.shell', 'desktop');
        const first = (await load()).useUiShell;
        expect(renderHook(() => first()).result.current.pref).toBe('desktop');
        vi.resetModules();
        localStorage.setItem('ui.shell', 'tablet');
        const second = (await load()).useUiShell;
        expect(renderHook(() => second()).result.current.pref).toBe('auto');
    });

    it('updates every reader when the choice changes', async () => {
        const { useUiShell, setShellPref } = await load();
        const a = renderHook(() => useUiShell());
        const b = renderHook(() => useUiShell());
        act(() => setShellPref('mobile'));
        expect(a.result.current.shell).toBe('mobile');
        expect(b.result.current.shell).toBe('mobile');
        expect(localStorage.getItem('ui.shell')).toBe('mobile');
    });
});
