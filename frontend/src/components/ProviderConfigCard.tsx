import { ContentCopy as CopyIcon } from '@mui/icons-material';
import InfoIcon from '@mui/icons-material/Info';
import VisibilityIcon from '@mui/icons-material/Visibility';
import {
    Box,
    type BoxProps,
    Divider,
    IconButton,
    Tooltip,
    Typography
} from '@mui/material';
import React, {type ReactNode } from 'react';
import { ApiConfigRow } from './ApiConfigRow';
import { BaseUrlRow } from './BaseUrlRow';
import PluginFeatures from './PluginFeatures';

export interface ConfigSectionProps {
    label: string;
    children: ReactNode;
    infoTooltip?: string;
}

const ConfigSection: React.FC<ConfigSectionProps> = ({ label, children, infoTooltip }) => (
    <Box sx={{ display: 'flex', alignItems: 'center', py: 2, gap: 3 }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 180 }}>
            <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                {label}
            </Typography>
            {infoTooltip && (
                <Tooltip title={infoTooltip} arrow>
                    <InfoIcon sx={{ fontSize: '1rem', color: 'text.secondary', cursor: 'help' }} />
                </Tooltip>
            )}
        </Box>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flex: 1 }}>
            {children}
        </Box>
    </Box>
);

export interface ProviderConfigCardProps {
    /** Card title */
    title: string;
    /** Base URL path (e.g., "/tingly/anthropic") */
    baseUrlPath: string;
    /** Full base URL from getBaseUrl() */
    baseUrl: string;
    /** Copy handler */
    onCopy: (text: string, label: string) => Promise<void>;
    /** API key token */
    token: string;
    /** Handler to show token modal */
    onShowTokenModal: () => void;
    /** Optional: scenario for experimental features */
    scenario?: string;
    /** Optional: mode selection component */
    modeSelection?: ReactNode;
    /** Optional: additional content before experimental features */
    extraContent?: ReactNode;
    /** Optional: show API key row */
    showApiKeyRow?: boolean;
    showBaseUrlRow?: boolean;
    /** Optional: info tooltip for base URL in title */
    titleInfoTooltip?: string;
    /** Optional: custom label for base URL row */
    baseUrlLabel?: string;
    /** Container props */
    containerProps?: BoxProps;
}

/**
 * Unified provider configuration card component.
 * Provides a consistent layout for SDK configuration across all provider pages.
 *
 * Structure:
 * 1. Base URL row (always shown)
 * 2. API Key row (optional, shown by default)
 * 3. Divider
 * 4. Mode selection (optional, e.g., ClaudeCode)
 * 5. Experimental features (optional, when scenario is provided)
 */
export const ProviderConfigCard: React.FC<ProviderConfigCardProps> = ({
    title,
    baseUrlPath,
    baseUrl,
    onCopy,
    token,
    onShowTokenModal,
    scenario,
    modeSelection,
    extraContent,
    showApiKeyRow = true,
    showBaseUrlRow = true,
    titleInfoTooltip,
    baseUrlLabel = 'Base URL',
    containerProps,
}) => {
    const showOptionalSections = scenario || modeSelection || extraContent;
    const hasDivider = showApiKeyRow && showOptionalSections;

    return (
        <Box {...containerProps}>
            {/* Base URL Row - Always shown */}
            {showBaseUrlRow && <Box sx={{ p: 2, pb: showApiKeyRow ? 2 : 0 }}>
                <BaseUrlRow
                    label={baseUrlLabel}
                    path={baseUrlPath}
                    baseUrl={baseUrl}
                    onCopy={(url) => onCopy(url, title)}
                    urlLabel={`${title} Base URL`}
                />
            </Box>}

            {/* API Key Row - Optional but shown by default */}
            {showApiKeyRow && (
                <Box sx={{ px: 2, pb: hasDivider ? 2 : 2 }}>
                    <ApiConfigRow label="API Key" value={token}>
                        <Box sx={{ display: 'flex', gap: 0.5, ml: 'auto' }}>
                            <Tooltip title="View Token">
                                <IconButton onClick={onShowTokenModal} size="small">
                                    <VisibilityIcon fontSize="small" />
                                </IconButton>
                            </Tooltip>
                            <Tooltip title="Copy Token">
                                <IconButton onClick={() => onCopy(token, 'API Key')} size="small">
                                    <CopyIcon fontSize="small" />
                                </IconButton>
                            </Tooltip>
                        </Box>
                    </ApiConfigRow>
                </Box>
            )}

            {/* Divider - Between core config and optional sections */}
            {hasDivider && (
                <Divider sx={{ mx: 2 }} />
            )}

            {/* Mode Selection - Optional (e.g., ClaudeCode unified/separate) */}
            {modeSelection && (
                <Box sx={{ px: 2 }}>
                    {modeSelection}
                </Box>
            )}

            {/* Extra Content - Optional */}
            {extraContent && (
                <Box sx={{ px: 2 }}>
                    {extraContent}
                </Box>
            )}

            {/* Scenario Features (Thinking Effort + Plugin) - Optional (when scenario is provided) */}
            {scenario && (
                <Box sx={{ px: 2 }}>
                    <PluginFeatures scenario={scenario} />
                </Box>
            )}
        </Box>
    );
};

export default ProviderConfigCard;
