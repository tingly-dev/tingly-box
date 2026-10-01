import { useState } from 'react';
import {
    Box,
    Button,
    ButtonBase,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    IconButton,
    MenuItem,
    Paper,
    Select,
    Stack,
    TextField,
    Tooltip,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import ConfirmDialog from '@/components/ConfirmDialog';
import { CenterFocusStrong, Close, Delete } from '@/components/icons';
import type { Workbench } from '@/services/imageArchiveApi';
import ThumbImage from './ThumbImage';
import { THUMB_EDGE_BADGE } from './imageThumbnails';

// The focus workbench at the top of the controls column: the image being
// worked on, the description of its subject (sent ahead of every prompt made
// here), and what has been derived from it. With no workbench yet it renders
// nothing — the way in is "Focus on this image" on any image's viewer.
// See .design/image-workbench.md.

interface ImageWorkbenchBarProps {
    workbenches: Workbench[];
    active: Workbench | null;
    srcs: Record<string, string>;
    onFocus: (id: string | null) => void;
    onUpdate: (id: string, body: { name?: string; description?: string }) => void;
    onDelete: (id: string) => void;
    // Click on any of its images: put it in the request as a reference.
    onUseImage: (imageId: string) => void;
}

const thumbSx = {
    width: 48,
    height: 48,
    flexShrink: 0,
    borderRadius: 1,
    overflow: 'hidden',
    border: 1,
    borderColor: 'divider',
    bgcolor: 'action.hover',
} as const;

const ImageWorkbenchBar: React.FC<ImageWorkbenchBarProps> = ({
    workbenches,
    active,
    srcs,
    onFocus,
    onUpdate,
    onDelete,
    onUseImage,
}) => {
    const { t } = useTranslation();
    const [confirmDelete, setConfirmDelete] = useState(false);

    if (workbenches.length === 0) return null;

    const switcher = (
        <Select
            size="small"
            displayEmpty
            value={active?.id ?? ''}
            onChange={(event) => onFocus(event.target.value || null)}
            inputProps={{ 'aria-label': t('playground.workbench.switch', { defaultValue: 'Workbench' }) }}
            sx={{ flex: 1, minWidth: 0, '& .MuiSelect-select': { py: 0.5 } }}
            renderValue={(value) => (value
                ? workbenches.find((wb) => wb.id === value)?.name || t('playground.workbench.untitled', { defaultValue: 'Untitled' })
                : t('playground.workbench.none', { defaultValue: 'No focus — free generation' }))}
        >
            <MenuItem value="">{t('playground.workbench.none', { defaultValue: 'No focus — free generation' })}</MenuItem>
            {workbenches.map((wb) => (
                <MenuItem key={wb.id} value={wb.id}>
                    {wb.name || t('playground.workbench.untitled', { defaultValue: 'Untitled' })}
                    <Typography component="span" variant="caption" color="text.secondary" sx={{ ml: 1 }}>
                        {t('playground.workbench.itemCount', { defaultValue: '{{count}} derived', count: wb.items?.length ?? 0 })}
                    </Typography>
                </MenuItem>
            ))}
        </Select>
    );

    if (!active) {
        return (
            <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                <CenterFocusStrong fontSize="small" color="action" />
                {switcher}
            </Stack>
        );
    }

    const items = active.items ?? [];
    const imageTile = (imageId: string, label: string) => (
        <Tooltip key={imageId} title={label}>
            <ButtonBase
                onClick={() => onUseImage(imageId)}
                aria-label={label}
                sx={thumbSx}
            >
                {srcs[imageId] && <ThumbImage src={srcs[imageId]} alt={label} edge={THUMB_EDGE_BADGE} />}
            </ButtonBase>
        </Tooltip>
    );
    const useLabel = t('playground.workbench.useImage', { defaultValue: 'Use as reference' });

    return (
        <Paper variant="outlined" sx={{ p: 1.25, borderColor: 'primary.main' }}>
            <Stack spacing={1}>
                <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                    <CenterFocusStrong fontSize="small" color="primary" />
                    {switcher}
                    <Tooltip title={t('playground.workbench.delete', { defaultValue: 'Delete workbench' })}>
                        <IconButton size="small" onClick={() => setConfirmDelete(true)}
                            aria-label={t('playground.workbench.delete', { defaultValue: 'Delete workbench' })}>
                            <Delete fontSize="small" />
                        </IconButton>
                    </Tooltip>
                    <Tooltip title={t('playground.workbench.leave', { defaultValue: 'Leave focus' })}>
                        <IconButton size="small" edge="end" onClick={() => onFocus(null)}
                            aria-label={t('playground.workbench.leave', { defaultValue: 'Leave focus' })}>
                            <Close fontSize="small" />
                        </IconButton>
                    </Tooltip>
                </Stack>
                <Stack direction="row" spacing={1} sx={{ alignItems: 'flex-start' }}>
                    {imageTile(active.root_image_id, `${t('playground.workbench.root', { defaultValue: 'Focus image' })} · ${useLabel}`)}
                    {/* Keyed by what is saved, so switching workbenches (or a save
                        landing) starts the field from the stored text. */}
                    <DescriptionField
                        key={`${active.id}:${active.description}`}
                        saved={active.description}
                        onSave={(description) => onUpdate(active.id, { description })}
                    />
                </Stack>
                {items.length > 0 && (
                    <Box sx={{ display: 'flex', gap: 0.75, overflowX: 'auto', pb: 0.25, scrollbarWidth: 'thin' }}>
                        {items.map((item) => imageTile(item.image_id, useLabel))}
                    </Box>
                )}
            </Stack>
            <ConfirmDialog
                open={confirmDelete}
                title={t('playground.workbench.deleteTitle', { defaultValue: 'Delete “{{name}}”?', name: active.name })}
                description={t('playground.workbench.deleteBody', {
                    defaultValue: 'Only the workbench goes. Its images stay in the output folder.',
                })}
                confirmLabel={t('common.delete', { defaultValue: 'Delete' })}
                cancelLabel={t('common.cancel', { defaultValue: 'Cancel' })}
                confirmColor="error"
                onClose={() => setConfirmDelete(false)}
                onConfirm={() => { setConfirmDelete(false); onDelete(active.id); }}
            />
        </Paper>
    );
};

export default ImageWorkbenchBar;

// The description, edited in place and saved when the field loses focus.
const DescriptionField: React.FC<{ saved: string; onSave: (description: string) => void }> = ({ saved, onSave }) => {
    const { t } = useTranslation();
    const [description, setDescription] = useState(saved);
    return (
        <TextField
            size="small"
            multiline
            minRows={2}
            maxRows={4}
            fullWidth
            value={description}
            onChange={(event) => setDescription(event.target.value)}
            onBlur={() => { if (description !== saved) onSave(description); }}
            placeholder={t('playground.workbench.descriptionPlaceholder', {
                defaultValue: 'Describe the subject — who or what this is, what must stay the same',
            })}
            helperText={description.trim()
                ? t('playground.workbench.descriptionHint', { defaultValue: 'Sent ahead of every prompt in this workbench' })
                : undefined}
            slotProps={{ formHelperText: { sx: { mx: 0 } } }}
        />
    );
};

// Naming a new workbench. Both fields start filled from the image — a name
// from its prompt or file, the description from its prompt — so the common
// case is one click.
interface CreateWorkbenchDialogProps {
    open: boolean;
    defaultName: string;
    defaultDescription: string;
    onClose: () => void;
    onCreate: (name: string, description: string) => void;
    busy: boolean;
}

export const CreateWorkbenchDialog: React.FC<CreateWorkbenchDialogProps> = (props) => (
    <Dialog open={props.open} onClose={props.onClose} maxWidth="sm" fullWidth>
        {/* Remounted per image, so each opening starts from that image's defaults. */}
        {props.open && <CreateWorkbenchForm key={`${props.defaultName}:${props.defaultDescription}`} {...props} />}
    </Dialog>
);

const CreateWorkbenchForm: React.FC<CreateWorkbenchDialogProps> = ({
    defaultName,
    defaultDescription,
    onClose,
    onCreate,
    busy,
}) => {
    const { t } = useTranslation();
    const [name, setName] = useState(defaultName);
    const [description, setDescription] = useState(defaultDescription);

    return (
        <>
            <DialogTitle>{t('playground.workbench.createTitle', { defaultValue: 'Focus on this image' })}</DialogTitle>
            <DialogContent>
                <Stack spacing={2} sx={{ pt: 1 }}>
                    <Typography variant="body2" color="text.secondary">
                        {t('playground.workbench.createBody', {
                            defaultValue: 'A workbench keeps this image and its description in every request, and collects what you make from it. It is saved with your images, not in this browser.',
                        })}
                    </Typography>
                    <TextField
                        autoFocus
                        size="small"
                        label={t('playground.workbench.name', { defaultValue: 'Name' })}
                        value={name}
                        onChange={(event) => setName(event.target.value)}
                    />
                    <TextField
                        size="small"
                        multiline
                        minRows={3}
                        maxRows={8}
                        label={t('playground.workbench.description', { defaultValue: 'Description' })}
                        placeholder={t('playground.workbench.descriptionPlaceholder', {
                            defaultValue: 'Describe the subject — who or what this is, what must stay the same',
                        })}
                        value={description}
                        onChange={(event) => setDescription(event.target.value)}
                    />
                </Stack>
            </DialogContent>
            <DialogActions sx={{ px: 3, pb: 2 }}>
                <Button onClick={onClose}>{t('common.cancel', { defaultValue: 'Cancel' })}</Button>
                <Button
                    variant="contained"
                    disabled={busy || !name.trim()}
                    onClick={() => onCreate(name.trim(), description)}
                >
                    {t('playground.workbench.create', { defaultValue: 'Start workbench' })}
                </Button>
            </DialogActions>
        </>
    );
};
