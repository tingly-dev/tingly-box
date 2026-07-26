import CodeBlock from '@/components/CodeBlock';
import {getDisplayOrigin} from '@/utils/protocol';
import {Box, Stack, Typography} from '@mui/material';
import {useTranslation} from 'react-i18next';

// NotifyGuide is the body of the IM Notify usage guide — it teaches an operator
// how to drive a bot's chat via the authenticated bot-interaction API, embedded
// in the product (ux-principles #8). It is organized around the three questions
// an integrator actually asks (ux-principles #1):
//
//   1. Who can call this, and how do I authenticate?
//   2. What exactly do I send?  (concrete curl + JSON — principle #5)
//   3. Where do I get the chat_id the body requires?
//
// Auth reuses the operator's existing user token — no new credential — so the
// guide says so plainly rather than inventing a "notify token" story (see
// .design/bot-interaction-api.md §3.1). The base URL is the operator's own
// origin so the curl is copy-pasteable as-is (principle #11 — hand over the
// artifact for the next action).
const NotifyGuide: React.FC = () => {
    const {t} = useTranslation();
    const origin = getDisplayOrigin();

    // Concrete, copy-pasteable curl. <BOT_UUID> and <CHAT_ID> are left as
    // placeholders the operator fills from the table below + the per-bot chats
    // list — the guide points there explicitly in section 3.
    const curl = `curl -X POST ${origin}/api/v1/bots/<BOT_UUID>/notify \\
  -H "Authorization: Bearer <USER_TOKEN>" \\
  -H "Content-Type: application/json" \\
  -d '{
    "chat_id": "<CHAT_ID>",
    "title": "Build #412 failed",
    "body": "main branch is red",
    "level": "warn"
  }'`;

    const jsonBody = `{
  "chat_id": "<CHAT_ID>",   // required — the channel-native conversation id
  "title": "Build #412 failed",  // optional
  "body": "main branch is red",  // required
  "level": "info"                // optional: info | warn | error
}`;

    return (
        <Stack spacing={2.5}>
            {/* 1. Who can call this / auth */}
            <Box>
                <Typography variant="subtitle2" sx={{fontWeight: 600, mb: 0.5}}>
                    {t('notify.guide.auth.title', {defaultValue: '1. Authenticate with your user token'})}
                </Typography>
                <Typography variant="body2" sx={{color: 'text.secondary'}}>
                    {t('notify.guide.auth.body', {
                        defaultValue: 'Any integration can drive a bot’s chat. Auth reuses your existing operator user token (the same one this web UI uses) as a Bearer header — no new credential to mint. Interactive prompts (/interact) and one-way notifications (/notify) are separate URLs, so the request shape is the mode.',
                    })}
                </Typography>
            </Box>

            {/* 2. What to send */}
            <Box>
                <Typography variant="subtitle2" sx={{fontWeight: 600, mb: 0.5}}>
                    {t('notify.guide.send.title', {defaultValue: '2. Send a one-way notification'})}
                </Typography>
                <Typography variant="body2" sx={{color: 'text.secondary', mb: 1}}>
                    {t('notify.guide.send.body', {
                        defaultValue: 'POST to /api/v1/bots/{bot}/notify with the bot UUID in the path. A 200 means delivered.',
                    })}
                </Typography>
                <CodeBlock code={curl} language="bash"/>
                <Typography variant="caption" sx={{color: 'text.secondary', mt: 1, display: 'block'}}>
                    {t('notify.guide.send.json', {defaultValue: 'Request body:'})}
                </Typography>
                <CodeBlock code={jsonBody} language="json"/>
            </Box>

            {/* 3. Where to get chat_id */}
            <Box>
                <Typography variant="subtitle2" sx={{fontWeight: 600, mb: 0.5}}>
                    {t('notify.guide.chatid.title', {defaultValue: '3. Get the chat_id from the list below'})}
                </Typography>
                <Typography variant="body2" sx={{color: 'text.secondary'}}>
                    {t('notify.guide.chatid.body', {
                        defaultValue: 'Each bot row shows the chats it can reach — open it and copy the Chat ID. A chat appears here only after a user has messaged the bot (on Telegram/Discord/Slack, pair it first). Or send a test message right from the row to verify the link end-to-end.',
                    })}
                </Typography>
            </Box>
        </Stack>
    );
};

export default NotifyGuide;
