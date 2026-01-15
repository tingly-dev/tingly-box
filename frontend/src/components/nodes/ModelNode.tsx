import {
    Box,
    TextField,
    Typography
} from '@mui/material';
import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { StyledModelNode, modelNode } from './styles.tsx';

// Model Node Component with editing support
export interface ModelNodeProps {
    active: boolean;
    label: string;
    value: string;
    editable?: boolean;
    onUpdate?: (value: string) => void;
    showStatusIcon?: boolean;
    compact?: boolean;
}

export const ModelNode: React.FC<ModelNodeProps> = ({
    active,
    label,
    value,
    editable = false,
    onUpdate,
    showStatusIcon = true,
    compact = false
}) => {
    const { t } = useTranslation();
    const [editMode, setEditMode] = useState(false);
    const [tempValue, setTempValue] = useState(value);

    React.useEffect(() => {
        setTempValue(value);
    }, [value]);

    const handleSave = () => {
        if (onUpdate && tempValue.trim()) {
            onUpdate(tempValue.trim());
        }
        setEditMode(false);
    };

    const handleCancel = () => {
        setTempValue(value);
        setEditMode(false);
    };

    const handleKeyDown = (e: React.KeyboardEvent) => {
        if (e.key === 'Enter') {
            handleSave();
        } else if (e.key === 'Escape') {
            handleCancel();
        }
    };

    return (
        <StyledModelNode compact={compact}>
            {editMode && editable ? (
                <TextField
                    value={tempValue}
                    onChange={(e) => setTempValue(e.target.value)}
                    onBlur={handleSave}
                    onKeyDown={handleKeyDown}
                    size="small"
                    fullWidth
                    autoFocus
                    label={t('rule.card.unspecifiedModel')}
                    sx={{
                        '& .MuiInputBase-input': {
                            color: 'text.primary',
                            fontWeight: 'inherit',
                            fontSize: 'inherit',
                            backgroundColor: 'transparent',
                        },
                        '& .MuiOutlinedInput-notchedOutline': {
                            borderColor: 'primary.main',
                        },
                        '& .MuiOutlinedInput-root:hover .MuiOutlinedInput-notchedOutline': {
                            borderColor: 'primary.dark',
                        },
                    }}
                />
            ) : (
                <Box
                    onClick={() => editable && setEditMode(true)}
                    sx={{
                        cursor: editable ? 'pointer' : 'default',
                        width: '100%',
                        height: '100%',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        '&:hover': editable ? {
                            backgroundColor: 'action.hover',
                            borderRadius: 1,
                        } : {},
                    }}
                >
                    <Typography variant="body2" sx={{ fontWeight: 600, color: 'text.primary', fontSize: '0.9rem' }}>
                        {value || label}
                    </Typography>
                </Box>
            )}
        </StyledModelNode>
    );
};
