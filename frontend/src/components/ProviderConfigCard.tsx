import { ContentCopy as CopyIcon } from '@/components/icons';
import { Info as InfoIcon } from '@/components/icons';
import { Visibility as VisibilityIcon } from '@/components/icons';
import {
    Box,
    type BoxProps,
    Divider,
    IconButton,
    Tooltip,
    Typography
} from '@mui/material';
import React, { type ReactNode, useState } from 'react';
import { ConfigRow } from './ConfigRow';
import { CompactConfigCard } from './CompactConfigCard';
import PluginFeatures from './PluginFeatures';
import { EnvironmentModeSwitcher, type EnvironmentMode } from './EnvironmentModeSwitcher';
import { useScenarioPageModal } from '@/pages/scenario/context/ScenarioPageContext';
import { copyableTextStyle } from '@/styles/textStyles';
import { maskToken, transformUrlByMode } from '@/utils/tokenUtils';

export interface ProviderConfigCardProps {
    /** Card title */
    title: string;
    /** Base URL path (e.g., "/tingly/anthropic") */
    baseUrlPath: string;
    /** Full base URL from getBaseUrl() */
    baseUrl: string;
    /** Copy handler */
    onCopy: (text: string, label: string) => Promise<void>;
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
    /** Optional: use compact horizontal tab mode */
    compact?: boolean;
    /** Container props */
    containerProps?: BoxProps;
    /** Controlled Local / Docker mode (the page also shows the URL elsewhere); omit to keep it local. */
    envMode?: EnvironmentMode;
    onEnvModeChange?: (mode: EnvironmentMode) => void;
    /** Show the scenario's plugin row (default); a page that places it itself passes false. */
    showPlugins?: boolean;
}

/**
 * Unified provider configuration card component.
 * Provides a consistent layout for SDK configuration across all provider pages.
 *
 * Modal state (token, showTokenModal, setShowTokenModal) is obtained from
 * ScenarioPageModalProvider context, so the parent page doesn't need to pass them.
 *
 * Modes:
 * - Compact: Horizontal tab switching (Base URL | API Key) - saves vertical space
 * - Standard: Vertical rows with Base URL and API Key sections
 *
 * Standard Structure:
 * 1. Base URL row (when showBaseUrlRow=true)
 * 2. API Key row (when showApiKeyRow=true)
 * 3. Divider
 * 4. Mode selection (optional, e.g., ClaudeCode)
 * 5. Experimental features (optional, when scenario is provided)
 */
export const ProviderConfigCard: React.FC<ProviderConfigCardProps> = ({
    title,
    baseUrlPath,
    baseUrl,
    onCopy,
    scenario,
    modeSelection,
    extraContent,
    showApiKeyRow = true,
    showBaseUrlRow = true,
    baseUrlLabel = 'Base URL',
    compact = false,
    containerProps,
    envMode: envModeProp,
    onEnvModeChange,
    showPlugins = true,
}) => {
    // Get modal state from context instead of props
    const { token, setShowTokenModal } = useScenarioPageModal();
    const showOptionalSections = (showPlugins && scenario) || modeSelection || extraContent;
    const hasDivider = showApiKeyRow && showOptionalSections;

    // Compact mode: single horizontal layout with tab switching
    if (compact) {
        return (
            <Box {...containerProps}>
                <Box sx={{ px: 2, py: 0.5 }}>
                    <CompactConfigCard
                        baseUrlPath={baseUrlPath}
                        baseUrl={baseUrl}
                        onCopy={onCopy}
                        baseUrlLabel={baseUrlLabel}
                        title={title}
                        envMode={envModeProp}
                        onEnvModeChange={onEnvModeChange}
                        showApiKeyRow={showApiKeyRow}
                    />
                </Box>

                {/* Mode Selection - Optional */}
                {modeSelection && (
                    <Box sx={{ px: 2, pt: 0.5 }}>
                        {modeSelection}
                    </Box>
                )}

                {/* Extra Content - Optional */}
                {extraContent && (
                    <Box sx={{ px: 2, pt: 0.5 }}>
                        {extraContent}
                    </Box>
                )}

                {/* Scenario Features - Optional */}
                {showPlugins && scenario && (
                    <Box sx={{ px: 2, pt: 0.5 }}>
                        <PluginFeatures scenario={scenario} />
                    </Box>
                )}
            </Box>
        );
    }

    // Standard mode: two separate rows using ConfigRow (single tab each)
    const [localEnvMode, setLocalEnvMode] = useState<EnvironmentMode>('local');
    const envMode = envModeProp ?? localEnvMode;
    const setEnvMode = onEnvModeChange ?? setLocalEnvMode;

    // Build full URL based on environment mode
    const fullUrl = React.useMemo(() => {
        const url = `${baseUrl}${baseUrlPath}`;
        return transformUrlByMode(url, envMode);
    }, [baseUrl, baseUrlPath, envMode]);

    return (
        <Box {...containerProps}>
            {/* Base URL Row - single tab mode */}
            {showBaseUrlRow && (
                <Box sx={{ px: 2, py: 0.5 }}>
                    <ConfigRow
                        tabs={[
                            {
                                key: 'baseUrl',
                                label: baseUrlLabel,
                                content: (
                                    <Tooltip title={`Click to copy ${baseUrlLabel}`} arrow placement="top">
                                        <Typography
                                            variant="subtitle2"
                                            onClick={() => onCopy(fullUrl, `${title} ${baseUrlLabel}`)}
                                            sx={copyableTextStyle}
                                        >
                                            {fullUrl}
                                        </Typography>
                                    </Tooltip>
                                ),
                                actions: (
                                    <>
                                        <Tooltip title={`Copy ${baseUrlLabel}`} arrow>
                                            <IconButton onClick={() => onCopy(fullUrl, `${title} ${baseUrlLabel}`)} size="small">
                                                <CopyIcon fontSize="small" />
                                            </IconButton>
                                        </Tooltip>
                                        <EnvironmentModeSwitcher
                                            value={envMode}
                                            onChange={setEnvMode}
                                        />
                                    </>
                                ),
                            },
                        ]}
                        activeTab="baseUrl"
                        onTabChange={() => {}}
                    />
                </Box>
            )}

            {/* API Key Row - single tab mode */}
            {showApiKeyRow && (
                <Box sx={{ px: 2, py: 0.5 }}>
                    <ConfigRow
                        tabs={[
                            {
                                key: 'apiKey',
                                label: 'API Key',
                                content: (
                                    <Tooltip title="Click to copy API Key" arrow placement="top">
                                        <Typography
                                            variant="subtitle2"
                                            onClick={() => onCopy(token, 'API Key')}
                                            sx={copyableTextStyle}
                                        >
                                            {maskToken(token)}
                                        </Typography>
                                    </Tooltip>
                                ),
                                actions: (
                                    <>
                                        <Tooltip title="Copy API Key" arrow>
                                            <IconButton onClick={() => onCopy(token, 'API Key')} size="small">
                                                <CopyIcon fontSize="small" />
                                            </IconButton>
                                        </Tooltip>
                                        <Tooltip title="View Full Token">
                                            <IconButton onClick={() => setShowTokenModal(true)} size="small">
                                                <VisibilityIcon fontSize="small" />
                                            </IconButton>
                                        </Tooltip>
                                    </>
                                ),
                            },
                        ]}
                        activeTab="apiKey"
                        onTabChange={() => {}}
                    />
                </Box>
            )}

            {/* Mode Selection - Optional (e.g., ClaudeCode unified/separate) */}
            {modeSelection && (
                <Box sx={{ px: 2, py: 0.5 }}>
                    {modeSelection}
                </Box>
            )}

            {/* Extra Content - Optional */}
            {extraContent && (
                <Box sx={{ px: 2, py: 0.5 }}>
                    {extraContent}
                </Box>
            )}

            {/* Scenario Plugins (Thinking Effort + scenario-level plugins) - Optional (when scenario is provided) */}
            {showPlugins && scenario && (
                <Box sx={{ px: 2, py: 0.5 }}>
                    <PluginFeatures scenario={scenario} />
                </Box>
            )}
        </Box>
    );
};

export default ProviderConfigCard;
