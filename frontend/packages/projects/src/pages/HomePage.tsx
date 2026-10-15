import { Box, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Composer } from '../components/Composer';
import { useChosenModel, useStartChat } from '../hooks/useStartChat';

function greetingKey(): string {
    const h = new Date().getHours();
    if (h < 5) return 'home.greetingNight';
    if (h < 12) return 'home.greetingMorning';
    if (h < 18) return 'home.greetingAfternoon';
    return 'home.greetingEvening';
}

/** New standalone chat. Ongoing work belongs in a project. */
export default function HomePage() {
    const { t } = useTranslation();
    const start = useStartChat();
    const { model, setModel } = useChosenModel();

    return (
        <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', justifyContent: 'center', px: 2, pb: '12vh', overflowY: 'auto' }}>
            <Box sx={{ width: '100%', maxWidth: 720, mx: 'auto' }}>
                <Typography variant="h1" sx={{ textAlign: 'center', mb: 4, fontSize: { xs: '1.8rem', md: '2.4rem' } }}>
                    {t(greetingKey())}
                </Typography>
                <Composer autoFocus minRows={3} model={model} onModelChange={setModel} placeholder={t('home.placeholder')} onSend={(text) => start({ text, model })} />
                <Typography variant="caption" color="text.secondary" sx={{ display: 'block', textAlign: 'center', mt: 1.5 }}>
                    {t('home.projectHint')}
                </Typography>
            </Box>
        </Box>
    );
}
