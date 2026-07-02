# API Tokens

Path: `/tingly-box-token`

![API Tokens](../images/api-tokens.png)

The API Tokens page manages Bearer Tokens for external clients accessing Tingly-Box. Multiple named tokens can be created, suitable for scripts, CI/CD pipelines, or third-party integrations.

---

## Page Structure

### Token List Table

| Column | Description |
|--------|-------------|
| Name | Token name (specified at creation) |
| UUID | Unique token identifier (truncated) |
| Token | Token value (masked by default; click to show/hide) |
| Status | Active / Disabled |
| Created | Creation date |
| Last Used | Most recent usage date |
| Actions | Copy, show/hide, delete |

An empty state icon and message are shown when no tokens exist.

---

## Creating a Token

1. Click the **Create Token** button in the top-right
2. Enter a **Display Name** in the dialog (e.g. `ci-pipeline`, `my-script`)
3. Confirm — the token value is generated and shown immediately

> **Note**: The token value is only shown once, immediately after creation. Copy it right away.

---

## Using a Token

Created tokens can be used as Bearer Tokens to access Tingly-Box's proxy interfaces:

```bash
curl https://<your-tingly-box>/api/... \
  -H "Authorization: Bearer <your-token>"
```

Or as the `api_key` in SDK configurations:

```python
client = OpenAI(
    base_url="https://<your-tingly-box>/agent/openai/v1",
    api_key="<your-token>",
)
```

---

## Managing Tokens

### View Token Value

Click the eye icon on a token row to toggle between plaintext and masked display.

### Copy Token

Click the copy icon — the token value is written to the clipboard.

### Delete Token

Click the delete icon to open a confirmation dialog (showing the token name). After confirmation, the token is permanently deleted and immediately invalidated. Any clients holding the deleted token will no longer be able to access Tingly-Box.

---

## Comparison with User Token

| | API Token | User Token |
|-|-----------|------------|
| Location | `/tingly-box-token` | `/access-control` |
| Purpose | External clients/scripts | Web UI login |
| Quantity | Multiple | Single |
| Named | Yes | No |

---

## Related Pages

- [Access Control](./18-access-control.md)
- [Credentials](./08-credentials.md)
