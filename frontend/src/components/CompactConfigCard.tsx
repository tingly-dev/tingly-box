import VisibilityIcon from '@mui/icons-material/Visibility';
import { IconButton, Tooltip, Typography } from '@mui/material';
import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ConfigRow, type TabKey } from './ConfigRow';
import { EnvironmentModeSwitcher, type EnvironmentMode } from './EnvironmentModeSwitcher';
import { useScenarioPageModal } from '@/pages/scenario/context/ScenarioPageContext';

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
// Helpers
// ============================================================================

const transformUrlByMode = (url: string, mode: EnvironmentMode): string => {
    switch (mode) {
        case 'docker':
            return url.replace(/\/\/([^/:]+)(?::(\d+))?/, '//host.docker.internal:$2');
        case 'cli':
        case 'npx':
        case 'wsl':
            // Future: implement specific transformations
            return url;
        case 'local':
        default:
            return url;
    }
};

const maskToken = (token: string): string => {
    if (token.length <= 16) return token;
    const start = token.slice(0, 12);
    const end = token.slice(-12);
    return `${start}${'*'.repeat(8)}${end}`;
};

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
                <Typography
                    variant="subtitle2"
                    onClick={() => onCopy(fullUrl, title || baseUrlLabel)}
                    sx={{
                        fontFamily: 'monospace',
                        fontSize: '0.75rem',
                        color: 'primary.main',
                        cursor: 'pointer',
                        '&:hover': {
                            textDecoration: 'underline',
                            backgroundColor: 'action.hover',
                        },
                        padding: 1,
                        borderRadius: 1,
                        transition: 'all 0.2s ease-in-out',
                    }}
                    title={`Click to copy ${baseUrlLabel}: ${fullUrl}`}
                >
                    {fullUrl}
                </Typography>
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
                <Typography
                    variant="subtitle2"
                    onClick={() => onCopy(token, apiKeyLabel)}
                    sx={{
                        fontFamily: 'monospace',
                        fontSize: '0.75rem',
                        color: 'primary.main',
                        cursor: 'pointer',
                        '&:hover': {
                            textDecoration: 'underline',
                            backgroundColor: 'action.hover',
                        },
                        padding: 1,
                        borderRadius: 1,
                        transition: 'all 0.2s ease-in-out',
                    }}
                    title={`Click to copy ${apiKeyLabel}`}
                >
                    {maskToken(token)}
                </Typography>
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
