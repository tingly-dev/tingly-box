import { describe, expect, it } from 'vitest';
import {
    buildExperimentalFeatureRedirect,
    parseExperimentalFeature,
    sanitizeFeatureReturnPath,
} from './ExperimentalFeatureGate';

describe('experimental feature routing', () => {
    it('preserves the requested page in the Experimental redirect', () => {
        expect(buildExperimentalFeatureRedirect('guardrails', '/guardrails/rules?source=bookmark')).toBe(
            '/system/experimental?feature=guardrails&returnTo=%2Fguardrails%2Frules%3Fsource%3Dbookmark',
        );
    });

    it('accepts only known feature keys', () => {
        expect(parseExperimentalFeature('mcp')).toBe('mcp');
        expect(parseExperimentalFeature('projects')).toBe('projects');
        expect(parseExperimentalFeature('unknown')).toBeUndefined();
    });

    it('accepts only local return paths', () => {
        expect(sanitizeFeatureReturnPath('/prompt/skill')).toBe('/prompt/skill');
        expect(sanitizeFeatureReturnPath('//example.com')).toBeUndefined();
        expect(sanitizeFeatureReturnPath('https://example.com')).toBeUndefined();
    });
});
