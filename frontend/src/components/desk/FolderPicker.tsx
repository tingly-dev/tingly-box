import {FolderOpen} from '@/components/icons';
import type {RecentFolder} from '@/services/deskApi';
import {Autocomplete, Box, InputAdornment, TextField, Typography} from '@mui/material';
import {useTranslation} from 'react-i18next';

interface FolderPickerProps {
    value: string;
    onChange: (path: string) => void;
    recentFolders: RecentFolder[];
}

// FolderPicker is one field, not a wizard (ux-principles #2): type a path
// directly, or pick a recently-used one. There is deliberately no
// server-side directory browser — that would mean an API that lists
// arbitrary filesystem paths on request (see .design/desk.md §6).
const FolderPicker = ({value, onChange, recentFolders}: FolderPickerProps) => {
    const {t} = useTranslation();

    return (
        <Autocomplete
            freeSolo
            size="small"
            options={recentFolders.map((f) => f.path)}
            inputValue={value}
            onInputChange={(_e, newValue) => onChange(newValue)}
            sx={{flex: 1, minWidth: 240}}
            renderOption={(props, option) => {
                const folder = recentFolders.find((f) => f.path === option);
                return (
                    <Box component="li" {...props} key={option}>
                        <Box>
                            <Typography variant="body2">{folder ? folder.name : option}</Typography>
                            <Typography variant="caption" color="text.secondary" sx={{fontFamily: 'monospace'}}>{option}</Typography>
                        </Box>
                    </Box>
                );
            }}
            renderInput={(params) => (
                <TextField
                    {...params}
                    variant="standard"
                    placeholder={t('desk.folderPlaceholder', {defaultValue: '/path/to/project'})}
                    slotProps={{
                        ...params.slotProps,
                        input: {
                            ...params.slotProps.input,
                            disableUnderline: true,
                            startAdornment: (
                                <InputAdornment position="start">
                                    <FolderOpen sx={{fontSize: 16}}/>
                                </InputAdornment>
                            ),
                        },
                        htmlInput: {...params.slotProps.htmlInput, style: {fontFamily: 'monospace', fontSize: '0.8rem'}},
                    }}
                />
            )}
        />
    );
};

export default FolderPicker;
