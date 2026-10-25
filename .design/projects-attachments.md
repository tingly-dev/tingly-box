# Projects attachments: files, images and long text as input

> TEP: [#2107](https://github.com/tingly-dev/tingly-box/issues/2107). Status:
> in progress. Implemented: the upload store, the project conversation,
> project threads and working-folder sessions. Not yet: standalone Chats, cleanup.

## Problem

Every composer took text only. A screenshot, a log or a long paste had to go
through another tool, or hit the message limit.

## What the user sees

One composer, the same everywhere it is wired:

- A paperclip button, dropping files on the box, and pasting files or images
  from the clipboard all attach. Each shows as a chip (a thumbnail for an
  image) above the text, with a spinner while it uploads and an x to remove it.
- A paste longer than 40 lines or 4,000 characters is not put in the box. It
  becomes a text file, shown as "Pasted text · N lines". The thresholds are
  constants (`components/attachments.tsx`), not a setting.
- Send waits for uploads; a file alone is a valid message.
- A sent message shows its files above the text, in the conversation and in a
  thread's transcript. An image opens as a thumbnail; a file that was removed
  from disk since shows as "removed".

## One place for uploads

Everything attached is written to disk as it is attached, under the project's
library: `library/uploads/<id>-<name>`, one flat file per upload (a short id keeps names unique; the name stays readable for the model). Nothing lives
only in memory or in the conversation.

- Composer uploads and Library uploads are one store: the Library tab lists
  them, and deleting a file there deletes it for the conversation too.
- A message records only a reference (name, library-relative path, type, size)
  in its payload; no schema change. The server re-reads size and type from
  disk when a send names a path, and refuses any path outside `uploads/`.
- Because each upload is one file, cleanup can later work per file, per
  age or per owner without touching anything else. Deleting a project already
  removes its library. Age-based cleanup of files never sent is the follow-up.

A working folder is a `type=folder` project, and its uploads live in the
project's own library like any other project's:
`<data>/projects/<project>/library/uploads/<id>-<name>`, shared by every thread
of the folder (see [workspace.md](../projects/.design/workspace.md)). It is
outside the folder the person works in on purpose: files put inside would show
up in their repository's `git status` and could be committed. Uploads made
before the first thread exists go straight there; there is no draft area.
A thread's Claude Code settings (the host profile's, if any, merged with the
library and memory directories) list them as allowed directories, so no
approval prompt. Threads started before uploads were shared per folder keep
their files in `<data>/projects/_sessions/<thread>/library/`; those stay
readable (the file route looks in the library first, then there). Routes:
`POST /projects/:id/attachments`, `POST /threads/:id/attachments` and
`GET /threads/:id/attachments/file` after.

## What the model gets

The message text is followed by a note listing each file's absolute path, type
and size. A thread reads them like any file (its directories already include
the library, and Claude Code reads images by path). The coordinator gets the
same note, so it can hand a path to a thread in a brief. This keeps images
working for any model that reaches the files through Claude Code, and keeps
the coordinator's token cost to a few lines per file instead of the content.
Inline image blocks for the coordinator's own vision are open.

## API

- `POST /projects/:id/attachments`: multipart `files`, returns the stored
  attachments (same per-file cap as the Library).
- Message and thread-message requests take `attachments`: the returned paths.
  `content` may be empty when attachments are present.

## Boundaries

`Composer` takes an `attach` prop; without it nothing changes, so hosts opt in.
The transcript renders files through a host-provided component, like its code
blocks, so `@tingly/projects/transcript` stays free of API calls.

## Open

- Drafts left in `_sessions/_new` by messages never sent are cleanup's job.
- Chats need multi-part message content in `llm`.
- Cleanup: unsent drafts' uploads, by age, and a per-project usage line.
