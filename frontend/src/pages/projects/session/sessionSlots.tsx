// Tingly Box's parts of a folder session in @tingly/projects: the Claude
// Code profile and model tier a session launches with, and the status line
// that says where its requests go. Projects knows a profile only as a name.
// A tier belongs to its profile, so picking another profile starts from the
// profile's default.
import type {SessionSlots} from '@tingly/projects';
import ModelSelect from '@/components/claudeCode/ModelSelect';
import ProfileSelect from '@/components/claudeCode/ProfileSelect';
import SessionStatusLine from './SessionStatusLine';

export const sessionSlots: SessionSlots = {
    launchPickers: ({value, onChange, disabled}) => (
        <>
            <ProfileSelect disabled={disabled} value={value.profile} onChange={(profile) => onChange({profile, model: ''})}/>
            <ModelSelect disabled={disabled} profile={value.profile} value={value.model} onChange={(model) => onChange({...value, model})}/>
        </>
    ),
    statusLine: ({session, events}) => <SessionStatusLine session={session} events={events}/>,
};
