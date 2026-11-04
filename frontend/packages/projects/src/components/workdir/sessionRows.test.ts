import { describe, expect, it } from 'vitest';
import type { Thread } from '../../client';
import { SESSION_GROUPS, sessionGroup, sessionRow } from './sessionRows';

const thread = (over: Partial<Thread>) => ({ id: 't', title: 'T', status: 'idle', cwd: '/home/me/app', updated_at: '2026-10-22T00:00:00Z', ...over }) as Thread;

describe('sessionRows', () => {
    it('groups ready with idle and archived apart, most urgent group first', () => {
        expect(sessionGroup(thread({ status: 'ready' }))).toBe('idle');
        expect(sessionGroup(thread({ status: 'waiting' }))).toBe('waiting');
        expect(sessionGroup(thread({ status: 'resolved', archived_at: '2026-10-22T00:00:00Z' }))).toBe('archived');
        expect(SESSION_GROUPS.map((g) => g.key)).toEqual(['waiting', 'failed', 'working', 'idle', 'resolved', 'archived']);
    });

    it('names the folder and the wait reason only where they apply', () => {
        const waiting = thread({ status: 'waiting', wait_reason: 'Pick one' });
        expect(sessionRow(waiting, { withFolder: true, locale: 'en', selected: false }).subtitle).toBe('app · Pick one');
        expect(sessionRow(waiting, { withFolder: false, locale: 'en', selected: true })).toMatchObject({ subtitle: 'Pick one', selected: true });
        expect(sessionRow(thread({ status: 'idle' }), { withFolder: false, locale: 'en', selected: false }).subtitle).toBeUndefined();
    });
});
