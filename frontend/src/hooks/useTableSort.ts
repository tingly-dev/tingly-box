import { useState } from 'react';

export type SortOrder = 'asc' | 'desc';

// useTableSort: state + click handler for sortable table headers. Clicking the
// active column toggles asc/desc; a new column starts desc when listed in
// `descFields` (time-like columns), asc otherwise.
export function useTableSort<Field extends string>(defaultField: Field, descFields: readonly string[] = ['time']) {
    const [sortField, setSortField] = useState<Field>(defaultField);
    const [sortOrder, setSortOrder] = useState<SortOrder>(descFields.includes(defaultField) ? 'desc' : 'asc');

    const handleSort = (field: Field) => {
        if (sortField === field) {
            // Toggle between asc/desc
            setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc');
        } else {
            // New field, default to desc for time, asc for others
            setSortField(field);
            setSortOrder(descFields.includes(field) ? 'desc' : 'asc');
        }
    };

    return { sortField, sortOrder, handleSort };
}
