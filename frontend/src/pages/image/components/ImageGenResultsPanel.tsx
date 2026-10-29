import { Box, Button, ButtonBase, Stack, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useMemo, useRef } from 'react';
import { ContentPaste, FileUpload, Photo, ViewGallery } from '@/components/icons';
import { CopyIconButton } from '@/components/CopyIconButton';
import { fontMono } from '@/theme/fonts';
import GenerationRunCard from './GenerationRunCard';
import ImportedImageCard from './ImportedImageCard';
import PanelAction from './PanelAction';
import { STRIP_CARD_HEIGHT, WRAP_GRID_COLUMNS, WRAP_GRID_GAP } from './imageGenSession';
import type { GenerationRun, ImportedImage, SelectedImage } from './ImageGenPlayground.types';

// One timeline for the results panel: images brought in to work on and
// images the model produced, in the order they arrived. Two lists would
// make "where did my image go" a question the user has to ask.
export type ImageGenTimelineEntry =
    | { kind: 'import'; at: number; item: ImportedImage }
    | { kind: 'run'; at: number; run: GenerationRun };

interface ImageGenResultsPanelProps {
    timeline: ImageGenTimelineEntry[];
    visibleTimeline: ImageGenTimelineEntry[];
    runsCount: number;
    importedCount: number;
    olderTimelineCount: number;
    desktopPanelHeight: string | number;
    // Where generated images are written on disk; shown under the results,
    // next to the images it is about.
    outputDir?: string;
    // Attached to the scrolling strip so the session hook can keep the newest
    // card in view as new ones arrive.
    historyTrackRef: React.RefObject<HTMLDivElement | null>;
    onDropFiles: (files: FileList | File[], onImages: (images: File[]) => void) => void;
    onImportFiles: (files: FileList | File[]) => void;
    onImportFromClipboard: () => void;
    onOpenGallery: () => void;
    onOpenImport: (item: ImportedImage) => void;
    onUseAsReference: (src: string) => void;
    onRemoveImport: (id: string) => void;
    onCancelRun: (id: string) => void;
    onRetryRun: (run: GenerationRun) => void;
    onReuseRun: (run: GenerationRun) => void;
    onSaveRunAsProfile?: (run: GenerationRun) => void;
    onOpenRunSource: (run: GenerationRun, index: number) => void;
    onOpenOutput: (image: SelectedImage) => void;
    onRemoveRun: (id: string) => void;
}

// The results half of the playground: an empty-state invitation when the
// session is empty, otherwise the header (counts + ways in) over a strip of
// imported images and generation runs, oldest to newest — scrolling sideways
// when the panel is short, wrapping into rows on the full-height workbench — with an "overview" tile collapsing everything older
// than the visible tail.
const ImageGenResultsPanel: React.FC<ImageGenResultsPanelProps> = ({
    timeline,
    visibleTimeline,
    runsCount,
    importedCount,
    olderTimelineCount,
    desktopPanelHeight,
    outputDir,
    historyTrackRef,
    onDropFiles,
    onImportFiles,
    onImportFromClipboard,
    onOpenGallery,
    onOpenImport,
    onUseAsReference,
    onRemoveImport,
    onCancelRun,
    onRetryRun,
    onReuseRun,
    onSaveRunAsProfile,
    onOpenRunSource,
    onOpenOutput,
    onRemoveRun,
}) => {
    const { t } = useTranslation();
    // The run cards are memoised, so what they are handed must not change
    // identity when the panel re-renders; these forward to the newest props.
    const latest = useRef({ onCancelRun, onRetryRun, onReuseRun, onSaveRunAsProfile, onOpenRunSource, onUseAsReference, onOpenOutput, onRemoveRun });
    latest.current = { onCancelRun, onRetryRun, onReuseRun, onSaveRunAsProfile, onOpenRunSource, onUseAsReference, onOpenOutput, onRemoveRun };
    const cardHandlers = useMemo(() => ({
        onCancel: (id: string) => latest.current.onCancelRun(id),
        onRetry: (run: GenerationRun) => latest.current.onRetryRun(run),
        onReuse: (run: GenerationRun) => latest.current.onReuseRun(run),
        onOpenSource: (run: GenerationRun, index: number) => latest.current.onOpenRunSource(run, index),
        onUseAsReference: (src: string) => latest.current.onUseAsReference(src),
        onOpenOutput: (image: SelectedImage) => latest.current.onOpenOutput(image),
        onRemove: (id: string) => latest.current.onRemoveRun(id),
        onSaveAsProfile: (run: GenerationRun) => latest.current.onSaveRunAsProfile?.(run),
    }), []);
    const importFileInputRef = useRef<HTMLInputElement>(null);
    // Its own words, not the reference row's "Browse" / "Paste": what lands
    // here is worked on as it is (crop, slice, mask), not generated from.
    const importLabel = t('playground.importFile', { defaultValue: 'Import' });
    const importClipboardLabel = t('playground.importClipboard', { defaultValue: 'From clipboard' });
    const outputDirLine = outputDir ? (
        <Stack
            direction="row"
            spacing={0.5}
            data-testid="imagegen-output-dir"
            sx={{ alignItems: 'center', minWidth: 0, color: 'text.secondary' }}
        >
            <Typography variant="caption" sx={{ flexShrink: 0 }}>
                {t('playground.outputDirLabel', { defaultValue: 'Generated images are saved to' })}:
            </Typography>
            <Typography
                variant="caption"
                sx={{ fontFamily: fontMono, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
                title={outputDir}
            >
                {outputDir}
            </Typography>
            <CopyIconButton
                value={outputDir}
                label={t('common.copy', { defaultValue: 'Copy' })}
                copiedLabel={t('common.copied', { defaultValue: 'Copied!' })}
                iconSize={14}
                sx={{ p: 0.25 }}
            />
        </Stack>
    ) : null;
    return (
        <Box
            data-testid="imagegen-preview-panel"
            onDragOver={(event) => event.preventDefault()}
            onDrop={(event) => {
                event.preventDefault();
                if (event.dataTransfer.files?.length) {
                    onDropFiles(event.dataTransfer.files, onImportFiles);
                }
            }}
            sx={{
                minWidth: 0,
                minHeight: 0,
                height: { xs: 320, lg: desktopPanelHeight },
                border: '1px solid',
                borderColor: 'divider',
                borderRadius: 2,
                bgcolor: 'action.hover',
                p: 2,
                display: 'flex',
                alignItems: timeline.length === 0 ? 'center' : 'stretch',
                justifyContent: timeline.length === 0 ? 'center' : 'flex-start',
                overflow: 'hidden',
            }}
        >
            <input
                ref={importFileInputRef}
                type="file"
                accept="image/png,image/jpeg,image/webp"
                multiple
                hidden
                onChange={(event) => {
                    if (event.target.files?.length) onImportFiles(event.target.files);
                    event.target.value = '';
                }}
            />
            {timeline.length === 0 ? (
                <Stack
                    spacing={1}
                    sx={{
                        alignItems: "center",
                        color: 'text.secondary',
                        textAlign: 'center'
                    }}>
                    <Photo sx={{ fontSize: 44, opacity: 0.45 }} />
                    <Typography variant="subtitle2" sx={{
                        color: "text.secondary"
                    }}>
                        {t('playground.previewEmpty', { defaultValue: 'Generated images appear here' })}
                    </Typography>
                    <Typography variant="caption" sx={{
                        color: "text.disabled"
                    }}>
                        {t('playground.previewHint', { defaultValue: 'To crop, slice or mask an image without generating, import it here.' })}
                    </Typography>
                    <Stack direction="row" spacing={0.5} sx={{ pt: 0.5 }}>
                        <Button
                            size="small"
                            color="inherit"
                            startIcon={<FileUpload fontSize="small" />}
                            onClick={() => importFileInputRef.current?.click()}
                            sx={{ color: 'text.secondary', '&:hover': { color: 'primary.main' } }}
                        >
                            {importLabel}
                        </Button>
                        <Button
                            size="small"
                            color="inherit"
                            startIcon={<ContentPaste fontSize="small" />}
                            onClick={onImportFromClipboard}
                            sx={{ color: 'text.secondary', '&:hover': { color: 'primary.main' } }}
                        >
                            {importClipboardLabel}
                        </Button>
                    </Stack>
                    {outputDirLine && <Box sx={{ pt: 2, maxWidth: '100%' }}>{outputDirLine}</Box>}
                </Stack>
            ) : (
                <Stack spacing={1.5} sx={{ width: '100%', minWidth: 0, height: '100%' }}>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
                        <Typography variant="subtitle2" sx={{ minWidth: 0 }}>
                            {t('playground.sessionOutputs', { defaultValue: 'Session images' })}
                        </Typography>
                        <Typography
                            variant="caption"
                            sx={{
                                color: "text.secondary",
                                flexShrink: 0,
                                whiteSpace: 'nowrap',
                            }}
                        >
                            {[
                                runsCount > 0 && (runsCount === 1
                                    ? t('playground.runCountOne', { defaultValue: '1 generation' })
                                    : t('playground.runCount', {
                                        defaultValue: '{{count}} generations',
                                        count: runsCount,
                                    })),
                                importedCount > 0 && t('playground.importCount', {
                                    defaultValue: '{{count}} imported',
                                    count: importedCount,
                                }),
                            ].filter(Boolean).join(' · ')}
                        </Typography>
                        {/* The same two ways in as the empty state offers: the
                            strip filling up must not take them away. Plus the way
                            out of the strip itself, which is what a session with
                            thirty images actually needs. */}
                        <Stack direction="row" spacing={0.5} sx={{ flexShrink: 0, ml: 'auto' }}>
                            <PanelAction
                                label={t('playground.gallery.action', { defaultValue: 'Overview' })}
                                icon={<ViewGallery fontSize="small" />}
                                onClick={onOpenGallery}
                                testId="imagegen-open-gallery"
                            />
                            <PanelAction
                                label={importLabel}
                                icon={<FileUpload fontSize="small" />}
                                onClick={() => importFileInputRef.current?.click()}
                            />
                            <PanelAction
                                label={importClipboardLabel}
                                icon={<ContentPaste fontSize="small" />}
                                onClick={onImportFromClipboard}
                            />
                        </Stack>
                    </Box>

                    <Box
                        ref={historyTrackRef}
                        data-testid="imagegen-history-track"
                        sx={{
                            // A short strip (stacked layout) scrolls sideways.
                            // The full-height workbench is a grid that scrolls
                            // down instead: columns from the panel's width, so
                            // a row is always full rather than one card and a
                            // gap (see WRAP_GRID_COLUMNS).
                            display: { xs: 'flex', lg: 'grid' },
                            gridTemplateColumns: { lg: WRAP_GRID_COLUMNS },
                            // Rows as tall as their cards; `auto` would squeeze
                            // them to fit the panel (cards clip, so their
                            // min-height is 0) instead of scrolling.
                            gridAutoRows: { lg: 'max-content' },
                            gap: { xs: 1.5, lg: `${WRAP_GRID_GAP}px` },
                            flex: 1,
                            minHeight: 0,
                            alignContent: 'flex-start',
                            overflowX: { xs: 'auto', lg: 'hidden' },
                            overflowY: { xs: 'hidden', lg: 'auto' },
                            // A size container, so cards can size themselves
                            // from the strip's height (`cqh`, see stripCardBasis).
                            containerType: 'size',
                            pb: 0.5,
                            scrollSnapType: { xs: 'x proximity', lg: 'y proximity' },
                            overflowAnchor: 'none',
                            scrollbarWidth: 'thin',
                            '&::-webkit-scrollbar': { height: 6 },
                            '&::-webkit-scrollbar-thumb': { bgcolor: 'action.selected', borderRadius: 3 },
                        }}
                    >
                        {olderTimelineCount > 0 && (
                            <ButtonBase
                                onClick={onOpenGallery}
                                data-testid="imagegen-history-view-all"
                                aria-label={t('playground.gallery.viewOlderHint', {
                                    defaultValue: 'View all {{count}} images in the overview',
                                    count: timeline.length,
                                })}
                                // In the sideways strip it is a narrow tile at
                                // the start of the row. On the wrapping workbench
                                // a full-height column would eat a card slot and
                                // push the first card off its row, so there it is
                                // a slim bar across the top instead.
                                sx={{
                                    flex: { xs: '0 0 84px' },
                                    gridColumn: { lg: '1 / -1' },
                                    height: { xs: STRIP_CARD_HEIGHT, lg: 36 },
                                    display: 'flex',
                                    flexDirection: { xs: 'column', lg: 'row' },
                                    alignItems: 'center',
                                    justifyContent: 'center',
                                    gap: { xs: 0.5, lg: 1 },
                                    borderRadius: 1.5,
                                    border: '1px dashed',
                                    borderColor: 'divider',
                                    bgcolor: 'background.paper',
                                    color: 'text.secondary',
                                    scrollSnapAlign: 'start',
                                    '&:hover': { color: 'primary.main', borderColor: 'primary.main' },
                                }}
                            >
                                <ViewGallery fontSize="small" />
                                <Typography variant="caption" sx={{ fontWeight: 600, lineHeight: 1.2 }}>
                                    +{olderTimelineCount}
                                </Typography>
                                <Typography variant="caption" sx={{ fontSize: { xs: 10, lg: 12 }, color: 'text.disabled' }}>
                                    {t('playground.gallery.action', { defaultValue: 'Overview' })}
                                </Typography>
                            </ButtonBase>
                        )}
                        {visibleTimeline.map((entry) => (entry.kind === 'import' ? (
                            <ImportedImageCard
                                key={entry.item.id}
                                item={entry.item}
                                onOpen={() => onOpenImport(entry.item)}
                                onUseAsReference={() => onUseAsReference(entry.item.src)}
                                onRemove={() => onRemoveImport(entry.item.id)}
                            />
                        ) : (
                            <GenerationRunCard
                                key={entry.run.id}
                                run={entry.run}
                                {...cardHandlers}
                                onSaveAsProfile={onSaveRunAsProfile ? cardHandlers.onSaveAsProfile : undefined}
                            />
                        )))}
                    </Box>
                    {outputDirLine}
                </Stack>
            )}
        </Box>
    );
};

export default ImageGenResultsPanel;
