import {Box, FormControl, FormLabel, Radio, Stack, ToggleButton, ToggleButtonGroup, Typography} from '@mui/material';
import React from 'react';
import {useTranslation} from 'react-i18next';
import type {UniqueProvider} from '../../services/serviceProviders';
import {Anthropic, OpenAI} from '../BrandIcons';

// The three orthogonal "shapes" a provider endpoint can take. 'openai' and
// 'anthropic' are both the single-protocol case (distinguished by sub-choice);
// 'fusion' is one endpoint serving both protocols (one provider entry);
// 'dual' is two separate endpoints (creates two provider entries).
export type ProtocolTopology = 'openai' | 'anthropic' | 'fusion' | 'dual';

interface ProtocolTopologySelectorProps {
    selectedProvider: UniqueProvider | null;
    protocolOpenAI: boolean;
    protocolAnthropic: boolean;
    isFusion: boolean;
    // showFusion / showDual gate the multi-protocol options. They are only true
    // when the chosen template exposes BOTH base URLs (free-form custom and
    // single-protocol templates can only be single).
    showFusion: boolean;
    showDual: boolean;
    supportsOpenAI: boolean;
    supportsAnthropic: boolean;
    openAICapabilities: string[];
    onSelect: (topology: ProtocolTopology) => void;
}

type Kind = 'single' | 'fusion' | 'dual' | null;

const ProtocolTopologySelector: React.FC<ProtocolTopologySelectorProps> = ({
    selectedProvider,
    protocolOpenAI,
    protocolAnthropic,
    isFusion,
    showFusion,
    showDual,
    supportsOpenAI,
    supportsAnthropic,
    openAICapabilities,
    onSelect,
}) => {
    const {t} = useTranslation();

    // Derive the currently-selected option from the raw protocol flags.
    let kind: Kind;
    let single: 'openai' | 'anthropic' | null;
    if (protocolOpenAI && protocolAnthropic) {
        kind = isFusion ? 'fusion' : 'dual';
        single = null;
    } else if (protocolOpenAI) {
        kind = 'single';
        single = 'openai';
    } else if (protocolAnthropic) {
        kind = 'single';
        single = 'anthropic';
    } else {
        kind = (showFusion || showDual) ? null : 'single';
        single = null;
    }

    const bothSupported = supportsOpenAI && supportsAnthropic;
    const hasMultiOptions = showFusion || showDual;

    const openAICaption =
        openAICapabilities.length > 0
            ? `${t('providerDialog.apiStyle.openAI')} · ${openAICapabilities.join(' + ')}`
            : t('providerDialog.apiStyle.openAI');

    // Sub-choice toggle (OpenAI / Anthropic) for the single-protocol case.
    const singleToggle = (
        <ToggleButtonGroup
            exclusive
            size="small"
            value={single}
            onChange={(_e, v: 'openai' | 'anthropic' | null) => {
                if (v) onSelect(v);
            }}
            sx={{mt: hasMultiOptions ? 1 : 0}}
        >
            {supportsOpenAI && (
                <ToggleButton value="openai" sx={{textTransform: 'none', gap: 0.75, px: 1.5}}>
                    <OpenAI size={16}/> {openAICaption}
                </ToggleButton>
            )}
            {supportsAnthropic && (
                <ToggleButton value="anthropic" sx={{textTransform: 'none', gap: 0.75, px: 1.5}}>
                    <Anthropic size={16}/> {t('providerDialog.apiStyle.anthropic')}
                </ToggleButton>
            )}
        </ToggleButtonGroup>
    );

    // Single-protocol-only providers (a one-protocol template, or free-form
    // custom): no topology choice, just the protocol sub-toggle.
    if (!hasMultiOptions) {
        return (
            <FormControl component="fieldset" fullWidth>
                <FormLabel component="legend" sx={{mb: 1}}>
                    {t('providerDialog.apiStyle.label')}
                </FormLabel>
                {bothSupported ? (
                    singleToggle
                ) : (
                    <Typography variant="body2" color="text.secondary">
                        {supportsAnthropic
                            ? t('providerDialog.apiStyle.anthropic')
                            : openAICaption}
                    </Typography>
                )}
            </FormControl>
        );
    }

    const optionRow = (
        optionKind: 'single' | 'fusion' | 'dual',
        title: string,
        desc: string,
        body?: React.ReactNode,
    ) => {
        const selected = kind === optionKind;
        const onClick = () => {
            if (optionKind === 'single') {
                onSelect(single ?? (supportsOpenAI ? 'openai' : 'anthropic'));
            } else if (optionKind === 'fusion') {
                onSelect('fusion');
            } else {
                onSelect('dual');
            }
        };
        return (
            <Box
                onClick={onClick}
                sx={{
                    border: 1,
                    borderColor: selected ? 'primary.main' : 'divider',
                    bgcolor: selected ? 'action.hover' : 'transparent',
                    borderRadius: 1.5,
                    px: 1.5,
                    py: 1.25,
                    cursor: 'pointer',
                    transition: 'all 0.15s',
                    '&:hover': {borderColor: selected ? 'primary.main' : 'text.disabled'},
                }}
            >
                <Stack direction="row" spacing={1} alignItems="flex-start">
                    <Radio size="small" checked={selected} sx={{p: 0, mt: 0.25}}/>
                    <Box sx={{flex: 1}}>
                        <Typography variant="body2" fontWeight={600}>{title}</Typography>
                        <Typography variant="caption" color="text.secondary" sx={{display: 'block', lineHeight: 1.4}}>
                            {desc}
                        </Typography>
                        {selected && body}
                    </Box>
                </Stack>
            </Box>
        );
    };

    return (
        <FormControl component="fieldset" fullWidth>
            <FormLabel component="legend" sx={{mb: 1}}>
                {t('providerDialog.topology.label', {defaultValue: 'Protocol topology'})}
            </FormLabel>
            <Stack spacing={1}>
                {optionRow(
                    'single',
                    t('providerDialog.topology.single.title', {defaultValue: 'Single protocol'}),
                    t('providerDialog.topology.single.desc', {defaultValue: 'This endpoint speaks one API.'}),
                    bothSupported ? singleToggle : undefined,
                )}
                {showFusion && optionRow(
                    'fusion',
                    t('providerDialog.topology.fusion.title', {defaultValue: 'One endpoint, both protocols'}),
                    t('providerDialog.topology.fusion.desc', {
                        defaultValue: 'Same URL serves OpenAI & Anthropic. Stays one provider entry.',
                    }),
                )}
                {showDual && optionRow(
                    'dual',
                    t('providerDialog.topology.dual.title', {defaultValue: 'Two separate endpoints'}),
                    t('providerDialog.topology.dual.desc', {
                        defaultValue: 'Different URL per protocol. Creates two provider entries (shared key).',
                    }),
                )}
            </Stack>
        </FormControl>
    );
};

export default ProtocolTopologySelector;
