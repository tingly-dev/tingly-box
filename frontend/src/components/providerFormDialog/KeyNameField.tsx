import {Edit} from '@/components/icons';
import {Box, IconButton, TextField, Tooltip, Typography} from '@mui/material';
import React from 'react';
import {useTranslation} from 'react-i18next';

interface KeyNameFieldProps {
    showField: boolean;
    onShowField: () => void;
    name: string;
    autoName: string;
    onNameChange: (value: string) => void;
}

const KeyNameField: React.FC<KeyNameFieldProps> = ({
    showField,
    onShowField,
    name,
    autoName,
    onNameChange,
}) => {
    const {t} = useTranslation();

    if (showField) {
        return (
            <TextField
                size="small"
                fullWidth
                autoFocus
                label={t('providerDialog.keyName.label')}
                value={name}
                onChange={(e) => onNameChange(e.target.value)}
                placeholder={autoName}
                helperText={t('providerDialog.keyName.helper', {
                    defaultValue: 'Leave blank to use the auto-generated name. You can rename later.',
                })}
            />
        );
    }

    return (
        <Box
            sx={{
                display: 'flex',
                alignItems: 'center',
                gap: 1,
                px: 1.5,
                py: 0.75,
                borderRadius: 1,
                bgcolor: 'background.default',
                border: 1,
                borderColor: 'divider',
            }}
        >
            <Typography variant="caption" color="text.secondary">
                {t('providerDialog.keyName.label')}
            </Typography>
            <Typography
                variant="body2"
                sx={{
                    flex: 1,
                    color: 'text.primary',
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                }}
            >
                {name || autoName}
            </Typography>
            <Tooltip
                title={t('providerDialog.keyName.editAction', {defaultValue: 'Edit name'})}
                arrow
            >
                <IconButton
                    size="small"
                    onClick={onShowField}
                    sx={{color: 'text.secondary'}}
                >
                    <Edit fontSize="small" />
                </IconButton>
            </Tooltip>
        </Box>
    );
};

export default KeyNameField;
