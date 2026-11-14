/**
 * Tingly Box's icon set (@tingly/ui/icons), plus the few Projects needs that
 * Tingly Box does not have. Import icons from here; for anything else use
 * `tablerMui(IconFoo)` and add it here if it is worth reusing. Do not import
 * `@mui/icons-material` directly.
 */
import {
    IconArchiveOff,
    IconArrowUp,
    IconCircleDashed,
    IconCircleDot,
    IconFileDiff,
    IconFileUpload,
    IconFolder,
    IconFolderOff,
    IconFolderOpen,
    IconFolderPlus,
    IconFolderShare,
    IconGitBranch,
    IconGitCommit,
    IconGitMerge,
    IconGitPullRequest,
    IconHandStop,
    IconHistoryToggle,
    IconLayoutKanban,
    IconLayoutRows,
    IconLayoutSidebarLeftCollapse,
    IconLayoutSidebarLeftExpand,
    IconLayoutSidebarRight,
    IconLayoutSidebarRightCollapse,
    IconLayoutSidebarRightExpand,
    IconLoader2,
    IconMessage,
    IconMessageCircleQuestion,
    IconMessages,
    IconMinus,
    IconLayout2,
    IconPaperclip,
    IconPencil,
    IconPictureInPictureOn,
    IconRefresh,
    IconPlayerStopFilled,
    IconStack2,
    IconStarFilled,
    IconTextPlus,
} from '@tabler/icons-react';
import { tablerMui, tablerMuiFilled } from '@tingly/ui/icons';

export * from '@tingly/ui/icons';

// A starred item shows the filled star; Tingly Box's Star is the outline.
export const Star = tablerMuiFilled(IconStarFilled);

export const Stack = tablerMui(IconStack2);
export const Unarchive = tablerMui(IconArchiveOff);
export const Folder = tablerMui(IconFolder);
export const DriveFileMove = tablerMui(IconFolderShare);
export const FolderOff = tablerMui(IconFolderOff);
export const FolderOpen = tablerMui(IconFolderOpen);
export const ChatBubble = tablerMui(IconMessage);
export const Forum = tablerMui(IconMessages);
export const ArrowUpward = tablerMui(IconArrowUp);
export const Stop = tablerMuiFilled(IconPlayerStopFilled);
export const UploadFile = tablerMui(IconFileUpload);
export const PostAdd = tablerMui(IconTextPlus);
export const SidebarCollapse = tablerMui(IconLayoutSidebarLeftCollapse);
export const SidebarExpand = tablerMui(IconLayoutSidebarLeftExpand);
export const Pencil = tablerMui(IconPencil);
export const Attach = tablerMui(IconPaperclip);
export const MemoryOff = tablerMui(IconHistoryToggle);
export const Branch = tablerMui(IconGitBranch);
export const Merge = tablerMui(IconGitMerge);
export const GitCommit = tablerMui(IconGitCommit);
export const PullRequest = tablerMui(IconGitPullRequest);
export const IssueOpened = tablerMui(IconCircleDot);
export const OverviewPane = tablerMui(IconLayoutSidebarRight);
export const RailHide = tablerMui(IconLayoutSidebarRightCollapse);
export const RailShow = tablerMui(IconLayoutSidebarRightExpand);
export const Question = tablerMui(IconMessageCircleQuestion);
export const DiffIcon = tablerMui(IconFileDiff);
export const Spinner = tablerMui(IconLoader2);
export const Refresh = tablerMui(IconRefresh);
export const HandStop = tablerMui(IconHandStop);
export const NewFolder = tablerMui(IconFolderPlus);
export const CircleDashed = tablerMui(IconCircleDashed);
export const FloatWindow = tablerMui(IconPictureInPictureOn);
export const Minimize = tablerMui(IconMinus);
export const LayoutIcon = tablerMui(IconLayout2);
export const Kanban = tablerMui(IconLayoutKanban);
export const KanbanRows = tablerMui(IconLayoutRows);
