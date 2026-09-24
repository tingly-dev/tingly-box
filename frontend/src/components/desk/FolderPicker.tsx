import type {RecentFolder} from '@/services/deskApi';
import {Autocomplete, Box, TextField, Typography} from '@mui/material';
import {useTranslation} from 'react-i18next';

interface FolderPickerProps {
    value: string;
    onChange: (path: string) => void;
    recentFolders: RecentFolder[];
}

// FolderPicker is one field, not a wizard (ux-principles #2): type a path
// directly, or pick a recently-used one. There is deliberately no
// server-side directory browser here — that would mean an API that lists
// arbitrary filesystem paths on request, which is more surface than this
// first landing needs; typing a path or reusing one already used is enough
// to get started.
const FolderPicker = ({value, onChange, recentFolders}: FolderPickerProps) => {
    const {t} = useTranslation();

    return (
        <Autocomplete
            freeSolo
            fullWidth
            size="small"
            options={recentFolders.map((f) => f.path)}
            inputValue={value}
            onInputChange={(_e, newValue) => onChange(newValue)}
            renderOption={(props, option) => {
                const folder = recentFolders.find((f) => f.path === option);
                return (
                    <Box component="li" {...props} key={option}>
                        <Box>
                            <Typography variant="body2">{folder ? folder.name : option}</Typography>
                            <Typography variant="caption" color="text.secondary">{option}</Typography>
                        </Box>
                    </Box>
                );
            }}
            renderInput={(params) => (
                <TextField
                    {...params}
                    label={t('desk.folderPath', {defaultValue: 'Folder'})}
                    placeholder="/path/to/project"
                    slotProps={{
                        ...params.slotProps,
                        htmlInput: {...params.slotProps.htmlInput, style: {fontFamily: 'monospace'}},
                    }}
                />
            )}
        />
    );
};

export default FolderPicker;
