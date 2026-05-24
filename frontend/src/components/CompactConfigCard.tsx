import { Visibility as VisibilityIcon } from '@/components/icons';
import { IconButton, Tooltip, Typography } from '@mui/material';
import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ConfigRow, type TabKey } from './ConfigRow';
import { EnvironmentModeSwitcher, type EnvironmentMode } from './EnvironmentModeSwitcher';
import { useScenarioPageModal } from '@/pages/scenario/context/ScenarioPageContext';
import { copyableTextStyle } from '@/styles/textStyles';
import { maskToken, transformUrlByMode } from '@/utils/tokenUtils';

// ============================================================================
// Types
// ============================================================================

export type ConfigTabType = 'baseUrl' | 'apiKey';

interface CompactConfigCardProps {
    /** Base URL path (e.g., "/tingly/anthropic") */
    baseUrlPath: string;
    /** Full base URL from getBaseUrl() */
    baseUrl: string;
    /** Copy handler */
    onCopy: (text: string, label: string) => Promise<void>;
    /** Optional: custom label for base URL */
    baseUrlLabel?: string;
    /** Optional: custom label for API key */
    apiKeyLabel?: string;
    /** Optional: card title */
    title?: string;
    /** Optional: available environment modes (defaults to local + docker) */
    environmentModes?: EnvironmentMode[];
}

// ============================================================================
// Component
// ============================================================================

/**
 * Compact configuration card with horizontal text tab switching.
 * Uses ConfigRow component internally.
 *
 * Modal state (token, showTokenModal, setShowTokenModal) is obtained from
 * ScenarioPageModalProvider context.
 *
 * Layout:
 * ┌─────────────────────────────────────────────────────────────────┐
 * │ Base URL | API Key    http://localhost:8080/tingly/anthropic  │
 * │                       [💻][🐳]                                 │
 * └─────────────────────────────────────────────────────────────────┘
 *
 * Features:
 * - Click URL/token to copy (no separate copy button)
 * - Environment mode switcher (Local/Docker/etc) for URL transformation
 * - View token modal for API Key tab
 */
export const CompactConfigCard: React.FC<CompactConfigCardProps> = ({
    baseUrlPath,
    baseUrl,
    onCopy,
    baseUrlLabel = 'Base URL',
    apiKeyLabel = 'API Key',
    title,
    environmentModes,
}) => {
    const { t } = useTranslation();
    // Get modal state from context
    const { token, setShowTokenModal } = useScenarioPageModal();
    const [activeTab, setActiveTab] = useState<TabKey>('baseUrl');
    const [envMode, setEnvMode] = useState<EnvironmentMode>('local');

    // Build full URL based on environment mode
    const fullUrl = React.useMemo(() => {
        const url = `${baseUrl}${baseUrlPath}`;
        return transformUrlByMode(url, envMode);
    }, [baseUrl, baseUrlPath, envMode]);

    // Define tabs
    const tabs = [
        {
            key: 'baseUrl',
            label: 'Base URL',
            content: (
                <Tooltip title={`Click to copy ${baseUrlLabel}`} arrow placement="top">
                    <Typography
                        variant="subtitle2"
                        onClick={() => onCopy(fullUrl, title || baseUrlLabel)}
                        sx={copyableTextStyle}
                    >
                        {fullUrl}
                    </Typography>
                </Tooltip>
            ),
            actions: (
                <EnvironmentModeSwitcher
                    value={envMode}
                    onChange={setEnvMode}
                    modes={environmentModes}
                />
            ),
        },
        {
            key: 'apiKey',
            label: 'API Key',
            content: (
                <Tooltip title={`Click to copy ${apiKeyLabel}`} arrow placement="top">
                    <Typography
                        variant="subtitle2"
                        onClick={() => onCopy(token, apiKeyLabel)}
                        sx={copyableTextStyle}
                    >
                        {maskToken(token)}
                    </Typography>
                </Tooltip>
            ),
            actions: (
                <Tooltip title="View Full Token">
                    <IconButton onClick={() => setShowTokenModal(true)} size="small">
                        <VisibilityIcon fontSize="small" />
                    </IconButton>
                </Tooltip>
            ),
        },
    ];

    return (
        <ConfigRow
            tabs={tabs}
            activeTab={activeTab}
            onTabChange={setActiveTab}
        />
    );
};

export default CompactConfigCard;
