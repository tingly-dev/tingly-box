import { describe, expect, it } from 'vitest';
import { buildDevVersion, DEV_SHORT_SHA_LENGTH, readGitDevVersion } from './devVersion';

describe('buildDevVersion', () => {
    it('uses seven hex digits of the sha', () => {
        expect(DEV_SHORT_SHA_LENGTH).toBe(7);
        expect(buildDevVersion('a1b2c3d', false)).toBe('dev-a1b2c3d');
    });

    it('marks a dirty worktree', () => {
        expect(buildDevVersion('a1b2c3d', true)).toBe('dev-a1b2c3d-dirty');
    });

    it('falls back to plain dev without a sha', () => {
        expect(buildDevVersion(null, false)).toBe('dev');
        expect(buildDevVersion(undefined, true)).toBe('dev');
        expect(buildDevVersion('', false)).toBe('dev');
    });
});

describe('readGitDevVersion', () => {
    it('reads the short sha and a clean tree', () => {
        const run = (cmd: string) =>
            cmd.startsWith('git rev-parse') ? 'a1b2c3d\n' : '';
        expect(readGitDevVersion(run)).toBe('dev-a1b2c3d');
    });

    it('reads the short sha and a dirty tree', () => {
        const run = (cmd: string) =>
            cmd.startsWith('git rev-parse') ? 'a1b2c3d\n' : ' M src/app.tsx\n';
        expect(readGitDevVersion(run)).toBe('dev-a1b2c3d-dirty');
    });

    it('falls back to dev when git is unavailable', () => {
        const run = () => {
            throw new Error('git: command not found');
        };
        expect(readGitDevVersion(run)).toBe('dev');
    });

    it('falls back to dev when git prints an error instead of a sha', () => {
        const run = () => 'fatal: not a git repository\n';
        expect(readGitDevVersion(run)).toBe('dev');
    });
});
