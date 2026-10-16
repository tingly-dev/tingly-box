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
    IconFileDiff,
    IconFileUpload,
    IconFolder,
    IconFolderOff,
    IconFolderPlus,
    IconFolderShare,
    IconGitBranch,
    IconGitMerge,
    IconHandStop,
    IconHistoryToggle,
    IconLayoutSidebarLeftCollapse,
    IconLayoutSidebarLeftExpand,
    IconLayoutSidebarRight,
    IconLoader2,
    IconMessage,
    IconMessageCircleQuestion,
    IconMessages,
    IconMinus,
    IconLayout2,
    IconPencil,
    IconPictureInPictureOn,
    IconPlayerStopFilled,
    IconStarFilled,
    IconTextPlus,
} from '@tabler/icons-react';
import { tablerMui, tablerMuiFilled } from '@tingly/ui/icons';

export * from '@tingly/ui/icons';

// A starred item shows the filled star; Tingly Box's Star is the outline.
export const Star = tablerMuiFilled(IconStarFilled);

export const Unarchive = tablerMui(IconArchiveOff);
export const Folder = tablerMui(IconFolder);
export const DriveFileMove = tablerMui(IconFolderShare);
export const FolderOff = tablerMui(IconFolderOff);
export const ChatBubble = tablerMui(IconMessage);
export const Forum = tablerMui(IconMessages);
export const ArrowUpward = tablerMui(IconArrowUp);
export const Stop = tablerMuiFilled(IconPlayerStopFilled);
export const UploadFile = tablerMui(IconFileUpload);
export const PostAdd = tablerMui(IconTextPlus);
export const SidebarCollapse = tablerMui(IconLayoutSidebarLeftCollapse);
export const SidebarExpand = tablerMui(IconLayoutSidebarLeftExpand);
export const Pencil = tablerMui(IconPencil);
export const MemoryOff = tablerMui(IconHistoryToggle);
export const Branch = tablerMui(IconGitBranch);
export const Merge = tablerMui(IconGitMerge);
export const OverviewPane = tablerMui(IconLayoutSidebarRight);
export const Question = tablerMui(IconMessageCircleQuestion);
export const DiffIcon = tablerMui(IconFileDiff);
export const Spinner = tablerMui(IconLoader2);
export const HandStop = tablerMui(IconHandStop);
export const NewFolder = tablerMui(IconFolderPlus);
export const CircleDashed = tablerMui(IconCircleDashed);
export const FloatWindow = tablerMui(IconPictureInPictureOn);
export const Minimize = tablerMui(IconMinus);
export const LayoutIcon = tablerMui(IconLayout2);
