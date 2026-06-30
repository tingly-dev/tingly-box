import {OpenAI, Anthropic} from '../BrandIcons';
import {Box, Checkbox, InputBase, Link, Stack, Tooltip, Typography} from '@mui/material';
import {useTranslation} from 'react-i18next';

export interface ProtocolSlotData {
    url: string;
    enabled: boolean;
}

export type ProtocolKind = 'openai' | 'anthropic';

interface ProtocolSlotProps {
    kind: ProtocolKind;
    slot: ProtocolSlotData;
    onUrlChange: (url: string) => void;
    onUrlBlur: () => void;
    onToggle: () => void;
    helperText?: string;
    urlError?: boolean;
    /** Persistent /v1 suffix hint — shown above the URL field when true. */
    v1Hint?: { show: boolean; onApply: () => void };
}

interface BrandDef {
    icon: React.ReactNode;
    labelKey: string;
    defaultLabel: string;
}

const BRAND: Record<ProtocolKind, BrandDef> = {
    openai: {
        icon: <OpenAI size={18}/>,
        labelKey: 'providerDialog.protocol.openAILabel',
        defaultLabel: 'OpenAI Compatible',
    },
    anthropic: {
        icon: <Anthropic size={18}/>,
        labelKey: 'providerDialog.protocol.anthropicLabel',
        defaultLabel: 'Anthropic Compatible',
    },
};

const DEFAULT_HELPERS: Record<ProtocolKind, string> = {
    openai: 'Supports models from OpenAI, Google and many other OpenAI-compatible providers',
    anthropic: 'For Anthropic-compatible AI providers, commonly used with Claude Code',
};

const ProtocolSlot: React.FC<ProtocolSlotProps> = ({
    kind,
    slot,
    onUrlChange,
    onUrlBlur,
    onToggle,
    helperText,
    urlError,
    v1Hint,
}) => {
    const {t} = useTranslation();
    const brand = BRAND[kind];
    const helper = helperText || DEFAULT_HELPERS[kind];
    const enabled = slot.enabled;

    return (
        <Box
            sx={{
                borderRadius: 1,
                px: 1.5,
                py: 1,
                cursor: 'pointer',
                transition: 'all 0.15s',
                bgcolor: enabled ? 'action.selected' : 'transparent',
                '&:hover': { bgcolor: enabled ? 'action.selected' : 'action.hover' },
            }}
            onClick={onToggle}
        >
            <Box sx={{display: 'flex', alignItems: 'flex-start', gap: 1}}>
                <Box sx={{mt: 0.2, flexShrink: 0}}>{brand.icon}</Box>
                <Box sx={{flex: 1, minWidth: 0}}>
                    <Typography variant="body2" fontWeight={500}>
                        {t(brand.labelKey, {defaultValue: brand.defaultLabel})}
                    </Typography>
                    <Typography
                        variant="caption"
                        color="text.secondary"
                        sx={{display: 'block', lineHeight: 1.3, mt: 0.15}}
                    >
                        {helper}
                    </Typography>
                </Box>
                <Checkbox
                    size="small"
                    checked={enabled}
                    sx={{p: 0, mt: -0.5, flexShrink: 0}}
                    onClick={(e) => e.stopPropagation()}
                    onChange={onToggle}
                />
            </Box>

            {enabled && (
                <Tooltip
                    open={v1Hint?.show ?? false}
                    title={
                        <Stack spacing={1} sx={{maxWidth: 160}}>
                            <Typography variant="caption" color="text.secondary" sx={{lineHeight: 1.4}}>
                                {t('providerDialog.v1Hint.message', {
                                    defaultValue: 'Most OpenAI-compatible APIs need a /v1 suffix.',
                                })}
                            </Typography>
                            <Link
                                component="button"
                                type="button"
                                variant="caption"
                                onClick={(e) => {
                                    e.stopPropagation();
                                    v1Hint?.onApply();
                                }}
                                underline="always"
                                sx={{fontWeight: 600, whiteSpace: 'nowrap', alignSelf: 'flex-start'}}
                            >
                                {t('providerDialog.v1Hint.apply', {defaultValue: 'Append /v1'})}
                            </Link>
                        </Stack>
                    }
                    placement="left"
                    arrow
                    disableFocusListener
                    disableHoverListener
                    disableTouchListener
                    slotProps={{
                        tooltip: {
                            sx: {
                                bgcolor: 'background.paper',
                                color: 'text.primary',
                                border: 1,
                                borderColor: 'divider',
                                boxShadow: 2,
                                px: 1.5,
                                py: 1,
                            },
                        },
                        arrow: {
                            sx: {
                                fontSize: 16,
                                color: 'background.paper',
                                '&::before': {border: 1, borderColor: 'divider'},
                            },
                        },
                    }}
                >
                    <InputBase
                        fullWidth
                        size="small"
                        placeholder={t('providerDialog.provider.placeholder', {defaultValue: 'Base URL'})}
                        value={slot.url}
                        onChange={(e) => onUrlChange(e.target.value)}
                        onBlur={onUrlBlur}
                        error={urlError}
                        onClick={(e) => e.stopPropagation()}
                        sx={{
                            mt: 1.25,
                            px: 1.5,
                            py: 0.75,
                            fontSize: '0.8rem',
                            fontFamily: 'monospace',
                            color: 'primary.main',
                            bgcolor: 'background.default',
                            borderRadius: 0.75,
                            border: urlError ? 1 : '1px solid transparent',
                            borderColor: urlError ? 'error.main' : 'divider',
                            '&:hover': {borderColor: 'text.disabled'},
                            '&:focus-within': {
                                borderColor: 'primary.main',
                                boxShadow: '0 0 0 1px rgba(25,118,210,0.12)',
                            },
                        }}
                    />
                </Tooltip>
            )}
        </Box>
    );
};

export default ProtocolSlot;
