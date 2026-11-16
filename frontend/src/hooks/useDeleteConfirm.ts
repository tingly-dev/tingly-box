import { useState } from 'react';

interface DeleteConfirmState {
    open: boolean;
    anchorEl: HTMLElement | null;
    rowId: string;
    rowName: string;
}

// useDeleteConfirm: tracks the row pending deletion for a ConfirmPopover —
// `open` stores the row id, the element to anchor at and resolves its display name via `getName`;
// `confirm` forwards the id to `onDelete` before closing.
export function useDeleteConfirm(
    onDelete: ((rowId: string) => void) | undefined,
    getName: (rowId: string) => string | undefined,
) {
    const [state, setState] = useState<DeleteConfirmState>({
        open: false,
        anchorEl: null,
        rowId: "",
        rowName: "",
    });

    const open = (rowId: string, anchorEl: HTMLElement | null) => {
        setState({open: anchorEl !== null, anchorEl, rowId, rowName: getName(rowId) ?? "Unknown Provider"});
    };

    const close = () => {
        setState({open: false, anchorEl: null, rowId: "", rowName: ""});
    };

    const confirm = () => {
        if (onDelete && state.rowId) {
            onDelete(state.rowId);
        }
        close();
    };

    return { state, open, close, confirm };
}
