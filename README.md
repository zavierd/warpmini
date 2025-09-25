# WarpMini (Go)

A minimal cross-platform GUI tool to:
- Login via refresh_token and inject credentials into Warp (macOS Keychain or Windows user file)
- Perform "封号清理" (cleanup) of Warp-related local data

UI
- Single input field: refresh_token
- Two buttons: 登录 (Login), 清理 (Cleanup)

Strict keychain data format
- Conforms to the project format used in:
  - macOS: Keychain service "dev.warp.Warp-Stable"
  - Windows: %LOCALAPPDATA%/warp/Warp/data/dev.warp.Warp-User (DPAPI-encrypted JSON)
- JSON structure stored (both macOS and Windows) is:
  {
    "id_token": {
      "id_token": "<firebase id_token>",
      "refresh_token": "<firebase refresh_token>",
      "expiration_time": "<ISO8601 with +08:00>"
    },
    "refresh_token": "",
    "local_id": "<user_id>",
    "email": "<email>",
    "display_name": "<name or empty>",
    "photo_url": "<picture or empty>",
    "is_onboarded": true,
    "needs_sso_link": false,
    "anonymous_user_type": null,
    "linked_at": null,
    "personal_object_limits": null,
    "is_on_work_domain": true
  }

Build
- macOS:
  go build -o warpmini
- Windows:
  set GOOS=windows
  set GOARCH=amd64
  go build -o warpmini.exe

Notes
- No database connections are used.
- Uses Firebase Secure Token API to exchange refresh_token -> id_token.
- Uses macOS `security` CLI to store to Keychain (service dev.warp.Warp-Stable), stored under both accounts: <email> and "User".
- On Windows, writes DPAPI-encrypted JSON file to `dev.warp.Warp-User` in Warp data directory.
- Cleanup will kill Warp processes and remove known data directories/entries based on platform.
