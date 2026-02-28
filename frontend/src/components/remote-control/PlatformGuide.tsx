import { OpenInNew } from '@mui/icons-material';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import {
    Accordion,
    AccordionDetails,
    AccordionSummary,
    Box,
    Chip,
    Link,
    Stack,
    Typography,
} from '@mui/material';
import {
    IconBrandDingtalk,
    IconBrandDiscord,
    IconBrandSlack,
    IconBrandTelegram,
    IconBrandWechat
} from '@tabler/icons-react';

interface PlatformGuideProps {
    expanded: string | false;
    onChange: (panel: string) => (event: React.SyntheticEvent, isExpanded: boolean) => void;
}

interface PlatformConfig {
    id: string;
    name: string;
    icon: React.ReactNode;
    bgColor: string;
    iconColor: string;
    status: 'available' | 'coming-soon' | 'beta';
    requiredFields: string[];
    steps: React.ReactNode;
}

const platformConfigs: PlatformConfig[] = [
    {
        id: 'telegram',
        name: 'Telegram',
        icon: <IconBrandTelegram size={24} stroke={1.5} />,
        bgColor: '#0088cc',
        iconColor: '#ffffff',
        status: 'available',
        requiredFields: ['Bot Token'],
        steps: (
            <Stack spacing={2}>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        1. Create a bot
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        Open Telegram, search{' '}
                        <Link href="https://t.me/BotFather" target="_blank">
                            @BotFather <OpenInNew sx={{ fontSize: 10 }} />
                        </Link>
                    </Typography>
                    <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                        Send <code>/newbot</code>, follow the prompts, and copy the token
                    </Typography>
                </Box>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        2. Add bot
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        Click "Add Bot" button above and paste the token to create your bot.
                    </Typography>
                </Box>
                <Box sx={{ bgcolor: 'info.lighter', p: 1.5, borderRadius: 1, border: '1px solid', borderColor: 'info.light' }}>
                    <Typography variant="body2" color="info.dark">
                        Tip: Configure traffic proxy as needed for network access.
                    </Typography>
                </Box>
            </Stack>
        ),
    },
    {
        id: 'feishu',
        name: 'Feishu (飞书)',
        icon: <Typography sx={{ color: '#ffffff', fontWeight: 'bold', fontSize: '0.7rem' }}>飞书</Typography>,
        bgColor: '#3370FF',
        iconColor: '#ffffff',
        status: 'available',
        requiredFields: ['App ID', 'App Secret'],
        steps: (
            <Stack spacing={2}>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        1. Create a Feishu bot
                    </Typography>
                    <Typography variant="body2" color="text.secondary" component="div">
                        <Box component="ul" sx={{ pl: 2, m: 0 }}>
                            <li>Visit <Link href="https://open.feishu.cn/" target="_blank">Feishu Open Platform <OpenInNew sx={{ fontSize: 10 }} /></Link></li>
                            <li>Create a new app - Enable Bot capability</li>
                            <li>Permissions: Add <code>im:message</code> (send messages) and <code>im:message.p2p_msg:readonly</code> (receive messages)</li>
                            <li>Events: Add <code>im.message.receive_v1</code> (receive messages)</li>
                            <li>Select <strong>Long Connection</strong> mode (requires running nanobot first to establish connection)</li>
                            <li>Get App ID and App Secret from "Credentials & Basic Info"</li>
                            <li>Publish the app</li>
                        </Box>
                    </Typography>
                </Box>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        2. Add bot
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        Click "Add Bot" button above and fill in App ID and App Secret to create your bot.
                    </Typography>
                </Box>
                <Box sx={{ bgcolor: 'info.lighter', p: 1.5, borderRadius: 1, border: '1px solid', borderColor: 'info.light' }}>
                    <Typography variant="body2" color="info.dark">
                        Tip: Feishu uses WebSocket - no public IP needed. Configure traffic proxy as needed.
                    </Typography>
                </Box>
            </Stack>
        ),
    },
    {
        id: 'dingtalk',
        name: 'DingTalk (钉钉)',
        icon: <IconBrandDingtalk size={24} stroke={1.5} />,
        bgColor: '#0089FF',
        iconColor: '#ffffff',
        status: 'available',
        requiredFields: ['App Key', 'App Secret'],
        steps: (
            <Stack spacing={2}>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        1. Create a DingTalk bot
                    </Typography>
                    <Typography variant="body2" color="text.secondary" component="div">
                        <Box component="ul" sx={{ pl: 2, m: 0 }}>
                            <li>Visit <Link href="https://open.dingtalk.com/" target="_blank">DingTalk Open Platform <OpenInNew sx={{ fontSize: 10 }} /></Link></li>
                            <li>Create a new app - Add Robot capability</li>
                            <li>Configuration:</li>
                            <Box component="ul" sx={{ pl: 2 }}>
                                <li>Toggle <strong>Stream Mode</strong> ON</li>
                                <li>Permissions: Add necessary permissions for sending messages</li>
                            </Box>
                            <li>Get AppKey (Client ID) and AppSecret (Client Secret) from "Credentials"</li>
                            <li>Publish the app</li>
                        </Box>
                    </Typography>
                </Box>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        2. Add bot
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        Click "Add Bot" button above and fill in App Key and App Secret to create your bot.
                    </Typography>
                </Box>
                <Box sx={{ bgcolor: 'info.lighter', p: 1.5, borderRadius: 1, border: '1px solid', borderColor: 'info.light' }}>
                    <Typography variant="body2" color="info.dark">
                        Tip: DingTalk uses Stream Mode - no public IP required. Configure traffic proxy as needed.
                    </Typography>
                </Box>
            </Stack>
        ),
    },
    {
        id: 'lark',
        name: 'Lark',
        icon: <Typography sx={{ color: '#ffffff', fontWeight: 'bold', fontSize: '0.7rem' }}>Lark</Typography>,
        bgColor: '#3370FF',
        iconColor: '#ffffff',
        status: 'available',
        requiredFields: ['App ID', 'App Secret'],
        steps: (
            <Stack spacing={2}>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        1. Create a Lark bot
                    </Typography>
                    <Typography variant="body2" color="text.secondary" component="div">
                        <Box component="ul" sx={{ pl: 2, m: 0 }}>
                            <li>Visit <Link href="https://open.larksuite.com/" target="_blank">Lark Open Platform <OpenInNew sx={{ fontSize: 10 }} /></Link></li>
                            <li>Create a new app - Enable Bot capability</li>
                            <li>Permissions: Add <code>im:message</code> (send messages) and <code>im:message.p2p_msg:readonly</code> (receive messages)</li>
                            <li>Events: Add <code>im.message.receive_v1</code> (receive messages)</li>
                            <li>Select <strong>Long Connection</strong> mode</li>
                            <li>Get App ID and App Secret from "Credentials & Basic Info"</li>
                            <li>Publish the app</li>
                        </Box>
                    </Typography>
                </Box>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        2. Add bot
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        Click "Add Bot" button above and fill in App ID and App Secret to create your bot.
                    </Typography>
                </Box>
                <Box sx={{ bgcolor: 'info.lighter', p: 1.5, borderRadius: 1, border: '1px solid', borderColor: 'info.light' }}>
                    <Typography variant="body2" color="info.dark">
                        Tip: Lark uses WebSocket - no webhook or public IP needed. Configure traffic proxy as needed.
                    </Typography>
                </Box>
            </Stack>
        ),
    },
    {
        id: 'wechat',
        name: 'WeChat',
        icon: <IconBrandWechat size={24} stroke={1.5} />,
        bgColor: '#07c160',
        iconColor: '#ffffff',
        status: 'coming-soon',
        requiredFields: ['App ID', 'App Secret'],
        steps: (
            <Typography variant="body2" color="text.secondary">
                WeChat bot integration is currently under development. Stay tuned for updates!
            </Typography>
        ),
    },
    {
        id: 'discord',
        name: 'Discord',
        icon: <IconBrandDiscord size={24} stroke={1.5} />,
        bgColor: '#5865F2',
        iconColor: '#ffffff',
        status: 'coming-soon',
        requiredFields: ['Bot Token', 'Message Content Intent'],
        steps: (
            <Typography variant="body2" color="text.secondary">
                Discord bot integration is currently under development. Stay tuned for updates!
            </Typography>
        ),
    },
    {
        id: 'slack',
        name: 'Slack',
        icon: <IconBrandSlack size={24} stroke={1.5} />,
        bgColor: '#4A154B',
        iconColor: '#ffffff',
        status: 'coming-soon',
        requiredFields: ['Bot Token', 'App-Level Token'],
        steps: (
            <Typography variant="body2" color="text.secondary">
                Slack bot integration is currently under development. Stay tuned for updates!
            </Typography>
        ),
    },
];

const PlatformGuide: React.FC<PlatformGuideProps> = ({ expanded, onChange }) => {
    return (
        <Stack spacing={1}>
            {platformConfigs.map((platform) => (
                <Accordion
                    key={platform.id}
                    expanded={expanded === platform.id}
                    onChange={onChange(platform.id)}
                    disableGutters
                    sx={{
                        '&:before': { display: 'none' },
                        border: '1px solid',
                        borderColor: 'divider',
                        borderRadius: 1,
                        overflow: 'hidden',
                    }}
                >
                    <AccordionSummary
                        expandIcon={<ExpandMoreIcon />}
                        sx={{
                            '& .MuiAccordionSummary-content': {
                                alignItems: 'center',
                            },
                        }}
                    >
                        <Stack direction="row" spacing={2} alignItems="center">
                            <Box
                                sx={{
                                    width: 36,
                                    height: 36,
                                    borderRadius: 1.5,
                                    bgcolor: platform.bgColor,
                                    display: 'flex',
                                    alignItems: 'center',
                                    justifyContent: 'center',
                                    flexShrink: 0,
                                    color: platform.iconColor,
                                }}
                            >
                                {platform.icon}
                            </Box>
                            <Box>
                                <Stack direction="row" spacing={1} alignItems="center">
                                    <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
                                        {platform.name}
                                    </Typography>
                                    {platform.status === 'coming-soon' && (
                                        <Chip
                                            label="Coming Soon"
                                            size="small"
                                            sx={{
                                                height: 18,
                                                fontSize: '0.65rem',
                                                bgcolor: 'grey.100',
                                                color: 'text.secondary',
                                            }}
                                        />
                                    )}
                                    {platform.status === 'beta' && (
                                        <Chip
                                            label="Beta"
                                            size="small"
                                            color="warning"
                                            sx={{ height: 18, fontSize: '0.65rem' }}
                                        />
                                    )}
                                </Stack>
                                <Typography variant="caption" color="text.secondary">
                                    Required: {platform.requiredFields.join(', ')}
                                </Typography>
                            </Box>
                        </Stack>
                    </AccordionSummary>
                    <AccordionDetails sx={{ pt: 0, bgcolor: 'grey.50' }}>
                        {platform.steps}
                    </AccordionDetails>
                </Accordion>
            ))}
        </Stack>
    );
};

export default PlatformGuide;
