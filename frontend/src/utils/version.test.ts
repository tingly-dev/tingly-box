import { describe, expect, it } from 'vitest';
import { bareVersion, displayVersion } from './version';

describe('version formatting', () => {
    it.each([
        ['v1.261001.1', 'v1.261001.1', '1.261001.1'],
        ['1.261002.0', 'v1.261002.0', '1.261002.0'], // npm's latest, no "v"
        ['v1.261001.1-27-g4e437c592', 'v1.261001.1-27-g4e437c592', '1.261001.1-27-g4e437c592'],
        ['v1.261001.1+build.5', 'v1.261001.1', '1.261001.1'],
        ['dev', 'dev', 'dev'],
        ['Unknown', 'Unknown', 'Unknown'],
        ['vendor-build', 'vendor-build', 'vendor-build'], // a "v" that is not a prefix
    ])('%s → display %s, bare %s', (input, display, bare) => {
        expect(displayVersion(input)).toBe(display);
        expect(bareVersion(input)).toBe(bare);
    });

    it('never doubles the v', () => {
        expect(displayVersion(displayVersion('v1.2.3'))).toBe('v1.2.3');
    });

    it('shows Unknown for a missing version', () => {
        expect(displayVersion('')).toBe('Unknown');
        expect(displayVersion(null)).toBe('Unknown');
    });

    it('swaps the build commit in for a bare dev version', () => {
        expect(displayVersion('dev', 'dev-a1b2c3d')).toBe('dev-a1b2c3d');
        expect(displayVersion('dev', 'dev-a1b2c3d-dirty')).toBe('dev-a1b2c3d-dirty');
    });

    it('leaves a dev version alone when no commit was stamped in', () => {
        expect(displayVersion('dev', undefined)).toBe('dev');
        expect(displayVersion('dev', '')).toBe('dev');
    });

    it('never lets the dev commit override a release version', () => {
        expect(displayVersion('v1.261001.1', 'dev-a1b2c3d')).toBe('v1.261001.1');
        expect(displayVersion('1.261002.0', 'dev-a1b2c3d')).toBe('v1.261002.0');
    });
});
