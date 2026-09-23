// Pure text-list helpers shared by the policy editor dialog and payload
// building. Module-level functions (previously re-created inside the page
// component on every render).
import {
    MAX_INLINE_LIST_CHARS,
    MAX_INLINE_LIST_ITEMS,
    MAX_SUMMARY_CHARS,
    MAX_SUMMARY_VALUES,
    OVERSIZED_LIST_PREVIEW_ITEMS,
    type EditorListField,
    type OversizedListField,
} from './types';

export const splitLines = (value: string) =>
    value
        .split('\n')
        .map((item) => item.trim())
        .filter(Boolean);

export const textListRows = (value: string) => {
    const rows = value.split('\n');
    if (rows.length === 0) {
        return [''];
    }
    if (rows.length === 1 && rows[0] === '') {
        return [''];
    }
    return rows;
};

export const joinLines = (values?: string[]) => (Array.isArray(values) ? values.join('\n') : '');

export const isOversizedList = (values?: string[]) => {
    if (!Array.isArray(values) || values.length === 0) {
        return false;
    }
    if (values.length > MAX_INLINE_LIST_ITEMS) {
        return true;
    }
    let chars = 0;
    for (const value of values) {
        chars += value.length + 1;
        if (chars > MAX_INLINE_LIST_CHARS) {
            return true;
        }
    }
    return false;
};

export const prepareListField = (values?: string[]) => {
    const list = Array.isArray(values) ? values.filter(Boolean) : [];
    if (!isOversizedList(list)) {
        return { text: joinLines(list), oversized: undefined };
    }
    return {
        text: joinLines(list.slice(0, OVERSIZED_LIST_PREVIEW_ITEMS)),
        oversized: {
            values: list,
            preview: list.slice(0, OVERSIZED_LIST_PREVIEW_ITEMS),
            total: list.length,
        } satisfies OversizedListField,
    };
};

export const summarizeValues = (values?: string[], emptyLabel = 'none') => {
    const list = Array.isArray(values) ? values.filter(Boolean) : [];
    if (list.length === 0) {
        return emptyLabel;
    }
    const preview = list.slice(0, MAX_SUMMARY_VALUES).join(', ');
    const suffix = list.length > MAX_SUMMARY_VALUES ? ` (+${list.length - MAX_SUMMARY_VALUES} more)` : '';
    const text = `${preview}${suffix}`;
    if (text.length <= MAX_SUMMARY_CHARS) {
        return text;
    }
    return `${text.slice(0, MAX_SUMMARY_CHARS - 1)}…`;
};

// Resolves the effective value list for an editor field: rows the user typed
// in front of the read-only preview of an oversized list, plus the preserved
// tail that stays out of the text area.
export const effectiveListValues = (
    oversized: Partial<Record<EditorListField, OversizedListField>> | undefined,
    field: EditorListField,
    rawValue: string
) => {
    const oversizedField = oversized?.[field];
    if (!oversizedField) {
        return splitLines(rawValue);
    }
    const rawRows = textListRows(rawValue);
    const editablePrefixCount = Math.max(0, rawRows.length - oversizedField.preview.length);
    const additions = rawRows
        .slice(0, editablePrefixCount)
        .map((item) => item.trim())
        .filter(Boolean);
    return [...additions, ...oversizedField.values];
};

export const toggleValue = (values: string[], value: string) => {
    if (values.includes(value)) {
        return values.filter((item) => item !== value);
    }
    return [...values, value];
};

export const updateTextListValue = (value: string, index: number, nextItem: string) => {
    const items = textListRows(value);
    while (items.length <= index) {
        items.push('');
    }
    items[index] = nextItem;
    return items.join('\n');
};

export const appendTextListValue = (value: string) => {
    const items = textListRows(value);
    items.push('');
    return items.join('\n');
};

export const removeTextListValue = (value: string, index: number) => {
    const items = textListRows(value);
    if (index < 0 || index >= items.length) {
        return value;
    }
    items.splice(index, 1);
    if (items.length === 0) {
        return '';
    }
    return items.join('\n');
};
