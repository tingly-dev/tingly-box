import {act, cleanup, renderHook} from '@testing-library/react';
import {afterEach, describe, expect, it, vi} from 'vitest';
import {useDeskDrafts} from './useDeskDrafts';

afterEach(() => { cleanup(); sessionStorage.clear(); vi.restoreAllMocks(); });

describe('Desk drafts', () => {
    it('restores per-session drafts across remounts and saves successive edits', () => {
        const first = renderHook(() => useDeskDrafts('desk.test'));
        act(() => {
            first.result.current[1]((prev) => ({...prev, a: 'first'}));
            first.result.current[1]((prev) => ({...prev, b: 'second'}));
        });
        first.unmount();
        const restored = renderHook(() => useDeskDrafts('desk.test'));
        expect(restored.result.current[0]).toEqual({a: 'first', b: 'second'});
        act(() => restored.result.current[1]((prev) => ({...prev, a: ''})));
        expect(JSON.parse(sessionStorage.getItem('desk.test')!)).toEqual({a: '', b: 'second'});
    });

    it('saves acceptance even when navigation already unmounted the composer', () => {
        const draft = renderHook(() => useDeskDrafts('desk.test'));
        const update = draft.result.current[1];
        act(() => update({prompt: 'submitted'}));
        draft.unmount();
        act(() => update({prompt: ''}));
        expect(JSON.parse(sessionStorage.getItem('desk.test')!)).toEqual({prompt: ''});
    });

    it('ignores malformed storage and non-string fields', () => {
        sessionStorage.setItem('broken', '{');
        expect(renderHook(() => useDeskDrafts('broken')).result.current[0]).toEqual({});
        sessionStorage.setItem('mixed', JSON.stringify({a: 'valid', b: 3, c: {text: 'invalid'}}));
        expect(renderHook(() => useDeskDrafts('mixed')).result.current[0]).toEqual({a: 'valid'});
    });

    it('works in memory when browser storage is blocked', () => {
        vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('blocked'); });
        vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('blocked'); });
        const draft = renderHook(() => useDeskDrafts('desk.test'));
        act(() => draft.result.current[1]({a: 'still editable'}));
        expect(draft.result.current[0]).toEqual({a: 'still editable'});
    });
});
