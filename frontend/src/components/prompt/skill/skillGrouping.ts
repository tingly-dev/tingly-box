import { type Skill, type SkillLocation } from '@/types/prompt';

export interface SkillGroup {
    groupKey: string;
    groupLabel: string;
    skills: Skill[];
    level: number;
}

export const normalizePathLike = (value: string): string => {
    if (!value) return '';
    return value
        .replace(/\\/g, '/')
        .replace(/\/+/g, '/')
        .replace(/(^|\/)\.(?=\/|$)/g, '$1')
        .replace(/\/+/g, '/');
};

export const splitPathSegments = (value: string): string[] => {
    const normalized = normalizePathLike(value);
    if (normalized === '') return [];
    return normalized.split('/').filter((part) => part !== '' && part !== '.');
};

const normalizePatternForMatch = (value: string): string => {
    return splitPathSegments(value).join('/');
};

export const formatFileSize = (bytes?: number): string => {
    if (!bytes) return '-';
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
};

export const getRelativePath = (skill: Skill, location: SkillLocation): string => {
    const basePath = location.path.endsWith('/') ? location.path : `${location.path}/`;
    if (skill.path.startsWith(basePath)) {
        return skill.path.substring(basePath.length);
    }
    return skill.filename;
};

export const getTwoLevelDisplayName = (
    skill: Skill,
    location: SkillLocation,
): string => {
    const relativePath = getRelativePath(skill, location);
    const parts = splitPathSegments(relativePath);

    if (parts.length >= 2) {
        const parentDir = parts[parts.length - 2];
        const fileName = parts[parts.length - 1];
        return `${parentDir}/${fileName}`;
    }
    return relativePath;
};

const getGroupKeyFromPattern = (
    pattern: string,
    pathParts: string[],
): { groupKey: string; matched: boolean } => {
    const pathStr = pathParts.join('/');
    const normalizedPattern = normalizePatternForMatch(pattern);
    if (normalizedPattern === '') {
        return { groupKey: '', matched: false };
    }
    const patternIndex = pathStr.indexOf(normalizedPattern);

    if (patternIndex === -1) {
        return { groupKey: '', matched: false };
    }

    const matchEnd = patternIndex + normalizedPattern.length;
    const prefix = pathStr.substring(0, matchEnd);
    const groupKey =
        prefix.endsWith('/') && prefix.length > 1 ? prefix.slice(0, -1) : prefix;

    return { groupKey, matched: true };
};

const shouldSplitIntoSubGroups = (
    groupSkills: Skill[],
    location: SkillLocation,
): boolean => {
    const subGroups: Record<string, Skill[]> = {};
    for (const skill of groupSkills) {
        const relativePath = getRelativePath(skill, location);
        const parts = splitPathSegments(relativePath);
        if (parts.length >= 2) {
            const secondLevelDir = parts[1];
            if (!subGroups[secondLevelDir]) {
                subGroups[secondLevelDir] = [];
            }
            subGroups[secondLevelDir].push(skill);
        }
    }
    return Object.keys(subGroups).length >= 2;
};

const splitIntoSubGroups = (
    groupSkills: Skill[],
    location: SkillLocation,
    parentDir: string,
): SkillGroup[] => {
    const subGroups: Record<string, Skill[]> = {};
    const rootFiles: Skill[] = [];

    for (const skill of groupSkills) {
        const relativePath = getRelativePath(skill, location);
        const parts = splitPathSegments(relativePath);

        if (parts.length >= 2) {
            const secondLevelDir = parts[1];
            const key = `${parentDir}/${secondLevelDir}`;
            if (!subGroups[key]) {
                subGroups[key] = [];
            }
            subGroups[key].push(skill);
        } else {
            rootFiles.push(skill);
        }
    }

    const result: SkillGroup[] = [];

    if (rootFiles.length > 0) {
        result.push({
            groupKey: parentDir,
            groupLabel: parentDir,
            skills: rootFiles,
            level: 1,
        });
    }

    for (const [subKey, subSkills] of Object.entries(subGroups)) {
        result.push({
            groupKey: subKey,
            groupLabel: subKey,
            skills: subSkills,
            level: 2,
        });
    }

    return result;
};

export const groupSkillsIntelligently = (
    skills: Skill[],
    location: SkillLocation | null,
): SkillGroup[] => {
    if (!location) {
        return [{ groupKey: '', groupLabel: '(root)', skills, level: 0 }];
    }

    const strategy = location.grouping_strategy || {
        mode: 'auto' as const,
        min_files_for_split: 5,
    };
    const mode = strategy.mode || 'auto';
    const minFilesForSplit = strategy.min_files_for_split || 5;
    const result: SkillGroup[] = [];

    if (mode === 'flat') {
        return [{ groupKey: '', groupLabel: 'All Skills', skills, level: 0 }];
    }

    if (mode === 'pattern' && strategy.group_pattern) {
        const patternGroups: Record<string, Skill[]> = {};
        const otherFiles: Skill[] = [];

        for (const skill of skills) {
            const relativePath = getRelativePath(skill, location);
            const parts = splitPathSegments(relativePath);
            const { groupKey, matched } = getGroupKeyFromPattern(
                strategy.group_pattern,
                parts,
            );

            if (matched && groupKey) {
                if (!patternGroups[groupKey]) {
                    patternGroups[groupKey] = [];
                }
                patternGroups[groupKey].push(skill);
            } else {
                otherFiles.push(skill);
            }
        }

        for (const [groupKey, groupSkills] of Object.entries(patternGroups)) {
            if (
                groupSkills.length > minFilesForSplit &&
                shouldSplitIntoSubGroups(groupSkills, location)
            ) {
                result.push(...splitIntoSubGroups(groupSkills, location, groupKey));
            } else {
                result.push({
                    groupKey,
                    groupLabel: groupKey,
                    skills: groupSkills,
                    level: 1,
                });
            }
        }

        if (otherFiles.length > 0) {
            result.push({
                groupKey: '',
                groupLabel: '(other)',
                skills: otherFiles,
                level: 0,
            });
        }

        result.sort((a, b) => {
            if (a.groupKey === '') return 1;
            if (b.groupKey === '') return -1;
            return a.groupKey.localeCompare(b.groupKey);
        });
        return result;
    }

    const firstLevelGroups: Record<string, Skill[]> = {};
    const rootFiles: Skill[] = [];

    for (const skill of skills) {
        const relativePath = getRelativePath(skill, location);
        const parts = splitPathSegments(relativePath);

        if (parts.length === 1) {
            rootFiles.push(skill);
        } else {
            const firstLevelDir = parts[0];
            if (!firstLevelGroups[firstLevelDir]) {
                firstLevelGroups[firstLevelDir] = [];
            }
            firstLevelGroups[firstLevelDir].push(skill);
        }
    }

    if (rootFiles.length > 0) {
        result.push({
            groupKey: '',
            groupLabel: '(root)',
            skills: rootFiles,
            level: 0,
        });
    }

    for (const [dirName, dirSkills] of Object.entries(firstLevelGroups)) {
        if (
            dirSkills.length > minFilesForSplit &&
            shouldSplitIntoSubGroups(dirSkills, location)
        ) {
            result.push(...splitIntoSubGroups(dirSkills, location, dirName));
        } else {
            result.push({
                groupKey: dirName,
                groupLabel: dirName,
                skills: dirSkills,
                level: 1,
            });
        }
    }

    result.sort((a, b) => {
        if (a.level !== b.level) return a.level - b.level;
        if (a.groupKey === '') return 1;
        if (b.groupKey === '') return -1;
        return a.groupKey.localeCompare(b.groupKey);
    });
    return result;
};
