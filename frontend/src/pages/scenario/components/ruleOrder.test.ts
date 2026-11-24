import { describe, expect, it } from 'vitest';
import { moveBefore, pinToTop } from './ruleOrder';

describe('moveBefore', () => {
    const order = ['a', 'b', 'c', 'd'];

    it('moves an item down to the target slot', () => {
        expect(moveBefore(order, 'a', 'c')).toEqual(['b', 'c', 'a', 'd']);
    });

    it('moves an item up to the target slot', () => {
        expect(moveBefore(order, 'd', 'b')).toEqual(['a', 'd', 'b', 'c']);
    });

    it('does not mutate the input', () => {
        moveBefore(order, 'a', 'd');
        expect(order).toEqual(['a', 'b', 'c', 'd']);
    });

    it('is a no-op when dropped on itself', () => {
        expect(moveBefore(order, 'b', 'b')).toBeNull();
    });

    it('is a no-op when either uuid vanished (a refetch removed it mid-drag)', () => {
        expect(moveBefore(order, 'x', 'b')).toBeNull();
        expect(moveBefore(order, 'a', 'x')).toBeNull();
    });
});

describe('pinToTop', () => {
    it('moves an item to the first slot, keeping the others in order', () => {
        expect(pinToTop(['a', 'b', 'c', 'd'], 'c')).toEqual(['c', 'a', 'b', 'd']);
    });

    it('is a no-op for the first item and for unknown uuids', () => {
        expect(pinToTop(['a', 'b'], 'a')).toBeNull();
        expect(pinToTop(['a', 'b'], 'x')).toBeNull();
    });
});
