# Tingly Box Enterprise Edition

Enterprise features for multi-user deployment with role-based access control.

## Quick Start

1. Enable enterprise mode in `~/.tingly-box/config.json`:
```json
{
  "scenarios": [
    {
      "scenario": "enterprise",
      "extensions": {
        "enabled": true
      }
    }
  ]
}
```

2. Create admin user (API or UI):
```bash
# Via CLI (coming soon)
tingly-box enterprise create-admin --username admin --password Admin123!
```

3. Access admin panel:
- Navigate to http://localhost:12580/enterprise/login
- Login with admin credentials

## Features

- Multi-user authentication (username/password)
- Role-based access control (Admin, User, ReadOnly)
- API token management with scopes
- Audit logging for compliance
- User management (CRUD, activate/deactivate)
- Password reset functionality

## API Endpoints

All endpoints prefixed with `/enterprise/api/v1`:

- Authentication: `/auth/*`
- User Management: `/users/*` (admin only)
- Token Management: `/tokens/*`, `/my-tokens/*`
- Audit Logs: `/audit/*` (admin only)

## Documentation

See [full documentation](./README.md) for complete API reference, security details, and integration guide.

## License

Enterprise features require a commercial license.
