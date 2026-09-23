import { useState } from 'react';

interface DeleteConfirmState {
    open: boolean;
    rowId: string;
    rowName: string;
}

// useDeleteConfirm: tracks the row pending deletion for a confirm dialog —
// `open` stores the row id and resolves its display name via `getName`;
// `confirm` forwards the id to `onDelete` before closing.
export function useDeleteConfirm(
    onDelete: ((rowId: string) => void) | undefined,
    getName: (rowId: string) => string | undefined,
) {
    const [state, setState] = useState<DeleteConfirmState>({
        open: false,
        rowId: "",
        rowName: "",
    });

    const open = (rowId: string) => {
        setState({open: true, rowId, rowName: getName(rowId) ?? "Unknown Provider"});
    };

    const close = () => {
        setState({open: false, rowId: "", rowName: ""});
    };

    const confirm = () => {
        if (onDelete && state.rowId) {
            onDelete(state.rowId);
        }
        close();
    };

    return { state, open, close, confirm };
}
