import type { ReactElement } from 'react';

// Shared scaffolding for the catalog dialogs' category sidebars. The tables
// themselves (which categories exist, their labels and icons) stay per-dialog
// — only the lookup-with-fallback shape is shared.

export interface CategoryMeta {
    label: string;
    icon: ReactElement;
}

// makeCategoryMeta builds a category→meta resolver over a dialog-specific
// table, falling back to `fallback` for categories the table doesn't know
// (e.g. a category the backend added that the frontend hasn't mapped yet).
export function makeCategoryMeta(
    table: Record<string, CategoryMeta>,
    fallback: (category: string) => CategoryMeta,
): (category: string) => CategoryMeta {
    return (category) => table[category] ?? fallback(category);
}
