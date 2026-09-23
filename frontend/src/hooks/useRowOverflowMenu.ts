import { useState } from 'react';
import type React from 'react';

// useRowOverflowMenu: anchor state for a per-row "more actions" Menu. Open it
// from the row's overflow button, then read `menu.rowId` to render the current
// row's menu items.
export function useRowOverflowMenu() {
    const [menu, setMenu] = useState<{
        anchorEl: HTMLElement | null;
        rowId: string;
    }>({
        anchorEl: null,
        rowId: "",
    });

    const openMenu = (e: React.MouseEvent<HTMLElement>, rowId: string) => {
        e.stopPropagation();
        setMenu({anchorEl: e.currentTarget, rowId});
    };
    const closeMenu = () => setMenu({anchorEl: null, rowId: ""});

    return { menu, openMenu, closeMenu };
}
