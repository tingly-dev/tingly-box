import { useCallback, useMemo, useState } from 'react';
import type { ImageEntity } from './entityTypes';
import { activeMentionQuery } from './composeEntities';

interface Options {
    prompt: string;
    setPrompt: (value: string) => void;
    inputRef: React.RefObject<HTMLTextAreaElement | null>;
    entities: ImageEntity[];
}

// `@` in the prompt opens a picker of entities filtered by what follows it.
// The text stays plain text — `@林夏` is what is stored and what the user
// can copy — the picker only saves typing the name exactly.
export const useEntityMention = ({ prompt, setPrompt, inputRef, entities }: Options) => {
    const [mention, setMention] = useState<{ start: number; query: string } | null>(null);
    const [activeIndex, setActiveIndex] = useState(0);

    const candidates = useMemo(() => {
        if (!mention) return [];
        const query = mention.query.trim().toLowerCase();
        return entities
            .filter((entity) => !query || entity.name.toLowerCase().includes(query))
            .sort((a, b) => Number(!a.name.toLowerCase().startsWith(query)) - Number(!b.name.toLowerCase().startsWith(query))
                || b.uses - a.uses);
    }, [entities, mention]);

    const sync = useCallback((value: string, caret: number | null) => {
        const next = caret === null ? null : activeMentionQuery(value, caret);
        setMention(next);
        setActiveIndex(0);
    }, []);

    const handleChange = useCallback((event: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
        setPrompt(event.target.value);
        sync(event.target.value, event.target.selectionStart);
    }, [setPrompt, sync]);

    const close = useCallback(() => setMention(null), []);

    // The `@query` span being typed, so a caller can replace it later (after
    // a "new entity" dialog has taken the focus away).
    const range = useCallback(() => {
        if (!mention) return null;
        return { start: mention.start, end: mention.start + 1 + mention.query.length };
    }, [mention]);

    const select = useCallback((entity: ImageEntity) => {
        const input = inputRef.current;
        const caret = input?.selectionStart ?? prompt.length;
        const start = mention?.start ?? caret;
        const inserted = `@${entity.name} `;
        const next = prompt.slice(0, start) + inserted + prompt.slice(caret);
        setPrompt(next);
        setMention(null);
        requestAnimationFrame(() => {
            if (!input) return;
            input.focus();
            const position = start + inserted.length;
            input.setSelectionRange(position, position);
        });
    }, [inputRef, mention, prompt, setPrompt]);

    // Starts a mention from a button: drops an `@` at the caret and opens
    // the picker, exactly as if it had been typed.
    const begin = useCallback(() => {
        const input = inputRef.current;
        const caret = input?.selectionStart ?? prompt.length;
        const needsSpace = caret > 0 && !/\s$/.test(prompt.slice(0, caret));
        const insert = `${needsSpace ? ' ' : ''}@`;
        const next = prompt.slice(0, caret) + insert + prompt.slice(caret);
        setPrompt(next);
        setMention({ start: caret + insert.length - 1, query: '' });
        setActiveIndex(0);
        requestAnimationFrame(() => {
            if (!input) return;
            input.focus();
            const position = caret + insert.length;
            input.setSelectionRange(position, position);
        });
    }, [inputRef, prompt, setPrompt]);

    // Returns true when the key belonged to the picker.
    const handleKeyDown = useCallback((event: React.KeyboardEvent) => {
        if (!mention) return false;
        if (event.key === 'Escape') {
            event.preventDefault();
            setMention(null);
            return true;
        }
        if (!candidates.length) return false;
        if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
            event.preventDefault();
            const step = event.key === 'ArrowDown' ? 1 : -1;
            setActiveIndex((index) => (index + step + candidates.length) % candidates.length);
            return true;
        }
        if ((event.key === 'Enter' && !event.metaKey && !event.ctrlKey) || event.key === 'Tab') {
            event.preventDefault();
            select(candidates[Math.min(activeIndex, candidates.length - 1)]);
            return true;
        }
        return false;
    }, [activeIndex, candidates, mention, select]);

    return {
        open: mention !== null,
        query: mention?.query ?? '',
        candidates,
        activeIndex,
        setActiveIndex,
        handleChange,
        handleKeyDown,
        select,
        begin,
        close,
        range,
        sync,
    };
};
