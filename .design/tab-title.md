# Browser tab title

The tab's `document.title` says where the user is, most specific focus first,
then the product and the site it is served from:

```
<focus> · <context> · Tingly Box · <host:port>
```

## Rules

| Surface | Title |
| --- | --- |
| Any shell page | nav row name: `Usage · Tingly Box · localhost:12580`, `Model Key · …`, a profile/team by its own name |
| Bots pages | `Bots · …` |
| Workspace home / project list / IM settings | `Workspace · …`, `Projects · …`, IM settings title |
| Project, thread open | `#12 Fix login · tb-ui · …` (`threadRef`: number then title) |
| Project, no thread | `Coordinator · tb-ui · …` |
| Folder, session open | `#4 title · <folder name> · …`; folder list: `Folders · …` |
| Anything waits on the user | the title starts with `● ` |

- Product name is read once from `index.html`'s `<title>` (Tingly Box, or
  Tingly Project standalone). Strings follow the app language (they come from
  the same i18n keys as the nav and the Projects pages).
- Site = `location.host`, left out in the desktop (Wails) window where it is
  meaningless. Two instances (hosts or ports) therefore never share a title.
  No new setting or persistence; a user-set site name is possible later but
  needs a config field, so it is not part of this change.
- Each part is cut at 40 characters (`…`), so the suffix always survives.
- The attention mark is a single `●`, driven by the same "something waits"
  boolean as the rail dot (polled, not counted), so the tab never flickers with
  changing numbers.
- Open question, not done: a favicon per state/instance.

## Wiring

`useDocumentTitle(parts, priority)` and `setTitleAttention(on)` live in
`@tingly/ui` (`utils/documentTitle.ts`), used by both the host and the Projects
package. Entries register by priority: the shell's page label (`Layout`, via
`layout/pageTitle.ts`) is priority 0; a page with a finer focus (Projects
pages) uses 1 and wins while mounted; when it unmounts the shell's label
returns.
