import {
    Box,
    Button,
    ButtonBase,
    Card,
    CardContent,
    CircularProgress,
    IconButton,
    Stack,
    Tooltip,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { CopyIconButton } from '@/components/CopyIconButton';
import { Close, Edit, ErrorOutline, Refresh, RestartAlt, ZoomIn } from '@/components/icons';
import { overlayActionSx, zoomScrimSx } from './ImageGenPlayground.chrome';
import { resultSrc, runImage } from './imageGenSession';
import RunSourceStrip from './RunSourceStrip';
import type { GenerationRun, SelectedImage } from './ImageGenPlayground.types';

interface GenerationRunCardProps {
    run: GenerationRun;
    onCancel: (id: string) => void;
    onRetry: (run: GenerationRun) => void;
    // Puts the run's whole request back into the panel.
    onReuse: (run: GenerationRun) => void;
    onOpenSource: (run: GenerationRun, index: number) => void;
    onUseAsReference: (src: string) => void;
    onOpenOutput: (image: SelectedImage) => void;
    onRemove: (id: string) => void;
}

// The per-run actions that read the same in every card state: take the
// prompt away as text (a prompt should never have to be selected by hand),
// put the whole request back in the panel, and drop the card. While a run
// is in flight Cancel is what removes it, so `onRemove` is left out there.
const GenerationRunCard: React.FC<GenerationRunCardProps> = ({
    run,
    onCancel,
    onRetry,
    onReuse,
    onOpenSource,
    onUseAsReference,
    onOpenOutput,
    onRemove,
}) => {
    const { t } = useTranslation();
    const reuseLabel = t('playground.reuse.action', { defaultValue: 'Edit this request' });
    const copyPromptLabel = t('playground.copyPrompt', { defaultValue: 'Copy prompt' });
    const promptCopiedLabel = t('playground.promptCopied', { defaultValue: 'Copied' });
    const removeRunLabel = t('playground.removeRun', { defaultValue: 'Remove this generation' });

    const renderRunActions = (runActions: GenerationRun, onRemoveClick?: () => void) => (
        <Stack direction="row" spacing={0} sx={{ flexShrink: 0, alignItems: 'center' }}>
            <CopyIconButton
                value={runActions.prompt}
                label={copyPromptLabel}
                copiedLabel={promptCopiedLabel}
                iconSize={16}
                color="text.disabled"
                sx={{ p: 0.5, '&:hover': { color: 'text.primary' } }}
            />
            <Tooltip title={reuseLabel}>
                <IconButton
                    size="small"
                    onClick={() => { onReuse(runActions); }}
                    aria-label={reuseLabel}
                    data-testid="imagegen-reuse-run"
                    sx={{ p: 0.5, color: 'text.disabled', '&:hover': { color: 'text.primary' } }}
                >
                    <RestartAlt sx={{ fontSize: 16 }} />
                </IconButton>
            </Tooltip>
            {onRemoveClick && (
                // A destructive action, so it gets its own hover colour
                // (error, not the neutral text.primary the other two use) —
                // that is also what makes it findable, not just clickable.
                <Tooltip title={removeRunLabel}>
                    <IconButton
                        size="small"
                        onClick={onRemoveClick}
                        aria-label={removeRunLabel}
                        data-testid="imagegen-remove-run"
                        sx={{ p: 0.5, color: 'text.disabled', '&:hover': { color: 'error.main' } }}
                    >
                        <Close sx={{ fontSize: 16 }} />
                    </IconButton>
                </Tooltip>
            )}
        </Stack>
    );

    return (
        <Card
            data-testid="imagegen-generation-run"
            data-generation-status={run.status ?? 'completed'}
            variant="outlined"
            sx={{
                flex: { xs: '0 0 min(82vw, 320px)', md: '0 0 clamp(280px, 46%, 360px)' },
                height: '100%',
                bgcolor: 'background.paper',
                borderStyle: run.status === 'pending' ? 'dashed' : 'solid',
                borderColor: run.status === 'failed' ? 'error.main' : undefined,
                scrollSnapAlign: 'start',
            }}
        >
            <CardContent sx={{ p: 1.5, height: '100%', '&:last-child': { pb: 1.5 } }}>
                {run.status === 'pending' ? (
                    <Stack
                        spacing={1.25}
                        aria-live="polite"
                        sx={{
                            alignItems: 'center',
                            justifyContent: 'center',
                            height: '100%',
                            minWidth: 0,
                            textAlign: 'center',
                        }}
                    >
                        {/* A running request is still a request to copy or to
                            fork into the next one — the actions do not wait
                            for it to land. */}
                        <Box sx={{ alignSelf: 'flex-end', mt: -0.5, mr: -0.5 }}>
                            {renderRunActions(run)}
                        </Box>
                        <CircularProgress size={24} />
                        <Typography variant="body2" sx={{ fontWeight: 500 }}>
                            {t('playground.generatingNew', { defaultValue: 'Generating new images…' })}
                        </Typography>
                        <Typography
                            variant="caption"
                            sx={{
                                width: '100%',
                                color: 'text.secondary',
                                display: '-webkit-box',
                                WebkitLineClamp: 2,
                                WebkitBoxOrient: 'vertical',
                                overflow: 'hidden',
                            }}
                        >
                            {run.prompt}
                        </Typography>
                        <Typography
                            variant="caption"
                            sx={{
                                width: '100%',
                                color: 'text.disabled',
                                overflow: 'hidden',
                                textOverflow: 'ellipsis',
                                whiteSpace: 'nowrap',
                            }}
                        >
                            {run.model} · {run.size} · {run.quality} · images/{run.endpoint}
                        </Typography>
                        <RunSourceStrip
                            sources={run.sourceImages ?? []}
                            align="center"
                            onOpen={(index) => onOpenSource(run, index)}
                            onUseAsReference={onUseAsReference}
                        />
                        <Button
                            size="small"
                            variant="outlined"
                            color="inherit"
                            startIcon={<Close fontSize="small" />}
                            onClick={() => onCancel(run.id)}
                            data-testid="imagegen-cancel-run"
                        >
                            {t('playground.cancelRun', { defaultValue: 'Cancel' })}
                        </Button>
                    </Stack>
                ) : run.status === 'failed' ? (
                    <Stack spacing={1} sx={{ height: '100%', minWidth: 0 }}>
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
                            <ErrorOutline color="error" fontSize="small" />
                            <Typography variant="body2" sx={{ fontWeight: 500, flex: 1, minWidth: 0 }}>
                                {t('playground.runFailed', { defaultValue: 'Generation failed' })}
                            </Typography>
                            <Box sx={{ mr: -0.5, mt: -0.5 }}>
                                {renderRunActions(run, () => onRemove(run.id))}
                            </Box>
                        </Box>
                        <Typography
                            variant="caption"
                            sx={{
                                color: 'error.main',
                                display: '-webkit-box',
                                WebkitLineClamp: 3,
                                WebkitBoxOrient: 'vertical',
                                overflow: 'hidden',
                                wordBreak: 'break-word',
                            }}
                        >
                            {run.error}
                        </Typography>
                        <Typography
                            variant="caption"
                            sx={{
                                color: 'text.secondary',
                                display: '-webkit-box',
                                WebkitLineClamp: 3,
                                WebkitBoxOrient: 'vertical',
                                overflow: 'hidden',
                            }}
                        >
                            {run.prompt}
                        </Typography>
                        <Typography
                            variant="caption"
                            sx={{ color: 'text.disabled', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
                        >
                            {run.model} · {run.size} · {run.quality} · images/{run.endpoint}
                        </Typography>
                        {/* Retrying blind is not retrying: the images the failed
                            request was built from stay on the card, openable in
                            the same lightbox as any other image here. */}
                        <RunSourceStrip
                            sources={run.sourceImages ?? []}
                            onOpen={(index) => onOpenSource(run, index)}
                            onUseAsReference={onUseAsReference}
                        />
                        <Box sx={{ flex: 1 }} />
                        <Button
                            size="small"
                            variant="outlined"
                            startIcon={<Refresh fontSize="small" />}
                            onClick={() => onRetry(run)}
                            sx={{ alignSelf: 'flex-start' }}
                        >
                            {t('playground.retry', { defaultValue: 'Retry' })}
                        </Button>
                    </Stack>
                ) : (
                    <Stack spacing={1.25} sx={{ height: '100%' }}>
                        <Box sx={{ minWidth: 0 }}>
                            <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 0.75 }}>
                                <Typography
                                    variant="body2"
                                    sx={{
                                        fontWeight: 500,
                                        flex: 1,
                                        minWidth: 0,
                                        display: '-webkit-box',
                                        WebkitLineClamp: 2,
                                        WebkitBoxOrient: 'vertical',
                                        overflow: 'hidden',
                                    }}
                                >
                                    {run.prompt}
                                </Typography>
                                <Box sx={{ mr: -0.75, mt: -0.75 }}>
                                    {renderRunActions(run, () => onRemove(run.id))}
                                </Box>
                            </Box>
                            <Typography
                                variant="caption"
                                sx={{
                                    display: 'block',
                                    color: "text.secondary",
                                    overflow: 'hidden',
                                    textOverflow: 'ellipsis',
                                    whiteSpace: 'nowrap',
                                }}
                            >
                                {run.model} · {run.size} · {run.quality} · images/{run.endpoint}
                            </Typography>
                            <RunSourceStrip
                                sources={run.sourceImages ?? []}
                                onOpen={(index) => onOpenSource(run, index)}
                                onUseAsReference={onUseAsReference}
                            />
                        </Box>
                        <Box
                            sx={{
                                display: 'grid',
                                gridTemplateColumns: run.images.length === 1
                                    ? 'minmax(0, 1fr)'
                                    : 'repeat(2, minmax(0, 1fr))',
                                flex: 1,
                                minHeight: 0,
                                gap: 1,
                            }}
                        >
                            {run.images.map((image, index) => {
                                const src = resultSrc(image);
                                return src ? (
                                    <Box
                                        key={`${run.id}-${index}`}
                                        sx={{ position: 'relative', width: '100%', height: '100%', borderRadius: 1, overflow: 'hidden', bgcolor: 'action.hover' }}
                                    >
                                        <ButtonBase
                                            onClick={() => onOpenOutput(runImage(run, 'output', index, src))}
                                            aria-label={t('playground.openResult', {
                                                defaultValue: 'Open generated image {{number}}',
                                                number: index + 1,
                                            })}
                                            sx={{
                                                width: '100%',
                                                height: '100%',
                                                display: 'block',
                                                '&:hover .image-preview-overlay, &:focus-visible .image-preview-overlay': {
                                                    opacity: 1,
                                                },
                                            }}
                                        >
                                            <Box
                                                component="img"
                                                src={src}
                                                alt={t('playground.resultAlt', {
                                                    defaultValue: 'Generated image {{number}}',
                                                    number: index + 1,
                                                })}
                                                sx={{
                                                    width: '100%',
                                                    height: '100%',
                                                    maxHeight: '100%',
                                                    objectFit: 'contain',
                                                    display: 'block',
                                                }}
                                            />
                                            <Box className="image-preview-overlay" sx={zoomScrimSx}>
                                                <ZoomIn sx={{ fontSize: 30 }} />
                                            </Box>
                                            <Box
                                                sx={{
                                                    ...overlayActionSx(),
                                                    position: 'absolute',
                                                    top: 8,
                                                    right: 8,
                                                    display: 'flex',
                                                    alignItems: 'center',
                                                    justifyContent: 'center',
                                                    borderRadius: '50%',
                                                }}
                                            >
                                                <ZoomIn fontSize="small" />
                                            </Box>
                                        </ButtonBase>
                                        <IconButton
                                            size="small"
                                            onClick={(event) => { event.stopPropagation(); onUseAsReference(src); }}
                                            aria-label={t('playground.useAsReference', { defaultValue: 'Use as reference' })}
                                            sx={{ position: 'absolute', bottom: 8, right: 8, ...overlayActionSx() }}
                                        >
                                            <Edit fontSize="small" />
                                        </IconButton>
                                    </Box>
                                ) : (
                                    <Typography key={`${run.id}-${index}`} variant="caption" sx={{
                                        color: "text.secondary"
                                    }}>
                                        {t('playground.emptyResult', { defaultValue: 'No image returned' })}
                                    </Typography>
                                );
                            })}
                        </Box>
                    </Stack>
                )}
            </CardContent>
        </Card>
    );
};

export default GenerationRunCard;
