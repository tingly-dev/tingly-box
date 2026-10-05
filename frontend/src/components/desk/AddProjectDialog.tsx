import {Button, Dialog, DialogActions, DialogContent, DialogContentText, DialogTitle, TextField} from '@mui/material';
import {useState} from 'react';
import {useTranslation} from 'react-i18next';
import {isAbsoluteProjectPath, normalizeProjectPath} from './useDeskProjects';

interface AddProjectDialogProps {
    onClose: () => void;
    onAdd: (path: string) => boolean;
}

const AddProjectDialog = ({onClose, onAdd}: AddProjectDialogProps) => {
    const {t} = useTranslation();
    const [value, setValue] = useState('');
    const [error, setError] = useState(false);
    const path = normalizeProjectPath(value);
    const invalid = value.trim() !== '' && !isAbsoluteProjectPath(path);

    return (
        <Dialog open onClose={onClose} maxWidth="sm" fullWidth aria-labelledby="desk-add-project-title">
            <form onSubmit={(event) => {
                event.preventDefault();
                if (!path || invalid) return;
                setError(!onAdd(path));
            }}>
                <DialogTitle id="desk-add-project-title">{t('desk.addProject', {defaultValue: 'Add project'})}</DialogTitle>
                <DialogContent>
                    <DialogContentText sx={{mb: 2}}>
                        {t('desk.addProjectDescription', {defaultValue: 'Enter an existing project directory on the machine running tingly-box. Tasks will run in this directory.'})}
                    </DialogContentText>
                    <TextField
                        autoFocus fullWidth label={t('desk.projectDirectory', {defaultValue: 'Project directory'})}
                        placeholder={t('desk.projectPathExample', {defaultValue: '/path/to/project or C:\\projects\\app'})}
                        value={value} onChange={(event) => {setValue(event.target.value); setError(false);}}
                        error={invalid || error}
                        helperText={invalid ? t('desk.projectPathAbsolute', {defaultValue: 'Enter a full absolute path, such as /home/me/projects/app.'})
                            : error ? t('desk.projectSaveFailed', {defaultValue: 'Could not save the project in this browser. Please try again.'})
                                : t('desk.projectStorageHint', {defaultValue: 'Saved in this browser. The directory is checked when you start a task.'})}
                        slotProps={{htmlInput: {style: {fontFamily: 'monospace'}}}}
                    />
                </DialogContent>
                <DialogActions sx={{px: 3, pb: 2}}>
                    <Button onClick={onClose}>{t('common.cancel', {defaultValue: 'Cancel'})}</Button>
                    <Button type="submit" variant="contained" disabled={!path || invalid}>{t('desk.addProject', {defaultValue: 'Add project'})}</Button>
                </DialogActions>
            </form>
        </Dialog>
    );
};

export default AddProjectDialog;
