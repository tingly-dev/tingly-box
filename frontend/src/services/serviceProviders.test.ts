import { describe, expect, it } from 'vitest';
import { searchProviders, type UniqueProvider } from './serviceProviders';

describe('searchProviders', () => {
    it('matches provider names and identifiers, not protocol or description mentions', () => {
        const providers: UniqueProvider[] = [
            { id: 'anthropic', name: 'Anthropic', alias: 'Claude', supportsOpenAI: false, supportsAnthropic: true },
            { id: 'other', name: 'Other', supportsOpenAI: true, supportsAnthropic: true,
                description: 'Supports Anthropic models', baseUrlAnthropic: 'https://api.example.com/anthropic?provider=anthropic' },
        ];

        expect(searchProviders(providers, ' ANTHROPIC ')).toEqual([providers[0]]);
        expect(searchProviders(providers, 'claude')).toEqual([providers[0]]);
        expect(searchProviders(providers, 'other')).toEqual([providers[1]]);
        expect(searchProviders(providers, 'API.EXAMPLE.COM')).toEqual([providers[1]]);
        expect(searchProviders(providers, '  ')).toEqual(providers);
    });
});
