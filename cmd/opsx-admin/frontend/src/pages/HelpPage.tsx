import React from 'react';
import {
  Box,
  Card,
  CardContent,
  Typography,
  Button,
  Stepper,
  Step,
  StepLabel,
  StepContent,
  Alert,
  List,
  ListItem,
  ListItemIcon,
  ListItemText,
} from '@mui/material';
import {
  Terminal as TerminalIcon,
  ContentCopy as CopyIcon,
  OpenInNew as OpenIcon,
  CheckCircle as CheckIcon,
  Key as KeyIcon,
  Info as InfoIcon,
} from '@mui/icons-material';
import { alpha } from '@mui/material/styles';

const steps = [
  {
    label: 'Get your API token via SSH',
    description: 'SSH to the server and run the opsx-token command to get your API token.',
  },
  {
    label: 'Copy the token',
    description: 'Copy the generated token from your SSH session.',
  },
  {
    label: 'Access the admin UI',
    description: 'Open the admin URL in your browser and login with your token.',
  },
];

export const HelpPage: React.FC = () => {
  const copySSHCommand = () => {
    const cmd = 'ssh user@your-server.com "opsx-token"';
    navigator.clipboard.writeText(cmd);
  };

  const copyAdminUrl = () => {
    navigator.clipboard.writeText(window.location.origin);
  };

  return (
    <Box>
      <Typography variant="h4" fontWeight={700} gutterBottom>
        Welcome to OPSX Service
      </Typography>
      <Typography variant="body1" color="text.secondary" sx={{ mb: 4 }}>
        A secure remote execution service powered by Claude Code
      </Typography>

      {/* Quick Start Steps */}
      <Card sx={{ mb: 4 }}>
        <CardContent sx={{ p: 3 }}>
          <Typography variant="h6" fontWeight={600} gutterBottom>
            Quick Start Guide
          </Typography>
          <Stepper orientation="vertical" sx={{ mt: 2 }}>
            {steps.map((step) => (
              <Step key={step.label} expanded>
                <StepLabel>
                  <Typography fontWeight={500}>{step.label}</Typography>
                </StepLabel>
                <StepContent>
                  <Typography variant="body2" color="text.secondary" paragraph>
                    {step.description}
                  </Typography>
                </StepContent>
              </Step>
            ))}
          </Stepper>
        </CardContent>
      </Card>

      {/* Get Token */}
      <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 3, mb: 4 }}>
        <Box sx={{ width: { xs: '100%', md: 'calc(50% - 12px)' } }}>
          <Card sx={{ height: '100%' }}>
            <CardContent sx={{ p: 3 }}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, mb: 2 }}>
                <TerminalIcon color="primary" />
                <Typography variant="h6" fontWeight={600}>
                  Step 1: Get Your Token
                </Typography>
              </Box>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
                Run this command in your terminal to generate an API token:
              </Typography>

              <Box
                sx={{
                  p: 2,
                  bgcolor: alpha('#2563eb', 0.05),
                  borderRadius: 2,
                  fontFamily: 'monospace',
                  fontSize: 14,
                  mb: 2,
                }}
              >
                ssh user@your-server.com "opsx-token"
              </Box>

              <Button
                variant="outlined"
                size="small"
                startIcon={<CopyIcon />}
                onClick={copySSHCommand}
                sx={{ mb: 3 }}
              >
                Copy Command
              </Button>

              <Alert severity="info" icon={<InfoIcon />}>
                <Typography variant="body2">
                  Replace <code>user@your-server.com</code> with your actual SSH credentials.
                  The command will output a token like: <code>tingly-box-xxxxx...</code>
                </Typography>
              </Alert>
            </CardContent>
          </Card>
        </Box>

        <Box sx={{ width: { xs: '100%', md: 'calc(50% - 12px)' } }}>
          <Card sx={{ height: '100%' }}>
            <CardContent sx={{ p: 3 }}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, mb: 2 }}>
                <OpenIcon color="primary" />
                <Typography variant="h6" fontWeight={600}>
                  Step 2: Access Admin UI
                </Typography>
              </Box>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
                Open the admin interface in your browser:
              </Typography>

              <Box
                sx={{
                  p: 2,
                  bgcolor: alpha('#10b981', 0.05),
                  borderRadius: 2,
                  fontFamily: 'monospace',
                  fontSize: 14,
                  mb: 2,
                }}
              >
                {window.location.origin}
              </Box>

              <Button
                variant="outlined"
                size="small"
                startIcon={<CopyIcon />}
                onClick={copyAdminUrl}
                sx={{ mb: 3 }}
              >
                Copy URL
              </Button>

              <Alert severity="success" icon={<CheckIcon />}>
                <Typography variant="body2">
                  If you're accessing this page via SSH tunnel, the URL above is for your local machine.
                </Typography>
              </Alert>
            </CardContent>
          </Card>
        </Box>
      </Box>

      {/* SSH Tunnel Instructions */}
      <Card sx={{ mb: 4 }}>
        <CardContent sx={{ p: 3 }}>
          <Typography variant="h6" fontWeight={600} gutterBottom>
            SSH Tunnel Setup (Mobile Device)
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
            If you need to access the admin UI from your mobile device, set up an SSH tunnel:
          </Typography>

          <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 3 }}>
            <Box sx={{ width: { xs: '100%', md: 'calc(50% - 12px)' } }}>
              <Typography variant="subtitle2" fontWeight={600} gutterBottom>
                From your computer:
              </Typography>
              <Box
                sx={{
                  p: 2,
                  bgcolor: 'grey.50',
                  borderRadius: 2,
                  fontFamily: 'monospace',
                  fontSize: 13,
                  mb: 2,
                }}
              >
                ssh -L 9246:localhost:9246 user@your-server.com
              </Box>
              <Typography variant="caption" color="text.secondary">
                Keep this terminal open. Then access http://localhost:9246 on your mobile (same network) or use port forwarding.
              </Typography>
            </Box>
            <Box sx={{ width: { xs: '100%', md: 'calc(50% - 12px)' } }}>
              <Typography variant="subtitle2" fontWeight={600} gutterBottom>
                Alternative - Server Direct Access:
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                If the server has a public IP, you can:
              </Typography>
              <List dense>
                <ListItem>
                  <ListItemIcon>
                    <CheckIcon fontSize="small" color="success" />
                  </ListItemIcon>
                  <ListItemText
                    primary="Direct URL"
                    secondary="Access http://your-server-ip:9246 directly"
                  />
                </ListItem>
                <ListItem>
                  <ListItemIcon>
                    <CheckIcon fontSize="small" color="success" />
                  </ListItemIcon>
                  <ListItemText
                    primary="With Nginx/Proxy"
                    secondary="Configure reverse proxy for HTTPS access"
                  />
                </ListItem>
              </List>
            </Box>
          </Box>
        </CardContent>
      </Card>

      {/* Features */}
      <Card>
        <CardContent sx={{ p: 3 }}>
          <Typography variant="h6" fontWeight={600} gutterBottom>
            Features
          </Typography>
          <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 3 }}>
            <Box sx={{ width: { xs: '100%', md: 'calc(33.333% - 16px)' } }}>
              <List dense>
                <ListItem>
                  <ListItemIcon>
                    <KeyIcon color="action" fontSize="small" />
                  </ListItemIcon>
                  <ListItemText primary="Secure Tokens" secondary="JWT-based authentication" />
                </ListItem>
                <ListItem>
                  <ListItemIcon>
                    <CheckIcon color="action" fontSize="small" />
                  </ListItemIcon>
                  <ListItemText primary="Audit Logging" secondary="Track all API usage" />
                </ListItem>
              </List>
            </Box>
            <Box sx={{ width: { xs: '100%', md: 'calc(33.333% - 16px)' } }}>
              <List dense>
                <ListItem>
                  <ListItemIcon>
                    <TerminalIcon color="action" fontSize="small" />
                  </ListItemIcon>
                  <ListItemText primary="Claude Code" secondary="AI-powered execution" />
                </ListItem>
                <ListItem>
                  <ListItemIcon>
                    <InfoIcon color="action" fontSize="small" />
                  </ListItemIcon>
                  <ListItemText primary="Rate Limiting" secondary="Brute-force protection" />
                </ListItem>
              </List>
            </Box>
            <Box sx={{ width: { xs: '100%', md: 'calc(33.333% - 16px)' } }}>
              <List dense>
                <ListItem>
                  <ListItemIcon>
                    <OpenIcon color="action" fontSize="small" />
                  </ListItemIcon>
                  <ListItemText primary="Session Management" secondary="Track execution sessions" />
                </ListItem>
                <ListItem>
                  <ListItemIcon>
                    <CopyIcon color="action" fontSize="small" />
                  </ListItemIcon>
                  <ListItemText primary="Token Generation" secondary="Easy API access" />
                </ListItem>
              </List>
            </Box>
          </Box>
        </CardContent>
      </Card>
    </Box>
  );
};
