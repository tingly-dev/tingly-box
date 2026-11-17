import { describe, expect, it } from 'vitest';
import type { DiffFile } from '../../../client';
import { ancestorsOf, buildTree, flattenTree, treeOrder, treeStats } from './diffTree';

const file = (path: string, extra: Partial<DiffFile> = {}): DiffFile => ({ path, status: 'modified', additions: 2, deletions: 1, ...extra });
const files = (...fs: DiffFile[]) => fs;

describe('buildTree', () => {
    it('groups files by folder and orders folders before files, each by name', () => {
        const tree = buildTree(files(file('src/b.ts'), file('README.md'), file('src/a.ts'), file('docs/guide.md')));
        const shown = tree.nodes.map((n) => n.id);
        expect(shown).toEqual(['docs/', 'docs/guide.md', 'src/', 'src/a.ts', 'src/b.ts', 'README.md']);
        expect(tree.nodes.map((n) => n.depth)).toEqual([0, 1, 0, 1, 1, 0]);
        expect(tree.nodes.map((n) => n.file)).toEqual([-1, 3, -1, 2, 0, 1]);
    });

    it('compacts a chain of single-child folders into one row', () => {
        const tree = buildTree(files(file('src/components/usage/Card.tsx'), file('src/main.ts')));
        const dirs = tree.nodes.filter((n) => n.file < 0).map((n) => n.name);
        expect(dirs).toEqual(['src', 'components/usage']);
        expect(tree.byId.get('src/components/usage/')).toBeDefined();
    });

    it('does not compact a folder that also holds files, and not two sibling folders', () => {
        const tree = buildTree(files(file('a/b/c.ts'), file('a/b/d.ts'), file('a/x.ts'), file('a/y/z.ts')));
        // a/b has two files; a/y is a single-child chain inside a, which also holds x.ts.
        expect(tree.nodes.filter((n) => n.file < 0).map((n) => n.name)).toEqual(['a', 'b', 'y']);
    });

    it('keeps a renamed file at its new path', () => {
        const tree = buildTree(files(file('new/loc.ts', { status: 'renamed', old_path: 'old/loc.ts' })));
        expect(tree.nodes.map((n) => n.id)).toEqual(['new/', 'new/loc.ts']);
    });

    it('is consistent: parents precede children, ends bracket subtrees, fileNode and byId agree', () => {
        const fs = files(file('a/b/c/d.ts'), file('a/b/e.ts'), file('a/f.ts'), file('top.ts'), file('a/b/c/g.ts'));
        const tree = buildTree(fs);
        tree.nodes.forEach((n, i) => {
            if (n.parent >= 0) expect(n.parent).toBeLessThan(i);
            expect(n.end).toBeGreaterThan(i);
            // The slice is the subtree: everything in it sits below `n`.
            for (let k = i + 1; k < n.end; k++) expect(tree.nodes[k].parent).toBeGreaterThanOrEqual(i);
        });
        tree.fileNode.forEach((node, fi) => expect(tree.nodes[node].id).toBe(fs[fi].path));
        tree.byId.forEach((i, id) => expect(tree.nodes[i].id).toBe(id));
    });

    it('builds a 10,000-file tree in well under a frame budget', () => {
        const fs = Array.from({ length: 10000 }, (_, i) => file(`dir${i % 50}/sub${i % 7}/f${i}.ts`));
        const t0 = performance.now();
        const tree = buildTree(fs);
        const buildMs = performance.now() - t0;
        expect(tree.nodes.length).toBeGreaterThan(fs.length);
        const stats = treeStats(tree, fs, fs.map((_, k) => k), new Set());
        const t1 = performance.now();
        const rows = flattenTree(tree, stats, () => true);
        const flatMs = performance.now() - t1;
        expect(rows.length).toBe(fs.length + 50 + 50 * 7);
        expect(buildMs).toBeLessThan(250);
        expect(flatMs).toBeLessThan(50);
    });
});

describe('treeOrder', () => {
    it('walks the kept files in tree order, whatever the input order', () => {
        const tree = buildTree(files(file('src/b.ts'), file('README.md'), file('src/a.ts')));
        expect(treeOrder(tree, [0, 1, 2])).toEqual([2, 0, 1]);
        expect(treeOrder(tree, [2, 0])).toEqual([2, 0]);
    });
});

describe('treeStats', () => {
    it('rolls shown files up into their folders and skips hidden ones', () => {
        const fs = files(file('a/x.ts', { additions: 10, deletions: 2 }), file('a/sub/y.ts', { additions: 3, deletions: 4 }), file('b/z.ts', { additions: 1, deletions: 1 }));
        const tree = buildTree(fs);
        const shown = [0, 1]; // b/z.ts filtered out
        const s = treeStats(tree, fs, shown, new Set(['a/x.ts']));
        const dir = tree.byId.get('a/') as number;
        const sub = tree.byId.get('a/sub/') as number;
        expect(s.files[dir]).toBe(2);
        expect(s.additions[dir]).toBe(13);
        expect(s.deletions[dir]).toBe(6);
        expect(s.viewed[dir]).toBe(1);
        expect(s.viewed[sub]).toBe(0);
        const b = tree.byId.get('b/') as number;
        expect(s.files[b]).toBe(0);
    });
});

describe('flattenTree', () => {
    const fs = files(file('a/x.ts'), file('a/sub/y.ts'), file('top.ts'));
    const tree = buildTree(fs);
    const stats = () => treeStats(tree, fs, [0, 1, 2], new Set());

    it('lists folders before their files', () => {
        const rows = flattenTree(tree, stats(), () => true);
        expect(rows.map((i) => tree.nodes[i].id)).toEqual(['a/', 'a/sub/', 'a/sub/y.ts', 'a/x.ts', 'top.ts']);
    });

    it('skips a closed folder and its whole subtree in one step, and hides empty folders', () => {
        const a = tree.byId.get('a/') as number;
        const closed = flattenTree(tree, stats(), (n) => n.id !== 'a/');
        expect(closed.map((i) => tree.nodes[i].id)).toEqual(['a/', 'top.ts']);
        expect(tree.nodes[closed[0]].end).toBeGreaterThan(a);
        const onlyB = treeStats(tree, fs, [2], new Set());
        expect(flattenTree(tree, onlyB, () => true).map((i) => tree.nodes[i].id)).toEqual(['top.ts']);
    });
});

describe('ancestorsOf', () => {
    it('lists the folders above a file, outermost first', () => {
        const tree = buildTree(files(file('a/b/c.ts'), file('d.ts')));
        // `a` holds only the folder `b`, so the chain is the one row `a/b`.
        expect(ancestorsOf(tree, 'a/b/c.ts').map((i) => tree.nodes[i].id)).toEqual(['a/b/']);
        expect(ancestorsOf(tree, 'd.ts')).toEqual([]);
    });
});
