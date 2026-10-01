import { describe, expect, it } from 'vitest';
import { activeMentionQuery, composeEntities, findMentions } from './composeEntities';
import type { ImageEntity } from './entityTypes';

const entity = (name: string, kind: ImageEntity['kind'], refs: number, extra: Partial<ImageEntity> = {}): ImageEntity => ({
    id: name,
    kind,
    name,
    refs: Array.from({ length: refs }, (_, i) => `${name}-${i}`),
    prompt: `${name}的描述`,
    uses: 0,
    updatedAt: 0,
    ...extra,
});

const linxia = entity('林夏', 'character', 4);
const ayan = entity('阿岩', 'character', 3);
const film = entity('胶片人像', 'style', 2, { negative: '塑料皮肤' });
const filmNight = entity('胶片', 'style', 1);
const all = [linxia, ayan, film, filmNight];

describe('findMentions', () => {
    it('matches names that run straight into the next word', () => {
        expect(findMentions('@林夏坐在门口', all).map((m) => m.entity.name)).toEqual(['林夏']);
    });

    it('prefers the longest matching name', () => {
        expect(findMentions('风格 @胶片人像', all).map((m) => m.entity.name)).toEqual(['胶片人像']);
    });

    it('ignores an @ that names nothing', () => {
        expect(findMentions('邮件 a@b.com', all)).toEqual([]);
    });
});

describe('activeMentionQuery', () => {
    it('returns the query being typed', () => {
        expect(activeMentionQuery('坐在 @胶', 5)).toEqual({ start: 3, query: '胶' });
    });

    it('closes once the mention is followed by a space', () => {
        expect(activeMentionQuery('@林夏 坐', 5)).toBeNull();
    });
});

describe('composeEntities', () => {
    it('gives every entity its first reference before anyone gets a second', () => {
        const result = composeEntities('@林夏 和 @阿岩 @胶片人像', all, 5, 0);
        expect(result.allocations.map((a) => a.taken)).toEqual([2, 2, 1]);
        expect(result.truncated).toBe(true);
    });

    it('leaves the slots the user filled by hand alone', () => {
        const result = composeEntities('@林夏', all, 5, 3);
        expect(result.budget).toBe(2);
        expect(result.allocations[0].taken).toBe(2);
    });

    it('expands characters in place and moves styles to the end', () => {
        const result = composeEntities('@林夏坐在门口 @胶片人像', all, 5, 0);
        expect(result.expandedPrompt).toBe('林夏（林夏的描述）坐在门口\n画面风格：胶片人像的描述');
        expect(result.negative).toBe('塑料皮肤');
    });

    it('counts an entity mentioned twice once', () => {
        expect(composeEntities('@林夏 看着 @林夏', all, 5, 0).entities).toHaveLength(1);
    });
});
