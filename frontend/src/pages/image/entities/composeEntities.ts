import type { ImageEntity } from './entityTypes';

export interface EntityMention {
    entity: ImageEntity;
    // Character offsets of the whole `@name` token in the prompt.
    start: number;
    end: number;
}

// Find every `@name` that names an entity. Chinese prompts run the name
// straight into the next word (`@林夏坐在…`), so a mention ends where the
// longest matching entity name ends, not at the next space.
export const findMentions = (prompt: string, entities: ImageEntity[]): EntityMention[] => {
    const byLength = [...entities].sort((a, b) => b.name.length - a.name.length);
    const mentions: EntityMention[] = [];
    for (let index = prompt.indexOf('@'); index !== -1; index = prompt.indexOf('@', index + 1)) {
        const entity = byLength.find((item) => item.name && prompt.startsWith(item.name, index + 1));
        if (entity) mentions.push({ entity, start: index, end: index + 1 + entity.name.length });
    }
    return mentions;
};

// The `@query` being typed right before the caret, if any — what the mention
// picker filters on.
export const activeMentionQuery = (prompt: string, caret: number): { start: number; query: string } | null => {
    const before = prompt.slice(0, caret);
    const at = before.lastIndexOf('@');
    if (at === -1) return null;
    const query = before.slice(at + 1);
    if (/[\s@，。,.!?！？；;：:]/.test(query) || query.length > 12) return null;
    return { start: at, query };
};

export interface RefAllocation {
    entity: ImageEntity;
    taken: number;
    available: number;
}

export interface Composition {
    // Referenced entities, first mention order, each once.
    entities: ImageEntity[];
    allocations: RefAllocation[];
    // The reference slots the entities may use (the run's limit minus the
    // images the user attached by hand — those were chosen explicitly).
    budget: number;
    used: number;
    // True when some entity had to give up images: a default was applied,
    // so it is reported, not hidden.
    truncated: boolean;
    // What the model actually receives.
    expandedPrompt: string;
    negative: string;
}

// Split the budget round-robin in mention order, so every entity gets at
// least its first (best) reference before anyone gets a second.
const allocate = (entities: ImageEntity[], budget: number): RefAllocation[] => {
    const allocations = entities.map((entity) => ({ entity, taken: 0, available: entity.refs.length }));
    let left = budget;
    let progressed = true;
    while (left > 0 && progressed) {
        progressed = false;
        for (const allocation of allocations) {
            if (left === 0) break;
            if (allocation.taken < allocation.available) {
                allocation.taken += 1;
                left -= 1;
                progressed = true;
            }
        }
    }
    return allocations;
};

export const composeEntities = (
    prompt: string,
    allEntities: ImageEntity[],
    limit: number,
    manualRefs: number,
): Composition => {
    const mentions = findMentions(prompt, allEntities);
    const entities: ImageEntity[] = [];
    for (const mention of mentions) {
        if (!entities.includes(mention.entity)) entities.push(mention.entity);
    }
    const budget = Math.max(0, limit - manualRefs);
    const allocations = allocate(entities, budget);
    const used = allocations.reduce((sum, item) => sum + item.taken, 0);

    // Characters expand in place — "林夏（…）坐在" keeps the sentence intact.
    // Styles describe the whole picture, so their text moves to the end and
    // the mention itself is dropped.
    let expanded = '';
    let cursor = 0;
    for (const mention of mentions) {
        expanded += prompt.slice(cursor, mention.start);
        if (mention.entity.kind === 'character') {
            expanded += `${mention.entity.name}（${mention.entity.prompt}）`;
        }
        cursor = mention.end;
    }
    expanded += prompt.slice(cursor);
    const styles = entities.filter((entity) => entity.kind === 'style');
    expanded = expanded.replace(/\s+/g, ' ').trim();
    if (styles.length) {
        expanded = [expanded, ...styles.map((style) => `画面风格：${style.prompt}`)].filter(Boolean).join('\n');
    }
    const negative = entities.map((entity) => entity.negative).filter(Boolean).join('；');

    return {
        entities,
        allocations,
        budget,
        used,
        truncated: allocations.some((item) => item.taken < item.available),
        expandedPrompt: expanded,
        negative,
    };
};
