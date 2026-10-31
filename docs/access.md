# Users, roles and sign-in

## Users and roles

- **Settings → Users** invites people (by email, or a link to share) and gives them roles everywhere or per
  project. The first account is the **Owner** (every permission; there is always at least one active Owner).
- **Settings → Roles** shows the permission matrix. Built-in roles (Admin, Developer, Deployer, Viewer) can be
  cloned into your own.
- A project's **Members** card lets people with `project.members` add colleagues to that project.
- Every change is in the **audit log**.

## API tokens

Settings → API tokens: a token acts as its user, optionally limited to one role and one project, and can expire.
Use them for the [CLI](cli.md), CI and the [API](api.md) (`Authorization: Bearer nat_…`).

## Single sign-on

**Settings → Sign-in providers** adds buttons to the login page of the web panel:

| Provider | Setup |
|---|---|
| **GitHub** | GitHub → Settings → Developer settings → OAuth Apps → New; the panel uses the verified primary email |
| **Google** | Google Cloud console → APIs & Services → Credentials → OAuth client ID (Web application) |
| **OpenID Connect** | Authentik, Keycloak, Zitadel, Okta, Entra ID …: give the issuer URL; the panel reads its discovery document |

For each one, register the **callback URL** the dialog shows (`https://<panel>/api/auth/sso/<id>/callback`) at
the provider, then paste the client ID and secret (stored encrypted).

- Without **allowed email domains**, only people who already have a user can sign in this way (matched by
  verified email).
- With allowed domains you can **create users on their first sign-in** with a default role.
- **Group → role** mappings (OIDC `groups` claim, e.g. `nectar-admins=admin`) are applied at every sign-in.

Sign-in uses the authorization code flow with PKCE and a one-time state; the provider's own MFA applies.

## Passkeys

Settings → **Passkeys** → Add passkey (in the web panel, with a recent Chrome, Edge, Safari or Firefox): the
browser creates a passkey with Touch ID, Face ID, Windows Hello, your phone or a security key. On the login page,
**Sign in with a passkey** then signs you in without email, password or authenticator code — the passkey is both
factors. Synced passkeys (iCloud Keychain, Google Password Manager, 1Password …) work on all your devices.

A passkey belongs to the panel's address: one created at `panel.example.com` does not work at
`http://localhost:8080` or another domain of the panel. The mobile and desktop apps sign in with a password.
An admin who resets someone's two-factor authentication (or `nectar-apps admin reset-mfa`) also removes their
passkeys.

## Two-factor authentication

Settings → Security: any authenticator app, with 10 one-time recovery codes.
