# Browser tab title

The tab's `document.title` tells tabs apart without getting long:

```
[<focus> · ]Tingly Box · <host:port>
```

| Surface | Title |
| --- | --- |
| Ordinary pages (Usage, Credentials, Bots, …) | `Tingly Box · localhost:12580` — unchanged apart from the host |
| Workspace home, project list, IM settings | `Workspace · Tingly Box · host` |
| A project (coordinator or any thread open) | `tb-ui · Tingly Box · host` |
| A working folder | `<folder name> · Tingly Box · host` (list: `Folders`) |
| Something waits on the user | the title starts with `● ` |

Deliberately not in the title: per-page names for ordinary pages, thread
numbers/titles, counts. The tab is for telling sites and projects apart; the
page itself says the rest, and a short title stays readable in a narrow tab.

- Product is read once from `index.html`'s `<title>`. Strings follow the app
  language (the same keys as the Projects pages).
- Site = `location.host`, left out in the desktop (Wails) window. Two
  instances never share a title. No new setting or persistence; a user-set
  site name would need a config field and is not done.
- Each part is cut at 40 characters (`…`) so the suffix survives.
- `●` is one boolean, the same "something waits" check as the rail dot, so the
  tab never flickers with counts. Favicon per state/instance: not done.

## Wiring

`useDocumentTitle(parts, priority)` and `setTitleAttention(on)` in
`@tingly/ui` (`utils/documentTitle.ts`). Projects pages call the hook; `Layout`
sets only the attention mark.

## Notification clicks

A browser notification for a thread opens that thread (`?thread=<id>`, or the
folder session) — also when it waits on an answer, since the thread view shows
its approvals and questions. It used to deep-link to `?review=` which only
highlighted the request card in the Overview and never selected the thread.
A project the list has not loaded yet is assumed to be a project (not a
folder) so the click still navigates.
