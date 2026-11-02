import { describe, expect, it } from 'vitest';
import type { Provider } from '@/types/provider';
import { sortCredentials } from './sortCredentials';

const provider = (name: string, uuid = name) => ({ name, uuid } as Provider);

describe('sortCredentials', () => {
    it('sorts names naturally without changing the input', () => {
        const original = [provider('Provider 10'), provider('beta'), provider('Provider 2'), provider('Alpha')];
        expect(sortCredentials(original, 'en').map(p => p.name)).toEqual(['Alpha', 'beta', 'Provider 2', 'Provider 10']);
        expect(original[0].name).toBe('Provider 10');
    });

    it('groups recognized providers before unknown providers, sorting names within each', () => {
        const original = [provider('Zeta', 'openai'), provider('Alpha', 'unknown'), provider('Beta 10', 'anthropic'), provider('Beta 2', 'anthropic-2')];
        const category = (p: Provider) => p.uuid.startsWith('anthropic') ? 'anthropic' : p.uuid === 'openai' ? 'openai' : undefined;
        expect(sortCredentials(original, 'en', 'provider', category).map(p => p.name)).toEqual(['Beta 2', 'Beta 10', 'Zeta', 'Alpha']);
    });

    it('orders equal names deterministically by UUID', () => {
        expect(sortCredentials([provider('Example', 'b'), provider('example', 'a')], 'en').map(p => p.uuid)).toEqual(['a', 'b']);
    });
});
