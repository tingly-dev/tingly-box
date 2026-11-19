import { describe, expect, it } from 'vitest';
import { type Skill, type SkillLocation } from '@/types/prompt';
import {
    formatFileSize,
    getRelativePath,
    getTwoLevelDisplayName,
    groupSkillsIntelligently,
    normalizePathLike,
} from './skillGrouping';

const location = (
    groupingStrategy?: SkillLocation['grouping_strategy'],
): SkillLocation => ({
    id: 'location-1',
    name: 'Skills',
    path: '/workspace/skills',
    ide_source: 'custom',
    skill_count: 0,
    grouping_strategy: groupingStrategy,
});

const skill = (relativePath: string): Skill => ({
    id: relativePath,
    location_id: 'location-1',
    name: relativePath,
    filename: relativePath.split('/').at(-1) || relativePath,
    path: `/workspace/skills/${relativePath}`,
    file_type: 'markdown',
});

describe('skill presentation helpers', () => {
    it('normalizes path separators and redundant segments', () => {
        expect(normalizePathLike(String.raw`docs\\./rules//policy.md`)).toBe(
            'docs/rules/policy.md',
        );
    });

    it('derives relative and two-level display paths', () => {
        const item = skill('security/network/policy.md');

        expect(getRelativePath(item, location())).toBe('security/network/policy.md');
        expect(getTwoLevelDisplayName(item, location())).toBe('network/policy.md');
    });

    it('formats common file-size boundaries', () => {
        expect(formatFileSize()).toBe('-');
        expect(formatFileSize(1023)).toBe('1023 B');
        expect(formatFileSize(1024)).toBe('1.0 KB');
        expect(formatFileSize(1024 * 1024)).toBe('1.0 MB');
    });
});

describe('skill grouping', () => {
    it('keeps flat mode in a single group', () => {
        const items = [skill('one.md'), skill('docs/two.md')];
        const groups = groupSkillsIntelligently(
            items,
            location({ mode: 'flat' }),
        );

        expect(groups).toEqual([
            {
                groupKey: '',
                groupLabel: 'All Skills',
                skills: items,
                level: 0,
            },
        ]);
    });

    it('splits large auto groups by their second-level directory', () => {
        const groups = groupSkillsIntelligently(
            [skill('docs/api/one.md'), skill('docs/guides/two.md')],
            location({ mode: 'auto', min_files_for_split: 1 }),
        );

        expect(groups.map((group) => group.groupKey)).toEqual([
            'docs/api',
            'docs/guides',
        ]);
    });

    it('separates unmatched files in pattern mode', () => {
        const groups = groupSkillsIntelligently(
            [skill('team/skills/one.md'), skill('misc/two.md')],
            location({
                mode: 'pattern',
                group_pattern: '/skills/',
                min_files_for_split: 5,
            }),
        );

        expect(groups.map((group) => group.groupLabel)).toEqual([
            'team/skills',
            '(other)',
        ]);
    });
});
