import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { readLocal, writeLocal } from '../utils/storage';
import { readMessageDetail, useMessageDetail } from './useMessageDetail';

beforeEach(() => localStorage.clear());
afterEach(() => { cleanup(); vi.restoreAllMocks(); });

describe('message detail preferences', () => {
    it('defaults to detailed and does not confuse tool expansion with visibility', () => {
        writeLocal('thread.expandTools', '0');
        expect(readMessageDetail('thread')).toBe('detailed');
        expect(readMessageDetail('conversation')).toBe('detailed');
    });

    it('maps the old report preference and gives a valid new preference precedence', () => {
        writeLocal('conversation.threadReports', '0');
        expect(readMessageDetail('conversation')).toBe('concise');
        expect(readMessageDetail('thread')).toBe('detailed');
        writeLocal('conversation.messageDetail', 'detailed');
        expect(readMessageDetail('conversation')).toBe('detailed');
        writeLocal('conversation.messageDetail', 'bad-value');
        expect(readMessageDetail('conversation')).toBe('detailed');
        localStorage.clear();
        writeLocal('conversation.threadReports', '1');
        expect(readMessageDetail('conversation')).toBe('detailed');
    });

    it('persists on remount, syncs open thread panes and keeps the manager independent', () => {
        const first = renderHook(() => useMessageDetail('thread'));
        const second = renderHook(() => useMessageDetail('thread'));
        const manager = renderHook(() => useMessageDetail('conversation'));
        act(() => first.result.current[1]('concise'));
        expect(second.result.current[0]).toBe('concise');
        expect(manager.result.current[0]).toBe('detailed');
        expect(readLocal('thread.messageDetail')).toBe('concise');
        first.unmount();
        expect(renderHook(() => useMessageDetail('thread')).result.current[0]).toBe('concise');
        act(() => {
            writeLocal('thread.messageDetail', 'detailed');
            window.dispatchEvent(new StorageEvent('storage'));
        });
        expect(second.result.current[0]).toBe('detailed');
    });

    it('works for the current page when storage is unavailable', () => {
        vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('blocked'); });
        vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('blocked'); });
        const hook = renderHook(() => useMessageDetail('thread'));
        expect(hook.result.current[0]).toBe('detailed');
        act(() => hook.result.current[1]('concise'));
        expect(hook.result.current[0]).toBe('concise');
    });
});
