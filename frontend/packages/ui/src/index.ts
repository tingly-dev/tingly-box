// The page primitives. The theme lives at '@tingly/ui/theme' (plus
// '@tingly/ui/theme/<file>' for fonts, status, elevation, controls, options)
// and the icons at '@tingly/ui/icons'.
export { default as PageLayout } from './components/PageLayout';
export { default as PageHeader } from './components/PageHeader';
export { default as Surface } from './components/Surface';
export { default as EmptyState } from './components/EmptyState';
export { default as DialogHeader } from './components/DialogHeader';
export { default as ConfirmDialog } from './components/ConfirmDialog';
export { default as ConfirmPopover } from './components/ConfirmPopover';
export type { ConfirmPopoverProps } from './components/ConfirmPopover';
export { CoachMark } from './components/CoachMark';
export { CopyIconButton } from './components/CopyIconButton';
export { useCopyFeedback } from './hooks/useCopyFeedback';
export { writeToClipboard } from './utils/clipboard';
export { EntityLabel } from './components/EntityLabel';
export type { EntityLabelProps } from './components/EntityLabel';
export { entityText, shortId, bareId, aliasOrName } from './utils/entityLabel';
export type { EntityKind, EntityParts, ShortId, EntityTextOptions } from './utils/entityLabel';
