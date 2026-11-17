import { act, renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

// Keep the write local: the real setSyncedItem also mirrors to the server.
vi.mock('@/services/uiPrefs', () => ({
    setSyncedItem: (key: string, value: string) => localStorage.setItem(key, value),
}));

const { SCENARIOS, getHiddenScenarios, useHiddenScenarios } = await import('./scenarioRegistry');

describe('hidden scenarios', () => {
    beforeEach(() => localStorage.clear());

    it('hides cursor and vmodel by default when nothing is stored', () => {
        expect([...getHiddenScenarios()]).toEqual(['cursor', 'vmodel']);
        const { result } = renderHook(() => useHiddenScenarios());
        expect(SCENARIOS.filter((s) => s.id !== 'cursor' && s.id !== 'vmodel').some((s) => result.current.isHidden(s.id))).toBe(false);
    });

    it('persists a hidden agent across remounts and leaves the rest visible', () => {
        const first = renderHook(() => useHiddenScenarios());
        act(() => first.result.current.toggleHidden('codex'));
        first.unmount();

        const { result } = renderHook(() => useHiddenScenarios());
        expect(result.current.isHidden('codex')).toBe(true);
        expect(SCENARIOS.filter((s) => s.id !== 'codex' && s.id !== 'cursor' && s.id !== 'vmodel').some((s) => result.current.isHidden(s.id))).toBe(false);
    });

    it('persists showing a default-hidden agent', () => {
        const first = renderHook(() => useHiddenScenarios());
        act(() => first.result.current.toggleHidden('cursor'));
        first.unmount();

        expect(getHiddenScenarios().has('cursor')).toBe(false);
        expect(localStorage.getItem('scenario.hiddenScenarios')).toBe('["vmodel"]');
    });

    it('uses the VModel power-up visibility for its scenario', () => {
        expect(SCENARIOS.find((s) => s.id === 'vmodel')?.path).toBe('/agent/vmodel');
        const { result } = renderHook(() => useHiddenScenarios());
        expect(result.current.isHidden('vmodel')).toBe(true);
        act(() => result.current.toggleHidden('vmodel'));
        expect(result.current.isHidden('vmodel')).toBe(false);
        expect(getHiddenScenarios().has('vmodel')).toBe(false);
    });

    it('keeps a stored choice over the defaults', () => {
        localStorage.setItem('scenario.hiddenScenarios', '["pi"]');
        localStorage.setItem('scenario.hiddenDefaultsVersion', '5');
        expect([...getHiddenScenarios()]).toEqual(['pi']);
    });
});
