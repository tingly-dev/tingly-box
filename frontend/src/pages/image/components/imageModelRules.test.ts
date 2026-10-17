import { describe, expect, it } from 'vitest';
import type { Rule } from '@/components/RoutingGraphTypes';
import { directService, findDirectRule, imageModelOptions } from './imageModelRules';

const rule = (request_model: string, services: Rule['services'], extra: Partial<Rule> = {}): Rule => ({
    uuid: request_model,
    scenario: 'imagegen',
    request_model,
    active: true,
    services,
    ...extra,
});

describe('directService', () => {
    it('returns the one active service', () => {
        expect(directService(rule('a', [
            { provider: 'p1', model: 'gpt-image-1' },
            { provider: 'p2', model: 'x', active: false },
        ]))).toEqual({ provider: 'p1', model: 'gpt-image-1' });
    });

    it('has no answer for several services, none, or smart routing', () => {
        expect(directService(rule('a', [{ provider: 'p1', model: 'm' }, { provider: 'p2', model: 'm' }]))).toBeNull();
        expect(directService(rule('a', []))).toBeNull();
        expect(directService(rule('a', [{ provider: 'p1', model: 'm' }], {
            smart_enabled: true,
            smart_routing: [{ uuid: 's', description: '', ops: [], services: [] }] as unknown as Rule['smart_routing'],
        }))).toBeNull();
    });
});

describe('imageModelOptions', () => {
    it('skips inactive and unnamed rules, dedupes names, and puts direct rules first', () => {
        const options = imageModelOptions([
            rule('mixed', [{ provider: 'p1', model: 'm' }, { provider: 'p2', model: 'm' }]),
            rule('off', [{ provider: 'p1', model: 'm' }], { active: false }),
            rule(' ', [{ provider: 'p1', model: 'm' }]),
            rule('gpt-image-1', [{ provider: 'p1', model: 'gpt-image-1' }]),
            rule('gpt-image-1', [{ provider: 'p2', model: 'other' }]),
        ]);
        expect(options.map((option) => option.requestModel)).toEqual(['gpt-image-1', 'mixed']);
        expect(options[0].direct).toEqual({ provider: 'p1', model: 'gpt-image-1' });
        expect(options[1].direct).toBeNull();
    });
});

describe('findDirectRule', () => {
    it('reuses an active rule that points only at the pair', () => {
        const rules = [
            rule('both', [{ provider: 'p1', model: 'm' }, { provider: 'p2', model: 'm' }]),
            rule('off', [{ provider: 'p1', model: 'm' }], { active: false }),
            rule('mine', [{ provider: 'p1', model: 'm' }]),
        ];
        expect(findDirectRule(rules, 'p1', 'm')?.request_model).toBe('mine');
        expect(findDirectRule(rules, 'p2', 'm')).toBeUndefined();
    });
});
