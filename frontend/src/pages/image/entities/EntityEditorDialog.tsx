import { useEffect, useRef, useState } from 'react';
import {
    Box,
    Button,
    ButtonBase,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    IconButton,
    Stack,
    TextField,
    Tooltip,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Add, ChevronLeft, ChevronRight, Close, DeleteOutline } from '@/components/icons';
import type { EntityKind, ImageEntity } from './entityTypes';
import { ENTITY_KINDS } from './entityTypes';
import { newEntityId, removeEntity, saveEntity, useEntities } from './entityStore';
import { KIND_COLOR, KindIcon, useKindLabel } from './entityUi';

export type EntityDraft = Partial<ImageEntity>;

interface Props {
    // null = closed. A draft without an id is a new entity.
    draft: EntityDraft | null;
    onClose: () => void;
    onSaved?: (entity: ImageEntity) => void;
    // Shown for an existing entity when the dialog is opened away from the
    // playground: the next thing to do with an entity is use it.
    onUse?: (entity: ImageEntity) => void;
}

const EntityEditorDialog: React.FC<Props> = ({ draft, onClose, onSaved, onUse }) => {
    const { t } = useTranslation();
    const kindLabel = useKindLabel();
    const entities = useEntities();
    const fileInputRef = useRef<HTMLInputElement>(null);
    const [kind, setKind] = useState<EntityKind>('character');
    const [name, setName] = useState('');
    const [refs, setRefs] = useState<string[]>([]);
    const [prompt, setPrompt] = useState('');
    const [negative, setNegative] = useState('');

    useEffect(() => {
        if (!draft) return;
        setKind(draft.kind ?? 'character');
        setName(draft.name ?? '');
        setRefs(draft.refs ?? []);
        setPrompt(draft.prompt ?? '');
        setNegative(draft.negative ?? '');
    }, [draft]);

    const isNew = !draft?.id;
    const trimmed = name.trim();
    const nameTaken = entities.some((entity) => entity.name === trimmed && entity.id !== draft?.id);
    const nameInvalid = /[\s@]/.test(trimmed);
    const canSave = Boolean(trimmed) && !nameTaken && !nameInvalid && (refs.length > 0 || Boolean(prompt.trim()));

    const kindHelp: Record<EntityKind, string> = {
        character: t('imageEntity.editor.characterHelp', { defaultValue: 'Pins who or what is in the picture: a person, a pet, a product.' }),
        style: t('imageEntity.editor.styleHelp', { defaultValue: 'Pins how the picture is drawn: medium, light, palette.' }),
    };

    const move = (index: number, step: number) => {
        setRefs((current) => {
            const next = [...current];
            const target = index + step;
            if (target < 0 || target >= next.length) return current;
            [next[index], next[target]] = [next[target], next[index]];
            return next;
        });
    };

    const handleSave = () => {
        if (!canSave) return;
        const entity: ImageEntity = {
            id: draft?.id ?? newEntityId(),
            kind,
            name: trimmed,
            refs,
            prompt: prompt.trim(),
            negative: negative.trim() || undefined,
            uses: draft?.uses ?? 0,
            updatedAt: Date.now(),
        };
        saveEntity(entity);
        onSaved?.(entity);
        onClose();
    };

    return (
        <Dialog open={draft !== null} onClose={onClose} maxWidth="sm" fullWidth>
            <DialogTitle sx={{ display: 'flex', alignItems: 'center', pr: 1 }}>
                <Typography variant="h6" component="span" sx={{ flex: 1, fontSize: '1.05rem' }}>
                    {isNew
                        ? t('imageEntity.editor.newTitle', { defaultValue: 'New entity' })
                        : t('imageEntity.editor.editTitle', { defaultValue: 'Edit @{{name}}', name: draft?.name ?? '' })}
                </Typography>
                <IconButton onClick={onClose} aria-label={t('common.cancel', { defaultValue: 'Cancel' })}>
                    <Close />
                </IconButton>
            </DialogTitle>
            <DialogContent dividers>
                <Stack spacing={2.5}>
                    {/* The kind decides how an @mention expands, so it is a
                        real choice — shown as two described options rather
                        than a bare dropdown. */}
                    <Box>
                        <Typography sx={{ fontSize: 13, fontWeight: 600, mb: 1 }}>
                            {t('imageEntity.editor.kind', { defaultValue: 'What it pins' })}
                        </Typography>
                        <Box sx={{ display: 'grid', gridTemplateColumns: 'repeat(2, minmax(0, 1fr))', gap: 1 }}>
                            {ENTITY_KINDS.map((option) => {
                                const selected = option === kind;
                                return (
                                    <ButtonBase
                                        key={option}
                                        onClick={() => setKind(option)}
                                        aria-pressed={selected}
                                        sx={{
                                            p: 1.25,
                                            borderRadius: 1.5,
                                            border: '1px solid',
                                            borderColor: selected ? KIND_COLOR[option].fg : 'divider',
                                            bgcolor: selected ? KIND_COLOR[option].bg : 'transparent',
                                            textAlign: 'left',
                                            alignItems: 'flex-start',
                                            justifyContent: 'flex-start',
                                            flexDirection: 'column',
                                            gap: 0.5,
                                        }}
                                    >
                                        <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center' }}>
                                            <KindIcon kind={option} />
                                            <Typography sx={{ fontSize: 14, fontWeight: 600 }}>{kindLabel(option)}</Typography>
                                        </Stack>
                                        <Typography sx={{ fontSize: 12, color: 'text.secondary' }}>{kindHelp[option]}</Typography>
                                    </ButtonBase>
                                );
                            })}
                        </Box>
                    </Box>

                    <TextField
                        label={t('imageEntity.editor.name', { defaultValue: 'Name' })}
                        value={name}
                        onChange={(event) => setName(event.target.value)}
                        size="small"
                        error={nameTaken || nameInvalid}
                        helperText={nameTaken
                            ? t('imageEntity.editor.nameTaken', { defaultValue: 'Another entity already has this name.' })
                            : nameInvalid
                                ? t('imageEntity.editor.nameInvalid', { defaultValue: 'No spaces or @ — the name is what follows @ in a prompt.' })
                                : t('imageEntity.editor.nameHelp', { defaultValue: 'Referenced as @{{name}} in prompts.', name: trimmed || '…' })}
                        autoFocus={isNew}
                    />

                    <Box>
                        <Stack direction="row" sx={{ alignItems: 'baseline', mb: 1 }}>
                            <Typography sx={{ fontSize: 13, fontWeight: 600, flex: 1 }}>
                                {t('imageEntity.editor.refs', { defaultValue: 'Reference images' })}
                            </Typography>
                            <Typography sx={{ fontSize: 12, color: 'text.secondary' }}>
                                {t('imageEntity.editor.refsOrder', { defaultValue: 'In order of priority — when a run is short on slots, the first ones go.' })}
                            </Typography>
                        </Stack>
                        <Box sx={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(92px, 1fr))', gap: 1 }}>
                            {refs.map((src, index) => (
                                <Box
                                    key={`${src.slice(-24)}-${index}`}
                                    sx={{
                                        position: 'relative',
                                        aspectRatio: kind === 'character' ? '3 / 4' : '1 / 1',
                                        borderRadius: 1,
                                        overflow: 'hidden',
                                        bgcolor: 'action.hover',
                                        '& img': { width: '100%', height: '100%', objectFit: 'cover', display: 'block' },
                                        '&:hover .ref-actions, &:focus-within .ref-actions': { opacity: 1 },
                                    }}
                                >
                                    <img src={src} alt="" />
                                    <Box
                                        sx={{
                                            position: 'absolute',
                                            top: 4,
                                            left: 4,
                                            minWidth: 20,
                                            height: 20,
                                            borderRadius: 10,
                                            bgcolor: 'rgba(0,0,0,0.6)',
                                            color: '#fff',
                                            fontSize: 12,
                                            display: 'flex',
                                            alignItems: 'center',
                                            justifyContent: 'center',
                                        }}
                                    >
                                        {index + 1}
                                    </Box>
                                    <Stack
                                        className="ref-actions"
                                        direction="row"
                                        sx={{
                                            position: 'absolute',
                                            left: 0,
                                            right: 0,
                                            bottom: 0,
                                            justifyContent: 'space-between',
                                            bgcolor: 'rgba(0,0,0,0.55)',
                                            opacity: 0,
                                            transition: 'opacity 120ms',
                                            '& .MuiIconButton-root': { color: '#fff', p: 0.5 },
                                        }}
                                    >
                                        <IconButton size="small" disabled={index === 0} onClick={() => move(index, -1)} aria-label={t('imageEntity.editor.moveEarlier', { defaultValue: 'Move earlier' })}>
                                            <ChevronLeft sx={{ fontSize: 16 }} />
                                        </IconButton>
                                        <IconButton size="small" onClick={() => setRefs((current) => current.filter((_, i) => i !== index))} aria-label={t('imageEntity.editor.removeRef', { defaultValue: 'Remove image' })}>
                                            <DeleteOutline sx={{ fontSize: 16 }} />
                                        </IconButton>
                                        <IconButton size="small" disabled={index === refs.length - 1} onClick={() => move(index, 1)} aria-label={t('imageEntity.editor.moveLater', { defaultValue: 'Move later' })}>
                                            <ChevronRight sx={{ fontSize: 16 }} />
                                        </IconButton>
                                    </Stack>
                                </Box>
                            ))}
                            <ButtonBase
                                onClick={() => fileInputRef.current?.click()}
                                sx={{
                                    aspectRatio: kind === 'character' ? '3 / 4' : '1 / 1',
                                    borderRadius: 1,
                                    border: '1px dashed',
                                    borderColor: 'divider',
                                    flexDirection: 'column',
                                    gap: 0.5,
                                    color: 'text.secondary',
                                }}
                            >
                                <Add />
                                <Typography sx={{ fontSize: 12 }}>{t('imageEntity.editor.addRef', { defaultValue: 'Add image' })}</Typography>
                            </ButtonBase>
                        </Box>
                        <input
                            ref={fileInputRef}
                            type="file"
                            accept="image/*"
                            multiple
                            hidden
                            onChange={(event) => {
                                const files = Array.from(event.target.files ?? []);
                                setRefs((current) => [...current, ...files.map((file) => URL.createObjectURL(file))]);
                                event.target.value = '';
                            }}
                        />
                    </Box>

                    <TextField
                        label={t('imageEntity.editor.prompt', { defaultValue: 'Standard description' })}
                        value={prompt}
                        onChange={(event) => setPrompt(event.target.value)}
                        multiline
                        minRows={2}
                        helperText={kind === 'character'
                            ? t('imageEntity.editor.promptCharacterHelp', { defaultValue: 'Inserted where @{{name}} appears.', name: trimmed || '…' })
                            : t('imageEntity.editor.promptStyleHelp', { defaultValue: 'Appended to the end of the prompt — a style describes the whole picture.' })}
                    />
                    <TextField
                        label={t('imageEntity.editor.negative', { defaultValue: 'Avoid (optional)' })}
                        value={negative}
                        onChange={(event) => setNegative(event.target.value)}
                        size="small"
                    />
                </Stack>
            </DialogContent>
            <DialogActions sx={{ px: 3, py: 2 }}>
                {!isNew && draft?.id && (
                    <Tooltip title={t('imageEntity.editor.deleteHint', { defaultValue: 'Prompts that mention it keep the text @{{name}}.', name: draft.name ?? '' })}>
                        <Button
                            color="error"
                            startIcon={<DeleteOutline />}
                            onClick={() => { removeEntity(draft.id as string); onClose(); }}
                            sx={{ mr: 'auto' }}
                        >
                            {t('common.delete', { defaultValue: 'Delete' })}
                        </Button>
                    </Tooltip>
                )}
                {!isNew && onUse && draft && (
                    <Button
                        onClick={() => {
                            const current = entities.find((entity) => entity.id === draft.id);
                            if (current) onUse(current);
                        }}
                    >
                        {t('imageEntity.editor.use', { defaultValue: 'Use in Playground' })}
                    </Button>
                )}
                <Button variant="contained" onClick={handleSave} disabled={!canSave}>
                    {t('imageEntity.editor.save', { defaultValue: 'Save' })}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default EntityEditorDialog;
