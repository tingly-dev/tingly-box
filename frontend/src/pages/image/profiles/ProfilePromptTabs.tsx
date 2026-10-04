import { Box, ButtonBase, IconButton, Stack, Tooltip } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Add, Close } from '@/components/icons';
import type { ProfilePrompt } from './imageProfileTypes';
import { deriveLabel } from './promptLabel';

interface Props {
    prompts: ProfilePrompt[];
    activeId: string;
    onSelect: (id: string) => void;
    onAdd: () => void;
    onRemove: (id: string) => void;
}

// A profile's prompts as a row of small tabs over the one prompt field —
// switching swaps what the field holds, nothing more. Text-weight, not
// chips: it is a header for the field below, not a second control panel.
// Tabs are labelled by their prompt's opening words, so naming is never a
// step the user has to take.
const ProfilePromptTabs: React.FC<Props> = ({ prompts, activeId, onSelect, onAdd, onRemove }) => {
    const { t } = useTranslation();
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
                        <ButtonBase
                            role="tab"
                            aria-selected={active}
                            onClick={() => onSelect(item.id)}
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
        </Stack>
    );
};

export default ProfilePromptTabs;
