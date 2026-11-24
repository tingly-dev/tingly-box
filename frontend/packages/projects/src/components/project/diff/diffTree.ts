import type { DiffFile } from '../../../client';

/** One row of the tree: a folder (`file === -1`) or a changed file. */
export interface TreeNode {
    /** A file's path, or a folder's path with a trailing `/`: unique, and stable across filtering. */
    id: string;
    /** A folder's label (a chain of single-child folders is one `a/b/c`) or a file's base name. */
    name: string;
    /** Folders above it. */
    depth: number;
    /** Index of the parent folder in `nodes`, -1 at the top. */
    parent: number;
    /** Index just past this node's last descendant (`i + 1` for a file). */
    end: number;
    /** Index into the diff's `files`, -1 for a folder. */
    file: number;
}

/** The changed files as a folder tree. `nodes` is in display order (folders
 * before files, each sorted by name), so a parent always precedes its children
 * and a folder's subtree is the slice `i + 1 .. end`. */
export interface DiffTree {
    nodes: TreeNode[];
    /** Node index of each file, by index into `files`. */
    fileNode: Int32Array;
    /** Node index by `id`. */
    byId: Map<string, number>;
}

interface Trie {
    name: string;
    dirs: Map<string, Trie>;
    files: number[];
}

const byName = (a: string, b: string) => {
    const x = a.toLowerCase();
    const y = b.toLowerCase();
    return x < y ? -1 : x > y ? 1 : a < b ? -1 : a > b ? 1 : 0;
};

/** Builds the tree once per change set: O(files · path depth + n log n). A
 * folder whose only child is another folder is merged with it
 * (`src/components/usage`), as is conventional for deep, thin paths. A rename
 * sits at its new path; the old one stays in the row's tooltip. */
export function buildTree(files: DiffFile[]): DiffTree {
    const root: Trie = { name: '', dirs: new Map(), files: [] };
    files.forEach((file, i) => {
        const parts = file.path.split('/');
        let at = root;
        for (let k = 0; k < parts.length - 1; k++) {
            let next = at.dirs.get(parts[k]);
            if (!next) {
                next = { name: parts[k], dirs: new Map(), files: [] };
                at.dirs.set(parts[k], next);
            }
            at = next;
        }
        at.files.push(i);
    });

    const nodes: TreeNode[] = [];
    const fileNode = new Int32Array(files.length).fill(-1);
    const base = (i: number) => files[i].path.slice(files[i].path.lastIndexOf('/') + 1);

    const emit = (at: Trie, prefix: string, parent: number, depth: number) => {
        const dirs = [...at.dirs.values()].sort((a, b) => byName(a.name, b.name));
        for (let dir of dirs) {
            let name = dir.name;
            while (dir.files.length === 0 && dir.dirs.size === 1) {
                dir = dir.dirs.values().next().value as Trie;
                name += '/' + dir.name;
            }
            const path = `${prefix}${name}/`;
            const index = nodes.length;
            nodes.push({ id: path, name, depth, parent, end: 0, file: -1 });
            emit(dir, path, index, depth + 1);
            nodes[index].end = nodes.length;
        }
        const leaves = at.files.map((i) => ({ i, name: base(i) })).sort((a, b) => byName(a.name, b.name));
        for (const { i, name } of leaves) {
            fileNode[i] = nodes.length;
            nodes.push({ id: files[i].path, name, depth, parent, end: nodes.length + 1, file: i });
        }
    };
    emit(root, '', -1, 0);

    const byId = new Map<string, number>();
    nodes.forEach((n, i) => byId.set(n.id, i));
    return { nodes, fileNode, byId };
}

/** The given files (indexes into `files`) in the order the tree lists them. */
export function treeOrder(tree: DiffTree, indexes: number[]): number[] {
    const keep = new Uint8Array(tree.fileNode.length);
    for (const i of indexes) keep[i] = 1;
    const out: number[] = [];
    for (const n of tree.nodes) if (n.file >= 0 && keep[n.file]) out.push(n.file);
    return out;
}

/** Per node: how many shown files lie at or under it, their changed lines, and
 * how many of them are viewed. A node with `files[i] === 0` is not shown. */
export interface TreeStats {
    files: Int32Array;
    additions: Float64Array;
    deletions: Float64Array;
    viewed: Int32Array;
}

/** Rolls the shown files up into their folders in one reverse pass (children
 * always come after their parent in `nodes`). */
export function treeStats(tree: DiffTree, files: DiffFile[], shown: number[], viewed: Set<string>): TreeStats {
    const n = tree.nodes.length;
    const s: TreeStats = { files: new Int32Array(n), additions: new Float64Array(n), deletions: new Float64Array(n), viewed: new Int32Array(n) };
    for (const i of shown) {
        const at = tree.fileNode[i];
        s.files[at] = 1;
        s.additions[at] = files[i].additions;
        s.deletions[at] = files[i].deletions;
        s.viewed[at] = viewed.has(files[i].path) ? 1 : 0;
    }
    for (let i = n - 1; i >= 0; i--) {
        const p = tree.nodes[i].parent;
        if (p < 0) continue;
        s.files[p] += s.files[i];
        s.additions[p] += s.additions[i];
        s.deletions[p] += s.deletions[i];
        s.viewed[p] += s.viewed[i];
    }
    return s;
}

/** The nodes to draw, top to bottom: those with shown files, minus everything
 * inside a closed folder. Skipping a closed subtree is one jump, so the cost is
 * the rows drawn, not the size of the change. */
export function flattenTree(tree: DiffTree, stats: TreeStats, isOpen: (node: TreeNode) => boolean): number[] {
    const rows: number[] = [];
    const { nodes } = tree;
    let i = 0;
    while (i < nodes.length) {
        const node = nodes[i];
        if (stats.files[i] === 0) i = node.end;
        else {
            rows.push(i);
            i = node.file < 0 && !isOpen(node) ? node.end : i + 1;
        }
    }
    return rows;
}

/** Folder node indexes above `id` (nearest last), so they can be opened to reveal it. */
export function ancestorsOf(tree: DiffTree, id: string): number[] {
    const out: number[] = [];
    let at = tree.byId.get(id);
    while (at != null && at >= 0) {
        at = tree.nodes[at].parent;
        if (at >= 0) out.push(at);
    }
    return out.reverse();
}
