import { useCallback, useEffect, useState } from 'react';
import {
    Box,
    Button,
    ButtonBase,
    CircularProgress,
    Collapse,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    IconButton,
    Slider,
    Stack,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Close, ExpandMore, MoodSmile } from '@/components/icons';
import {
    EMOTIONS,
    EXPRESSION_PRESET_KEYS,
    EXPRESSION_PRESETS,
    NEUTRAL,
    presetOf,
    withWeight,
    type ExpressionState,
    type ExpressionWeight,
} from './expressionState';
import { drawFace, exportFace, loadFaceStage } from './faceRenderer';

type Stage = Awaited<ReturnType<typeof loadFaceStage>>;

export interface ExpressionResult {
    file: File;
    previewUrl: string;
    expression: ExpressionState;
}

interface ExpressionDialogProps {
    open: boolean;
    // Re-entry: the expression being edited, or null for a new one.
    initial: ExpressionState | null;
    onClose: () => void;
    onSubmit: (result: ExpressionResult) => void;
    showNotification: (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;
}

const PREVIEW_PX = 320;
const THUMB_PX = 60;

// Draws once the canvas exists and again whenever the face changes. A
// callback ref rather than an effect, so a thumbnail paints the moment it
// mounts.
const FaceCanvas: React.FC<{ stage: Stage; state: ExpressionState; size: number }> = ({ stage, state, size }) => {
    const paint = useCallback((canvas: HTMLCanvasElement | null) => {
        if (canvas) drawFace(stage, state, canvas, size * 2);
    }, [stage, state, size]);
    return <Box component="canvas" ref={paint} aria-hidden sx={{ width: size, height: size, display: 'block', borderRadius: 1 }} />;
};

// The sliders, in the order a face is read: the feeling, then the eyes, then
// the mouth, then where it looks.
const WEIGHT_SLIDERS: readonly ExpressionWeight[] = [...EMOTIONS, 'blink', 'aa'];

// An anime face, trimmed to the face alone — no hair, no body, no clothes —
// so what the model is handed is an expression and nothing it could mistake
// for a character to copy. See .design/expression-reference.md.
const ExpressionDialog: React.FC<ExpressionDialogProps> = ({ open, initial, onClose, onSubmit, showNotification }) => {
    const { t } = useTranslation();
    const [stage, setStage] = useState<Stage | null>(null);
    const [failed, setFailed] = useState(false);
    const [state, setState] = useState<ExpressionState>(NEUTRAL);
    const [tuning, setTuning] = useState(false);
    const [saving, setSaving] = useState(false);

    useEffect(() => {
        if (!open) return;
        setState(initial ?? EXPRESSION_PRESETS.smile);
        // A re-opened expression that no preset matches was hand-tuned: show
        // the sliders it was tuned with.
        setTuning(initial !== null && presetOf(initial) === null);
        setFailed(false);
        let cancelled = false;
        loadFaceStage()
            .then((loaded) => { if (!cancelled) setStage(loaded); })
            .catch(() => { if (!cancelled) setFailed(true); });
        return () => { cancelled = true; };
    }, [open, initial]);

    const current = presetOf(state);

    const handleSubmit = async () => {
        if (!stage) return;
        setSaving(true);
        try {
            const { file, previewUrl } = await exportFace(stage, state);
            onSubmit({ file, previewUrl, expression: state });
        } catch {
            showNotification(t('playground.expression.failed'), 'error');
        } finally {
            setSaving(false);
        }
    };

    const weightLabel = (key: ExpressionWeight) => t(`playground.expression.control.${key}`);

    return (
        <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
            <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1, pr: 1 }}>
                <MoodSmile fontSize="small" />
                <Box sx={{ flex: 1 }}>
                    <Typography variant="h6" component="div" sx={{ lineHeight: 1.3 }}>
                        {t('playground.expression.title')}
                    </Typography>
                    <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                        {t('playground.expression.subtitle')}
                    </Typography>
                </Box>
                <IconButton onClick={onClose} aria-label={t('playground.expression.close')}>
                    <Close />
                </IconButton>
            </DialogTitle>
            <DialogContent dividers>
                {!stage ? (
                    <Stack spacing={1.5} sx={{ alignItems: 'center', justifyContent: 'center', minHeight: PREVIEW_PX }}>
                        {failed ? (
                            <Typography variant="body2" sx={{ color: 'error.main' }}>
                                {t('playground.expression.loadFailed')}
                            </Typography>
                        ) : (
                            <>
                                <CircularProgress size={28} />
                                <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                                    {t('playground.expression.loading')}
                                </Typography>
                            </>
                        )}
                    </Stack>
                ) : (
                    <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2} sx={{ alignItems: { xs: 'center', sm: 'flex-start' } }}>
                        {/* What is sent, at the size it is judged by. */}
                        <Box sx={{ flexShrink: 0, border: '1px solid', borderColor: 'divider', borderRadius: 1.5, p: 0.5 }}>
                            <FaceCanvas stage={stage} state={state} size={PREVIEW_PX} />
                        </Box>
                        <Stack spacing={1.5} sx={{ flex: 1, minWidth: 0, width: '100%' }}>
                            <Box
                                role="radiogroup"
                                aria-label={t('playground.expression.presets')}
                                sx={{ display: 'grid', gridTemplateColumns: `repeat(auto-fill, minmax(${THUMB_PX + 10}px, 1fr))`, gap: 0.75 }}
                            >
                                {EXPRESSION_PRESET_KEYS.map((key) => {
                                    const label = t(`playground.expression.preset.${key}`);
                                    const selected = current === key;
                                    return (
                                        <ButtonBase
                                            key={key}
                                            role="radio"
                                            aria-checked={selected}
                                            aria-label={label}
                                            onClick={() => setState(EXPRESSION_PRESETS[key])}
                                            sx={{
                                                flexDirection: 'column',
                                                p: 0.5,
                                                borderRadius: 1,
                                                border: '1px solid',
                                                borderColor: selected ? 'primary.main' : 'divider',
                                                bgcolor: selected ? 'action.selected' : 'background.paper',
                                                '&:hover': { borderColor: 'primary.main' },
                                            }}
                                        >
                                            <FaceCanvas stage={stage} state={EXPRESSION_PRESETS[key]} size={THUMB_PX} />
                                            <Typography variant="caption" sx={{ fontSize: 11, mt: 0.25, color: 'text.primary' }}>{label}</Typography>
                                        </ButtonBase>
                                    );
                                })}
                            </Box>
                            {/* Fine-tuning stays folded: a preset is what most
                                people want, and nine sliders up front would be
                                the whole dialog. */}
                            <Box>
                                <Button
                                    size="small"
                                    color="inherit"
                                    onClick={() => setTuning((v) => !v)}
                                    endIcon={<ExpandMore sx={{ transform: tuning ? 'rotate(180deg)' : 'none', transition: 'transform .2s' }} />}
                                    sx={{ textTransform: 'none', color: 'text.secondary', px: 0.5 }}
                                    aria-expanded={tuning}
                                >
                                    {t('playground.expression.fineTune')}
                                </Button>
                                <Collapse in={tuning}>
                                    <Box sx={{ display: 'grid', gridTemplateColumns: '88px 1fr', columnGap: 1.5, rowGap: 0, alignItems: 'center', pt: 0.5 }}>
                                        {WEIGHT_SLIDERS.map((key) => (
                                            <SliderRow
                                                key={key}
                                                label={weightLabel(key)}
                                                value={state.weights[key] ?? 0}
                                                min={0}
                                                onChange={(v) => setState((s) => withWeight(s, key, v))}
                                            />
                                        ))}
                                        <SliderRow
                                            label={t('playground.expression.control.gazeX')}
                                            value={state.gaze.x}
                                            min={-1}
                                            onChange={(v) => setState((s) => ({ ...s, gaze: { ...s.gaze, x: v } }))}
                                        />
                                        <SliderRow
                                            label={t('playground.expression.control.gazeY')}
                                            value={state.gaze.y}
                                            min={-1}
                                            onChange={(v) => setState((s) => ({ ...s, gaze: { ...s.gaze, y: v } }))}
                                        />
                                    </Box>
                                </Collapse>
                            </Box>
                        </Stack>
                    </Stack>
                )}
            </DialogContent>
            <DialogActions sx={{ px: 2, py: 1.5 }}>
                <Typography variant="caption" sx={{ color: 'text.secondary', mr: 'auto' }}>
                    {t('playground.expression.hint')}
                </Typography>
                <Button onClick={onClose}>{t('playground.expression.cancel')}</Button>
                <Button variant="contained" onClick={() => { void handleSubmit(); }} disabled={!stage || saving} startIcon={<MoodSmile />}>
                    {initial ? t('playground.expression.update') : t('playground.expression.add')}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

const SliderRow: React.FC<{ label: string; value: number; min: number; onChange: (value: number) => void }> = ({ label, value, min, onChange }) => (
    <>
        <Typography variant="caption" sx={{ color: 'text.secondary' }}>{label}</Typography>
        <Slider
            size="small"
            value={value}
            min={min}
            max={1}
            step={0.05}
            track={min < 0 ? false : 'normal'}
            onChange={(_, next) => onChange(next as number)}
            aria-label={label}
        />
    </>
);

export default ExpressionDialog;
