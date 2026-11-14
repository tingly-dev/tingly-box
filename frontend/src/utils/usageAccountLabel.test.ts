import { describe, expect, it } from 'vitest';
import { usageAccountLabel } from './usageAccountLabel';

describe('usageAccountLabel', () => {
    const teams = [{id: 'team-a', name: 'Project A'}, {id: 'team-b', name: 'Project B'}];
    it('distinguishes the same account name in different Teams', () => {
        expect(usageAccountLabel('alex', 'team-a', teams)).toBe('alex · Project A');
        expect(usageAccountLabel('alex', 'team-b', teams)).toBe('alex · Project B');
    });
    it('keeps identities distinguishable when Team metadata is missing', () => {
        expect(usageAccountLabel('alex', 'team-a', [])).toBe('alex · team-a');
        expect(usageAccountLabel('alex', 'team-b', [])).toBe('alex · team-b');
    });
    it('leaves the owner or legacy unscoped name alone', () => {
        expect(usageAccountLabel('Primary account', undefined, teams)).toBe('Primary account');
    });
});
