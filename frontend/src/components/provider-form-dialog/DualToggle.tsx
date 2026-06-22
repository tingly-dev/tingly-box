import {InfoOutlined} from '@/components/icons';
import {Box, Checkbox, FormControlLabel, Stack, Tooltip, Typography} from '@mui/material';
import React from 'react';
import {useTranslation} from 'react-i18next';

interface DualToggleProps {
    checked: boolean;
    onChange: (checked: boolean) => void;
}

const DualToggle: React.FC<DualToggleProps> = ({checked, onChange}) => {
    const {t} = useTranslation();

    return (
        <Box sx={{display: 'flex', justifyContent: 'flex-end', mt: -0.5, pr: 2}}>
            <FormControlLabel
                control={
                    <Checkbox
                        size="small"
                        checked={checked}
                        onChange={(e) => onChange(e.target.checked)}
                    />
                }
                label={(
                    <Stack direction="row" spacing={0.75} alignItems="center">
                        <Typography variant="body2">
                            {t('providerDialog.dual.modeLabel')}
                        </Typography>
                        <Tooltip
                            arrow
                            placement="top"
                            slotProps={{
                                tooltip: {
                                    sx: (theme) => ({
                                        maxWidth: 360,
                                        bgcolor: 'background.paper',
                                        color: 'text.primary',
                                        border: `1px solid ${theme.palette.divider}`,
                                        boxShadow: theme.shadows[6],
                                        p: 1.25,
                                        '& .MuiTypography-caption': {
                                            color: 'text.secondary',
                                            lineHeight: 1.45,
                                        },
                                    }),
                                },
                                arrow: {
                                    sx: (theme) => ({
                                        color: theme.palette.background.paper,
                                        '&:before': {
                                            border: `1px solid ${theme.palette.divider}`,
                                        },
                                    }),
                                },
                            }}
                            title={
                                <Box>
                                    <Typography variant="body2" sx={{fontWeight: 600, mb: 0.5}}>
                                        {t('providerDialog.dual.tooltipTitle')}
                                    </Typography>
                                    <Typography variant="caption" sx={{display: 'block'}}>
                                        {t('providerDialog.dual.normalModeDesc')}
                                    </Typography>
                                    <Typography variant="caption" sx={{display: 'block', mt: 0.75}}>
                                        {t('providerDialog.dual.dualModeDesc')}
                                    </Typography>
                                </Box>
                            }
                        >
                            <InfoOutlined sx={{fontSize: 16, color: 'text.secondary'}} />
                        </Tooltip>
                    </Stack>
                )}
                labelPlacement="start"
            />
        </Box>
    );
};

export default DualToggle;
