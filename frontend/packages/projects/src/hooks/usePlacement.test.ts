import { renderHook } from '@testing-library/react';
import { afterEach, expect, it } from 'vitest';
import { usePlacement } from './usePlacement';

afterEach(() => localStorage.clear());

it('opens threads in the main area by default and preserves saved placements', () => {
    const first = renderHook(() => usePlacement());
    expect(first.result.current.thread).toBe('main');
    first.unmount();
    localStorage.setItem('tingly-project.threadPlacement', 'float');
    expect(renderHook(() => usePlacement()).result.current.thread).toBe('float');
});
