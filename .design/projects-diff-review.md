# Projects diff review

**Review changes** on a thread opens `DiffDialog`: the thread's `base...HEAD`
changes, then **Merge it**. It has to stay usable for a change of any size: a
few hundred files, a 20k-line file, lock files, binaries, whole-directory renames.

## Why it is two requests

The first version sent one `git diff` (`stat` + `patch`, cut at 400 KB) and drew
every line as a DOM node. On a 459-file change that was 499 KB, 17k DOM nodes and
seconds of long tasks, and **7 of the 459 files were visible**: the cap fell
inside the first big lock file and the rest was silently dropped.

- `GET /threads/:id/diff` is a **summary**: one entry per file (`path`,
  `old_path`, `status`, `additions`, `deletions`, `binary`, `generated`) plus the
  totals and the uncommitted notice. One `git diff --raw --numstat -z -M`, no
  content: ~85 bytes a file.
- `GET /threads/:id/diff/file?path=&old_path=` is one file's patch, read when
  that file is opened. **A file that is too large or not text is simply not
  shown, and there is no "load anyway"**: a patch over 512 KB comes back as
  `too_large` with no content (git is stopped at the cap, not read to the end),
  and a binary file as `binary`. The viewer says so and leaves it to git. Paths
  are taken literally (`--literal-pathspecs`), so a name is never read as an
  option or pathspec magic.
- Masking for the extreme cases, so no change can be too big to open:
  - a line longer than 2000 bytes is shortened to its start plus `… [+N chars]`
    (`lines_cut` says how many): one minified or embedded-blob line can outweigh
    the whole file;
  - the summary lists at most 10,000 files (`files_omitted` counts the rest;
    `additions` / `deletions` still cover all). The dialog says the rest is to be
    reviewed with git. Merge is unaffected;
  - the browser keeps the last 6 parsed patches and refetches an older one when
    it is revisited, so paging through huge files does not accumulate memory.
- `generated` is true for lock files, minified/generated code and
  vendored/dist directories (a name list in `diff.go`), and for anything the
  repository marks `linguist-generated` / `linguist-vendored` in `.gitattributes`.

## The view

File list on the left, one file's patch on the right (stacked on a narrow
screen). The list is a **folder tree** or a flat list, switched next to the
filter field and remembered per browser (`diff.listMode` in localStorage,
`DiffListSwitch`). The tree is the default: a flat list of hundreds of files is
a wall of base names with directories squeezed into a suffix, while a tree
groups a change the way it is actually laid out — and on a small change the
tree is nearly flat anyway, so the default costs nothing.

The tree (`diffTree.ts`, `DiffTreeList.tsx`):
- built once per change set, memoized; a folder chain with single-child
  folders collapses into one row (`src/components/usage`), a rename sits at its
  new path;
- each folder rolls up its shown files: count, how many viewed (`2/5`), and the
  +/− totals;
- filters apply to files and keep the ancestor folders of everything that
  matches; a path filter shows its matches unfolded;
- up to 150 files every folder starts open; a bigger change opens only its
  top-level folders, so the tree is an outline to dig into. Folders the user
  toggles are remembered for the dialog's lifetime;
- selecting a file (click, `j`/`k`, the dialog opening) unfolds the folders
  above it before the rows below are computed, so there is no flash;
- the visible rows come from one `flattenTree` pass that jumps over closed
  subtrees and file-less folders, then `useVirtualWindow` windows them like the
  flat list: a 10,000-file change builds in a few ms and draws a screenful.
- Files in tree mode are walked in the tree's order, so `j` / `k`, the
  prefetch and the previous/next buttons follow what is on screen.
- The tree is an ARIA tree (one tab stop, `aria-activedescendant`):
  `↑`/`↓` move, `→` opens a folder or enters it, `←` closes it or goes to the
  parent, `Enter`/`Space` toggles a folder or opens a file; `j` / `k` / `v`
  keep working as in the flat list.
- A file row shows a status icon by how it changed (added / modified /
  deleted / renamed) instead of the flat list's letter.

- Nothing is fetched or drawn for a file until it is opened; the next file is
  prefetched. The list and the patch are both windowed (`useVirtualWindow`,
  equal-height rows), so cost follows the screen, not the change size.
- The dialog opens on the first file that opens by itself (not generated, binary or
over 20,000 changed lines). A generated file shows a folded notice with **Show
diff** (small ones only: a big lock file is over the size cap anyway). A file
over 20,000 changed lines, a patch the server refuses for its size, and a binary
show a plain note with no button, and the first and last are not requested at all.
- Filter by path, **Hide generated**, **Unviewed only**. **Viewed** checkboxes
  are kept per thread in localStorage and tied to the file's change
  (`status:+:-:old_path`), so a file that changed again is unviewed again.
- Keys: `j` / `k` previous / next file, `v` mark viewed and go to the next
  unviewed one. Ignored while typing in an input.
- Rows are plain elements styled from the scroller's single `sx` (class
  selectors), not a MUI component per cell: mounting dozens of rows per scroll
  frame was the remaining cost. Colours come from the theme palette.

Not done: a split (side-by-side) view, syntax highlighting, word-level
intra-line highlights.
