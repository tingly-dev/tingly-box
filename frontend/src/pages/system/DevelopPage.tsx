import CardGrid from '@/components/CardGrid';
import { PageLayout } from '@tingly/ui/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { Button, Stack, Typography } from '@mui/material';
import { useNotify } from '@/hooks/useNotify';

// Card spans full width; only the content inside is capped so controls don't
// stretch uncomfortably wide on big screens (same rhythm as System pages).
const CONTENT_MAX_WIDTH = 720;

// A realistic failure — the kind users hit and need to copy for a report.
const ERROR_DEMO_TITLE = 'Delete API key failed';
const ERROR_DEMO_MESSAGE =
    'Failed to delete API key sk-proj-****7f2a: upstream returned 502 Bad Gateway (provider: OpenAI, latency: 740ms)';

const DevelopPage = () => {
    const notify = useNotify();

    return (
        <PageLayout loading={false}>
            <CardGrid>
                <UnifiedCard
                    title="Develop"
                    titleHeadingLevel={1}
                    size="full"
                    contentMaxWidth={CONTENT_MAX_WIDTH}
                >
                    <Stack spacing={2}>
                        <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                            Developer utilities for verifying frontend behaviour by hand.
                        </Typography>

                        <Stack spacing={0.5}>
                            <Typography variant="subtitle2">Notification playground</Typography>
                            <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                                Success / info / warning toasts auto-dismiss. Errors stay on screen
                                until explicitly closed and carry a copy button for reporting — even
                                when the caller passes an explicit duration.
                            </Typography>
                            <Stack direction="row" spacing={1} sx={{ flexWrap: 'wrap', rowGap: 1, mt: 1 }}>
                                <Button
                                    size="small"
                                    variant="outlined"
                                    color="success"
                                    onClick={() => notify.success('API key created')}
                                >
                                    Success
                                </Button>
                                <Button
                                    size="small"
                                    variant="outlined"
                                    onClick={() => notify.info('Reloading provider list')}
                                >
                                    Info
                                </Button>
                                <Button
                                    size="small"
                                    variant="outlined"
                                    color="warning"
                                    onClick={() => notify.warning('Provider quota usage above 80%')}
                                >
                                    Warning
                                </Button>
                                <Button
                                    size="small"
                                    variant="outlined"
                                    color="error"
                                    onClick={() => notify.error(ERROR_DEMO_MESSAGE, { title: ERROR_DEMO_TITLE })}
                                >
                                    Error
                                </Button>
                                <Button
                                    size="small"
                                    variant="outlined"
                                    color="error"
                                    onClick={() =>
                                        notify.error(ERROR_DEMO_MESSAGE, {
                                            title: `${ERROR_DEMO_TITLE} (duration: 3000, must be ignored)`,
                                            duration: 3000,
                                        })
                                    }
                                >
                                    Error + explicit duration
                                </Button>
                                <Button size="small" onClick={() => notify.clear()}>
                                    Clear all
                                </Button>
                            </Stack>
                        </Stack>
                    </Stack>
                </UnifiedCard>
            </CardGrid>
        </PageLayout>
    );
};

export default DevelopPage;
