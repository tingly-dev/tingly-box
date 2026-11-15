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
- `GET /threads/:id/diff/file?path=&old_path=&force=` is one file's patch,
  read when that file is opened. A patch over 512 KB is not sent (`too_large`)
  unless `force=true`, which reads up to 32 MB (`truncated` past that). Git is
  stopped at the cap, not read to the end. Binary files have no patch. Paths are
  taken literally (`--literal-pathspecs`), so a name is never read as an option
  or pathspec magic.
- `generated` is true for lock files, minified/generated code and
  vendored/dist directories (a name list in `diff.go`), and for anything the
  repository marks `linguist-generated` / `linguist-vendored` in `.gitattributes`.

## The view

File list on the left, one file's patch on the right (stacked on a narrow
screen).

- Nothing is fetched or drawn for a file until it is opened; the next file is
  prefetched. The list and the patch are both windowed (`useVirtualWindow`,
  equal-height rows), so cost follows the screen, not the change size.
- The dialog opens on the first file that is not generated or binary.
- A generated file shows a folded notice with **Show diff**; a file over 4000
  changed lines shows **Large diff · Load anyway**; a binary shows a note.
  The backend's own cap gets the same button.
- Filter by path, **Hide generated**, **Unviewed only**. **Viewed** checkboxes
  are kept per thread in localStorage and tied to the file's change
  (`status:+:-:old_path`), so a file that changed again is unviewed again.
- Keys: `j` / `k` previous / next file, `v` mark viewed and go to the next
  unviewed one. Ignored while typing in an input.
- Rows are plain elements styled from the scroller's single `sx` (class
  selectors), not a MUI component per cell: mounting dozens of rows per scroll
  frame was the remaining cost. Colours come from the theme palette.

Not done: a split (side-by-side) view, syntax highlighting, word-level
intra-line highlights, a directory tree.
