import {describe, expect, it} from 'vitest';
import type {SessionInfo} from '@/services/deskApi';
import {attentionEvents} from './useDeskAttention';

const s = (id: string, status: string, awaiting = false) => ({id, status, awaiting_input: awaiting} as SessionInfo);
const before = (...list: SessionInfo[]) => new Map(list.map((x) => [x.id, x]));

describe('attentionEvents', () => {
    it('reports a turn that starts waiting on the user', () => {
        expect(attentionEvents(before(s('a', 'running')), [s('a', 'running', true)])).toEqual([{id: 'a', kind: 'needs-input'}]);
    });

    it('reports a turn that ends, as finished or failed', () => {
        expect(attentionEvents(before(s('a', 'running'), s('b', 'pending')), [s('a', 'completed'), s('b', 'failed')]))
            .toEqual([{id: 'a', kind: 'finished'}, {id: 'b', kind: 'failed'}]);
    });

    it('does not report archiving, steady states, or sessions it has not seen before', () => {
        expect(attentionEvents(before(s('a', 'running'), s('b', 'completed')), [s('a', 'closed'), s('b', 'completed'), s('c', 'completed')]))
            .toEqual([]);
    });
});
