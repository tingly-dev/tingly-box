import { useState } from 'react';
import { Box, ButtonBase, IconButton, InputBase, Link, Stack, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Add, Close } from '@/components/icons';
import type { ProfilePrompt } from './imageProfileTypes';
import { deriveLabel } from './promptLabel';

interface Props {
    prompts: ProfilePrompt[];
    activeId: string;
    onSelect: (id: string) => void;
    onAdd: () => void;
    onRename: (id: string, name: string) => void;
    onRemove: (id: string) => void;
    // Set for a few seconds after a removal: the way back, in place.
    removed: { label: string; onUndo: () => void } | null;
}

// A profile's prompts as a row of small tabs over the one prompt field —
// switching swaps what the field holds, nothing more. Text-weight, not
// chips: it is a header for the field below, not a second control panel.
const ProfilePromptTabs: React.FC<Props> = ({ prompts, activeId, onSelect, onAdd, onRename, onRemove, removed }) => {
    const { t } = useTranslation();
    const [renamingId, setRenamingId] = useState<string | null>(null);
    const [draft, setDraft] = useState('');

    // Clearing the name hands it back to the automatic label.
    const commit = () => {
        if (renamingId) onRename(renamingId, draft.trim());
        setRenamingId(null);
    };
    const labelOf = (item: ProfilePrompt, index: number) => item.name
        || deriveLabel(item.text)
        || t('imageProfile.promptN', { defaultValue: 'Prompt {{n}}', n: index + 1 });

    return (
        <Stack
            direction="row"
            role="tablist"
            aria-label={t('imageProfile.prompts', { defaultValue: 'Prompts' })}
            sx={{ alignItems: 'center', gap: 0.25, flexWrap: 'wrap', mb: -1 }}
        >
            {prompts.map((item, index) => {
                const active = item.id === activeId;
                if (renamingId === item.id) {
                    return (
                        <InputBase
                            key={item.id}
                            autoFocus
                            value={draft}
                            onChange={(event) => setDraft(event.target.value)}
                            onBlur={commit}
                            onFocus={(event) => event.target.select()}
                            onKeyDown={(event) => {
                                if (event.key === 'Enter') commit();
                                if (event.key === 'Escape') setRenamingId(null);
                            }}
                            inputProps={{ 'aria-label': t('imageProfile.renamePrompt', { defaultValue: 'Prompt name' }) }}
                            sx={{
                                fontSize: 13,
                                px: 1,
                                height: 28,
                                width: 120,
                                borderBottom: 2,
                                borderColor: 'primary.main',
                            }}
                        />
                    );
                }
                return (
                    <Box
                        key={item.id}
                        sx={{
                            display: 'flex',
                            alignItems: 'center',
                            borderBottom: 2,
                            borderColor: active ? 'primary.main' : 'transparent',
                            '&:hover .prompt-remove': { opacity: 1 },
                        }}
                    >
                        {/* describeChild: the hint describes the tab; without it the
                            Tooltip replaces the tab's accessible name with "Double-click
                            to rename". */}
                        <Tooltip describeChild title={active ? t('imageProfile.renameHint', { defaultValue: 'Double-click to rename' }) : ''} enterDelay={600}>
                            <ButtonBase
                                role="tab"
                                aria-selected={active}
                                onClick={() => onSelect(item.id)}
                                onDoubleClick={() => { setDraft(labelOf(item, index)); setRenamingId(item.id); }}
                                sx={{
                                    px: 1,
                                    height: 28,
                                    fontSize: 13,
                                    fontWeight: active ? 600 : 400,
                                    color: active ? 'text.primary' : 'text.secondary',
                                    borderRadius: 0.5,
                                    '&:hover': { color: 'text.primary' },
                                }}
                            >
                                {labelOf(item, index)}
                            </ButtonBase>
                        </Tooltip>
                        {active && prompts.length > 1 && (
                            <IconButton
                                className="prompt-remove"
                                size="small"
                                onClick={() => onRemove(item.id)}
                                aria-label={t('imageProfile.removePrompt', { defaultValue: 'Remove {{name}}', name: labelOf(item, index) })}
                                sx={{ p: 0.25, mr: 0.25, opacity: 0, transition: 'opacity 120ms', '&:focus-visible': { opacity: 1 } }}
                            >
                                <Close sx={{ fontSize: 14 }} />
                            </IconButton>
                        )}
                    </Box>
                );
            })}
            <Tooltip title={t('imageProfile.addPrompt', { defaultValue: 'New prompt' })}>
                <IconButton size="small" onClick={onAdd} aria-label={t('imageProfile.addPrompt', { defaultValue: 'New prompt' })} sx={{ p: 0.5 }}>
                    <Add sx={{ fontSize: 16 }} />
                </IconButton>
            </Tooltip>
            {removed && (
                <Typography role="status" sx={{ fontSize: 12, color: 'text.secondary', ml: 'auto' }}>
                    {t('imageProfile.promptRemoved', { defaultValue: 'Removed “{{name}}”', name: removed.label })}{' '}
                    <Link component="button" type="button" onClick={removed.onUndo} sx={{ fontSize: 12, verticalAlign: 'baseline' }}>
                        {t('imageProfile.undo', { defaultValue: 'Undo' })}
                    </Link>
                </Typography>
            )}
        </Stack>
    );
};

export default ProfilePromptTabs;
