import {Box, Checkbox, Chip, FormControl, FormLabel, Stack, Typography} from '@mui/material';
import React from 'react';
import {useTranslation} from 'react-i18next';
import type {UniqueProvider} from '../../services/serviceProviders';
import {Anthropic, OpenAI} from '../BrandIcons';
import ProtocolBaseUrlDisplay from './ProtocolBaseUrlDisplay';

interface ProtocolSelectorProps {
    selectedProvider: UniqueProvider | null;
    protocolOpenAI: boolean;
    protocolAnthropic: boolean;
    fusionLocked: boolean;
    openAICapabilities: string[];
    onToggleOpenAI: () => void;
    onToggleAnthropic: () => void;
}

const ProtocolSelector: React.FC<ProtocolSelectorProps> = ({
    selectedProvider,
    protocolOpenAI,
    protocolAnthropic,
    fusionLocked,
    openAICapabilities,
    onToggleOpenAI,
    onToggleAnthropic,
}) => {
    const {t} = useTranslation();

    const openAIDisabled = fusionLocked || (selectedProvider ? !selectedProvider.supportsOpenAI : false);
    const anthropicDisabled = fusionLocked || (selectedProvider ? !selectedProvider.supportsAnthropic : false);

    return (
        <FormControl component="fieldset">
            <FormLabel component="legend" sx={{mb: 1}}>
                {t('providerDialog.apiStyle.label')}
            </FormLabel>
            <Stack spacing={1}>
                <Box
                    sx={{
                        borderRadius: 1,
                        px: 1.5,
                        py: 1,
                        cursor: fusionLocked ? 'not-allowed' : 'pointer',
                        transition: 'all 0.15s',
                        bgcolor: protocolOpenAI ? 'action.selected' : 'transparent',
                        '&:hover': {
                            bgcolor:
                                fusionLocked
                                    ? protocolOpenAI
                                        ? 'action.selected'
                                        : 'transparent'
                                    : protocolOpenAI
                                        ? 'action.selected'
                                        : 'action.hover',
                        },
                    }}
                    onClick={onToggleOpenAI}
                >
                    <Stack direction="row" alignItems="flex-start" spacing={1}>
                        <OpenAI size={18} sx={{mt: 0.2}}/>
                        <Box sx={{flex: 1}}>
                            <Typography variant="body2" fontWeight={500}>
                                {t('providerDialog.apiStyle.openAI')}
                            </Typography>
                            <Typography
                                variant="caption"
                                color="text.secondary"
                                sx={{display: 'block', lineHeight: 1.2}}
                            >
                                {openAICapabilities.length > 0
                                    ? `Supports ${openAICapabilities.join(' + ')}`
                                    : t('providerDialog.apiStyle.helperOpenAI')}
                            </Typography>
                            <Stack
                                direction="row"
                                spacing={0.75}
                                sx={{mt: 0.75, flexWrap: 'wrap', rowGap: 0.75}}
                            >
                                {openAICapabilities.length > 0 &&
                                    openAICapabilities.map(capability => (
                                        <Chip
                                            key={capability}
                                            label={capability}
                                            size="small"
                                            variant="outlined"
                                            color="primary"
                                        />
                                    ))}
                            </Stack>
                            {selectedProvider?.baseUrlOpenAI && (
                                <ProtocolBaseUrlDisplay url={selectedProvider.baseUrlOpenAI}/>
                            )}
                        </Box>
                        <Checkbox
                            size="small"
                            checked={protocolOpenAI}
                            disabled={openAIDisabled}
                            sx={{p: 0, mt: -0.5}}
                            onClick={(e) => e.stopPropagation()}
                            onChange={onToggleOpenAI}
                        />
                    </Stack>
                </Box>

                <Box
                    sx={{
                        borderRadius: 1,
                        px: 1.5,
                        py: 1,
                        cursor: fusionLocked ? 'not-allowed' : 'pointer',
                        transition: 'all 0.15s',
                        bgcolor: protocolAnthropic ? 'action.selected' : 'transparent',
                        '&:hover': {
                            bgcolor:
                                fusionLocked
                                    ? protocolAnthropic
                                        ? 'action.selected'
                                        : 'transparent'
                                    : protocolAnthropic
                                        ? 'action.selected'
                                        : 'action.hover',
                        },
                    }}
                    onClick={onToggleAnthropic}
                >
                    <Stack direction="row" alignItems="flex-start" spacing={1}>
                        <Anthropic size={18} sx={{mt: 0.2}}/>
                        <Box sx={{flex: 1}}>
                            <Typography variant="body2" fontWeight={500}>
                                {t('providerDialog.apiStyle.anthropic')}
                            </Typography>
                            <Typography
                                variant="caption"
                                color="text.secondary"
                                sx={{display: 'block', lineHeight: 1.2}}
                            >
                                {t('providerDialog.apiStyle.helperAnthropic')}
                            </Typography>
                            {selectedProvider?.baseUrlAnthropic && (
                                <ProtocolBaseUrlDisplay url={selectedProvider.baseUrlAnthropic}/>
                            )}
                        </Box>
                        <Checkbox
                            size="small"
                            checked={protocolAnthropic}
                            disabled={anthropicDisabled}
                            sx={{p: 0, mt: -0.5}}
                            onClick={(e) => e.stopPropagation()}
                            onChange={onToggleAnthropic}
                        />
                    </Stack>
                </Box>
            </Stack>
        </FormControl>
    );
};

export default ProtocolSelector;
