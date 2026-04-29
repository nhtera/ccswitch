# ccswitch Export Bundle Format

Filename convention: `ccswitch-export-YYYYMMDD-HHMMSS.cce`. Permission `0600`.

## Wire Format

```
[ magic        : 4 bytes ]    "CCEX"
[ version      : 1 byte  ]    0x01
[ argon_time   : 1 byte  ]    iterations
[ argon_memMB  : 2 bytes ]    BE; megabytes
[ argon_threads: 1 byte  ]
[ salt         : 16 bytes]
[ nonce        : 12 bytes]
[ ciphertext   : N bytes ]    AES-256-GCM
[ tag          : 16 bytes]    GCM auth tag (covers all preceding bytes)
```

The header bytes (magic .. nonce) are bound into the AEAD as additional
authenticated data, so any tampering with params or salt fails verification.

## Plaintext (after decrypt)

UTF-8 JSON:

```json
{
  "version": 1,
  "exported_at": "2026-04-29T10:42:11Z",
  "profiles": [
    {
      "name": "work",
      "type": "sso",
      "created_at": "2026-04-29T03:13:05Z",
      "note": "Acme Corp",
      "env": { "ANTHROPIC_BASE_URL": "https://gw.acme.com" },
      "fingerprint": "sha256:...",
      "credential_blob_b64": "<base64 of opaque credential bytes>"
    }
  ]
}
```

Importers MUST reject `version` values they don't understand.

## Crypto

- Key derivation: Argon2id, params recorded in header so older bundles open
  after defaults change.
- Cipher: AES-256-GCM. 12-byte nonce per bundle (rolling new nonces per
  encryption — never reused for the same passphrase + salt).
- A wrong passphrase, corrupted ciphertext, or tampered header all surface
  as a single opaque error (no oracle leakage).

## Compatibility Rules

- New optional fields may be added inside `profiles[*]`; importers must
  ignore unknown keys.
- A bump of the bundle `version` (top level) is required for any breaking
  change.
- The wire-format `version` byte is independent — bumped only when the
  binary frame changes.
