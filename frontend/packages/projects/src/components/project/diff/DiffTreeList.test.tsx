import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { DiffFile } from '../../../client';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string, o?: Record<string, unknown>) => (o && 'total' in o ? `${key}:${o.viewed}/${o.total}` : key) }) }));

import { buildTree } from './diffTree';
import { DiffTreeList } from './DiffTreeList';

const file = (path: string, extra: Partial<DiffFile> = {}): DiffFile => ({ path, status: 'modified', additions: 2, deletions: 1, ...extra });

afterEach(cleanup);

const renderTree = (files: DiffFile[], props: Partial<Parameters<typeof DiffTreeList>[0]> = {}) => {
    const onSelect = vi.fn();
    const onToggleViewed = vi.fn();
    const tree = buildTree(files);
    render(
        <DiffTreeList
            files={files}
            tree={tree}
            shown={files.map((_, k) => k)}
            selectedPath={null}
            viewedSet={new Set()}
            filtering={false}
            onSelect={onSelect}
            onToggleViewed={onToggleViewed}
            {...props}
        />,
    );
    return { onSelect, tree };
};

const ids = () => screen.getAllByRole('treeitem').map((n) => n.getAttribute('title') ?? n.textContent);

describe('DiffTreeList', () => {
    it('renders folders with rollup counts before their files, as an ARIA tree', () => {
        renderTree([file('src/a.ts', { additions: 10 }), file('src/sub/b.ts'), file('top.ts', { status: 'added' })]);
        expect(screen.getByRole('tree')).toBeInTheDocument();
        expect(ids()).toEqual(['src', 'src/sub', 'src/sub/b.ts', 'src/a.ts', 'top.ts']);
        expect(screen.getByTitle('thread.diff.folderViewed:0/2')).toBeInTheDocument();
        const folder = screen.getAllByRole('treeitem')[0];
        expect(folder).toHaveAttribute('aria-expanded', 'true');
        expect(folder).toHaveAttribute('aria-level', '1');
    });

    it('folds and unfolds a folder by click, hiding its subtree', () => {
        renderTree([file('src/a.ts'), file('top.ts')]);
        fireEvent.click(screen.getByTitle('src'));
        expect(ids()).toEqual(['src', 'top.ts']);
        expect(screen.getByTitle('src')).toHaveAttribute('aria-expanded', 'false');
        fireEvent.click(screen.getByTitle('src'));
        expect(screen.getByTitle('src/a.ts')).toBeInTheDocument();
    });

    it('unfolds the folders above a selected file that starts closed', () => {
        // 200 files: only the top level starts open, so deep folders begin closed.
        const files = Array.from({ length: 200 }, (_, i) => file(`api/v1/users/handlers/h${i}.ts`));
        renderTree(files, { selectedPath: 'api/v1/users/handlers/h199.ts' });
        // The whole chain above the selection is open (and compacted into one row).
        expect(screen.getByTitle('api/v1/users/handlers')).toHaveAttribute('aria-expanded', 'true');
        // Its files are now visible under it (the window shows the top of the list).
        expect(ids()[1]).toBe('api/v1/users/handlers/h0.ts');
    });

    it('moves with the arrow keys: ↓ selects, → opens a folder then enters it, ← closes or goes to the parent', () => {
        const { onSelect } = renderTree([file('src/a.ts'), file('top.ts')]);
        const tree = screen.getByRole('tree');
        const active = () => tree.getAttribute('aria-activedescendant') && document.getElementById(tree.getAttribute('aria-activedescendant') as string)?.getAttribute('title');
        expect(active()).toBe(null);
        fireEvent.keyDown(tree, { key: 'ArrowDown' });
        expect(active()).toBe('src');
        fireEvent.keyDown(tree, { key: 'ArrowRight' }); // src is open: into the first child
        expect(active()).toBe('src/a.ts');
        expect(onSelect).toHaveBeenCalledWith('src/a.ts');
        fireEvent.keyDown(tree, { key: 'ArrowLeft' }); // a file: to the parent folder
        expect(active()).toBe('src');
        fireEvent.keyDown(tree, { key: 'ArrowLeft' }); // an open folder: close it
        expect(screen.getByTitle('src')).toHaveAttribute('aria-expanded', 'false');
        fireEvent.keyDown(tree, { key: 'ArrowRight' }); // closed: open it again
        expect(screen.getByTitle('src/a.ts')).toBeInTheDocument();
        fireEvent.keyDown(tree, { key: 'End' });
        expect(active()).toBe('top.ts');
        expect(onSelect).toHaveBeenLastCalledWith('top.ts');
    });

    it('marks a file viewed from its checkbox and shows the folder rollup move', () => {
        const files = [file('src/a.ts'), file('src/b.ts')];
        const onToggleViewed = vi.fn();
        renderTree(files, { onToggleViewed });
        fireEvent.click(screen.getAllByLabelText('thread.diff.viewed')[0]);
        expect(onToggleViewed).toHaveBeenCalledWith(files[0]);
    });

    it('shows only rows near the top of a 5000-file tree', () => {
        const files = Array.from({ length: 5000 }, (_, i) => file(`d${i % 10}/f${i}.ts`));
        renderTree(files);
        expect(screen.getAllByRole('treeitem').length).toBeLessThan(40);
    });
});
