/**
 * Tabler icons adapted to MUI conventions.
 *
 * Two ways to use:
 *
 * 1. Predefined, MUI-named icons (drop-in for `@mui/icons-material`):
 *      import { Close, Delete, ContentCopy } from '@/components/icons';
 *      <Delete fontSize="small" color="error" />
 *
 * 2. Generic factory for anything not predefined here:
 *      import { tablerMui } from '@/components/icons';
 *      import { IconRocket } from '@tabler/icons-react';
 *      const Rocket = tablerMui(IconRocket);
 *
 * Predefined icons keep the bare MUI name so migrating a file is usually just
 * swapping the import source. Both `fontSize` sizing and `color` semantics are
 * inherited from MUI's SvgIcon (see ./tablerMui).
 */
import {
    IconInfoCircle,
    IconRefresh,
    IconX,
    IconTrash,
    IconTrashX,
    IconSearch,
    IconChevronUp,
    IconChevronDown,
    IconChevronLeft,
    IconChevronRight,
    IconPlus,
    IconCirclePlus,
    IconEye,
    IconEyeOff,
    IconTerminal2,
    IconPlayerPlay,
    IconWorld,
    IconEdit,
    IconFileDescription,
    IconCopy,
    IconClipboard,
    IconKey,
    IconRestore,
    IconArrowUpRight,
    IconExternalLink,
    IconMapPin,
    IconLink,
    IconDeviceLaptop,
    IconDeviceDesktop,
    IconArrowBarToRight,
    IconArrowBarToLeft,
    IconPuzzle,
    IconAlertCircle,
    IconAlertTriangle,
    IconArrowsLeftRight,
    IconCircleCheck,
    IconCircleX,
    IconCalendar,
    IconClock,
    IconPlugConnected,
    IconBug,
    IconWand,
    IconSparkles,
    IconArrowRight,
    IconArrowLeft,
    IconArrowUp,
    IconSettings,
    IconUpload,
    IconDownload,
    IconFileDownload,
    IconCode,
    IconCheck,
    IconSunset,
    IconSend,
    IconFlask,
    IconQrcode,
    IconDotsVertical,
    IconDots,
    IconLogout,
    IconLogin,
    IconLock,
    IconListDetails,
    IconSun,
    IconMoon,
    IconBrandGithub,
    IconFolderOpen,
    IconPointFilled,
    IconBolt,
    IconBan,
    IconChartBar,
    IconApps,
    IconShare,
    IconChartDonut,
    IconRoute,
    IconSelector,
    IconGauge,
    IconActivity,
    IconTool,
    IconStopwatch,
    IconCoin,
    IconRouter,
    IconShieldLock,
    IconFileUpload,
    IconArticle,
    IconHelpCircle,
    IconHome,
    IconHistory,
    IconChecklist,
    IconMinus,
    IconList,
    IconNavigation,
    IconBrain,
} from '@tabler/icons-react';
import { tablerMui } from './tablerMui';

export { tablerMui };
export type { SvgIconComponent } from '@mui/icons-material';

// --- Navigation / chevrons ---------------------------------------------------
export const ChevronRight = tablerMui(IconChevronRight);
export const ChevronLeft = tablerMui(IconChevronLeft);
export const KeyboardArrowUp = tablerMui(IconChevronUp);
export const KeyboardArrowDown = tablerMui(IconChevronDown);
export const ExpandMore = tablerMui(IconChevronDown);
export const ExpandLess = tablerMui(IconChevronUp);
export const NavigateNext = tablerMui(IconChevronRight);
export const NavigateBefore = tablerMui(IconChevronLeft);
export const ArrowForwardIos = tablerMui(IconChevronRight);
export const ArrowBackIosNew = tablerMui(IconChevronLeft);
export const ArrowForward = tablerMui(IconArrowRight);
export const ArrowBack = tablerMui(IconArrowLeft);

// --- Actions -----------------------------------------------------------------
export const Add = tablerMui(IconPlus);
export const AddCircleOutline = tablerMui(IconCirclePlus);
export const Close = tablerMui(IconX);
export const Cancel = tablerMui(IconCircleX);
export const Delete = tablerMui(IconTrash);
export const DeleteSweep = tablerMui(IconTrashX);
export const Edit = tablerMui(IconEdit);
export const Search = tablerMui(IconSearch);
export const Refresh = tablerMui(IconRefresh);
export const RestartAlt = tablerMui(IconRestore);
export const ContentCopy = tablerMui(IconCopy);
export const ContentPaste = tablerMui(IconClipboard);
export const Upload = tablerMui(IconUpload);
export const Download = tablerMui(IconDownload);
export const FileDownload = tablerMui(IconFileDownload);
export const Send = tablerMui(IconSend);
export const MoreVert = tablerMui(IconDotsVertical);
export const MoreHoriz = tablerMui(IconDots);
export const OpenInNew = tablerMui(IconExternalLink);
export const Launch = tablerMui(IconExternalLink);
export const Link = tablerMui(IconLink);
export const PlayArrow = tablerMui(IconPlayerPlay);

// --- Status / feedback -------------------------------------------------------
export const Info = tablerMui(IconInfoCircle);
export const InfoOutlined = tablerMui(IconInfoCircle);
export const CheckCircle = tablerMui(IconCircleCheck);
export const Check = tablerMui(IconCheck);
export const Error = tablerMui(IconAlertCircle);
export const ErrorOutline = tablerMui(IconAlertCircle);
export const Warning = tablerMui(IconAlertTriangle);
export const WarningAmber = tablerMui(IconAlertTriangle);
export const Block = tablerMui(IconBan);
export const Bolt = tablerMui(IconBolt);
export const FiberManualRecord = tablerMui(IconPointFilled);

// --- Visibility / security ---------------------------------------------------
export const Visibility = tablerMui(IconEye);
export const VisibilityOutlined = tablerMui(IconEye);
export const VisibilityOff = tablerMui(IconEyeOff);
export const Lock = tablerMui(IconLock);
export const Key = tablerMui(IconKey);
export const VpnKey = tablerMui(IconKey);
export const Login = tablerMui(IconLogin);
export const Logout = tablerMui(IconLogout);

// --- Content / objects -------------------------------------------------------
export const Description = tablerMui(IconFileDescription);
export const FolderOpen = tablerMui(IconFolderOpen);
export const ListAlt = tablerMui(IconListDetails);
export const Code = tablerMui(IconCode);
export const Terminal = tablerMui(IconTerminal2);
export const Extension = tablerMui(IconPuzzle);
export const Settings = tablerMui(IconSettings);
export const SettingsApplications = tablerMui(IconSettings);
export const AppRegistration = tablerMui(IconApps);
export const BarChart = tablerMui(IconChartBar);
export const QrCode = tablerMui(IconQrcode);
export const CalendarToday = tablerMui(IconCalendar);
export const AccessTime = tablerMui(IconClock);
export const Science = tablerMui(IconFlask);
export const BugReport = tablerMui(IconBug);
export const AutoFixHigh = tablerMui(IconWand);
export const AutoAwesome = tablerMui(IconSparkles);
export const CompareArrows = tablerMui(IconArrowsLeftRight);
export const Outbound = tablerMui(IconArrowUpRight);
export const UpgradeOutlined = tablerMui(IconArrowUp);
export const LocationOn = tablerMui(IconMapPin);
export const Hub = tablerMui(IconShare);
export const Cable = tablerMui(IconPlugConnected);
export const Input = tablerMui(IconArrowBarToRight);
export const Output = tablerMui(IconArrowBarToLeft);
export const Psychology = tablerMui(IconBrain);

// --- Devices / brand / theme -------------------------------------------------
export const Laptop = tablerMui(IconDeviceLaptop);
export const Computer = tablerMui(IconDeviceDesktop);
export const Language = tablerMui(IconWorld);
export const Public = tablerMui(IconWorld);
export const GitHub = tablerMui(IconBrandGithub);
export const LightMode = tablerMui(IconSun);
export const DarkMode = tablerMui(IconMoon);
export const WbTwilight = tablerMui(IconSunset);
export const LaptopMac = tablerMui(IconDeviceLaptop);

// --- Additional aliases pulled in by the broad rollout ----------------------
export const DataUsage = tablerMui(IconChartDonut);
export const Route = tablerMui(IconRoute);
export const Router = tablerMui(IconRouter);
export const UnfoldMore = tablerMui(IconSelector);
export const Speed = tablerMui(IconGauge);
export const Stream = tablerMui(IconActivity);
export const Build = tablerMui(IconTool);
export const Schedule = tablerMui(IconClock);
export const Timer = tablerMui(IconStopwatch);
export const Token = tablerMui(IconCoin);
export const Sync = tablerMui(IconRefresh);
export const Security = tablerMui(IconShieldLock);
export const FileUpload = tablerMui(IconFileUpload);
export const Article = tablerMui(IconArticle);
export const ArticleOutlined = tablerMui(IconArticle);
export const Help = tablerMui(IconHelpCircle);
export const HelpOutline = tablerMui(IconHelpCircle);
export const Home = tablerMui(IconHome);
export const History = tablerMui(IconHistory);
export const DeleteOutline = tablerMui(IconTrash);
export const LockOutlined = tablerMui(IconLock);
export const CheckCircleRounded = tablerMui(IconCircleCheck);
export const Rule = tablerMui(IconChecklist);
export const Remove = tablerMui(IconMinus);
export const HorizontalRule = tablerMui(IconMinus);
export const ViewList = tablerMui(IconList);
export const NearMeOutlined = tablerMui(IconNavigation);
