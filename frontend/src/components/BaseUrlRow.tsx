import { ContentCopy as CopyIcon } from '@mui/icons-material';
import { Box, IconButton, Tooltip, Typography } from '@mui/material';
import DockerOriginal from "devicons-react/icons/DockerOriginal";
import React from "react";
import { useTranslation } from 'react-i18next';

const DockerIcon = DockerOriginal

interface BaseUrlRowProps {
    label: string;
    path: string;
    baseUrl: string;
    onCopy: (url: string, label: string) => void;
    urlLabel?: string;
}

const toDockerUrl = (url: string): string => {
    // Replace host with host.docker.internal
    // Handles: http://localhost:8080/path or http://192.168.1.1:8080/path
    return url.replace(/\/\/([^/:]+)(?::(\d+))?/, '//host.docker.internal:$2');
};

export const BaseUrlRow: React.FC<BaseUrlRowProps> = ({
    label,
    path,
    baseUrl,
    onCopy,
    urlLabel
}) => {
    const { t } = useTranslation();
    const [isDockerMode, setIsDockerMode] = React.useState(false);

    // Build full URL from baseUrl and path
    const fullUrl = React.useMemo(() => {
        const url = `${baseUrl}${path}`;
        return isDockerMode ? toDockerUrl(url) : url;
    }, [baseUrl, path, isDockerMode]);

    const displayUrl = urlLabel || label;

    return (
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1, maxWidth: 700 }}>
            <Typography
                variant="body2"
                color="text.secondary"
                sx={{
                    minWidth: 120,
                    flexShrink: 0,
                    fontWeight: 500
                }}
            >
                {label}:
            </Typography>
            <Typography
                variant="body2"
                onClick={() => onCopy(fullUrl, displayUrl)}
                sx={{
                    fontFamily: 'monospace',
                    fontSize: '0.75rem',
                    color: 'primary.main',
                    flex: 1,
                    minWidth: 0,
                    cursor: 'pointer',
                    '&:hover': {
                        textDecoration: 'underline',
                        backgroundColor: 'action.hover'
                    },
                    padding: 1,
                    borderRadius: 1,
                    transition: 'all 0.2s ease-in-out'
                }}
                title={`Click to copy ${displayUrl}`}
            >
                {fullUrl}
            </Typography>
            <Box sx={{ display: 'flex', gap: 0.5, ml: 'auto' }}>

                <Tooltip title={t('serverInfo.docker.tooltip')}>
                    <IconButton
                        onClick={() => setIsDockerMode(!isDockerMode)}
                        size="small"
                        color={isDockerMode ? "primary" : "default"}
                        sx={{ position: 'relative' }}
                    >
                        <DockerIcon size='25' color="blue" />
                        {isDockerMode && (
                            <Box
                                sx={{
                                    position: 'absolute',
                                    bottom: -1,
                                    right: -1,
                                    width: 14,
                                    height: 14,
                                    borderRadius: '50%',
                                    backgroundColor: 'success.main',
                                    display: 'flex',
                                    alignItems: 'center',
                                    justifyContent: 'center',
                                    border: '1.5px solid',
                                    borderColor: 'background.paper',
                                }}
                            >
                                <Typography
                                    sx={{
                                        fontSize: '10px',
                                        lineHeight: 1,
                                        color: 'background.paper',
                                        fontWeight: 'bold',
                                    }}
                                >
                                    ✓
                                </Typography>
                            </Box>
                        )}
                    </IconButton>
                </Tooltip>
                <Tooltip title="Copy Base URL">
                    <IconButton onClick={() => onCopy(fullUrl, displayUrl)} size="small">
                        <CopyIcon fontSize="small" />
                    </IconButton>
                </Tooltip>
            </Box>
        </Box>
    );
};

export default BaseUrlRow;
