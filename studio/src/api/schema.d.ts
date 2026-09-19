/**
 * Generated from platform/api/openapi.yaml by scripts/gen-api.mjs
 * (openapi-typescript). Do not edit; run `pnpm --filter @glossa/studio gen:api`.
 */

export interface paths {
    "/v1/meta": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * What this deployment offers
         * @description The sign-in methods this server accepts, whether it sends email,
         *     and glossa-edge's public base URL, so clients don't guess.
         *     Public and the same for every caller.
         */
        get: operations["getMeta"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/auth/magic-links": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Email a sign-in link
         * @description Sends a single-use link valid for 15 minutes. Signing in with it
         *     creates the account (and the person's individual tenant) if the
         *     address is new, and verifies the address. Always `202`, so the
         *     response never reveals whether an account exists. Problem codes:
         *     `email_disabled` (404: the server sends no email; see
         *     `GET /v1/meta`).
         */
        post: operations["requestMagicLink"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/auth/magic-link-redemptions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Sign in with an emailed link
         * @description Spends the link's token and starts a session. Problem codes:
         *     `link_invalid` (unknown, expired or already used),
         *     `email_disabled` (404).
         */
        post: operations["redeemMagicLink"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/auth/registrations": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Register with a password
         * @description Creates the account and its individual tenant, then emails a
         *     verification link (a sign-in link). Password sign-in works once
         *     the address is verified. Always `202`: an existing address gets
         *     a sign-in link instead, so registration can't be used to probe
         *     for accounts. On a server that sends no email
         *     (`email_delivery: false` in `GET /v1/meta`) nothing is mailed and
         *     the account can sign in with its password right away, its
         *     address unverified; an existing address is left untouched.
         */
        post: operations["register"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/auth/password-sessions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Sign in with email and password
         * @description Problem codes: `invalid_credentials` (401), `totp_required` (401,
         *     send `totp_code`), `totp_invalid` (401), `email_unverified` (403;
         *     only on servers that send email), `account_locked` (429 after
         *     repeated failures; the lock lifts after 15 minutes).
         */
        post: operations["signInWithPassword"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/auth/password-resets": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Email a password reset link
         * @description Always `202`, whether or not the address has an account. Problem
         *     codes: `email_disabled` (404).
         */
        post: operations["requestPasswordReset"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/auth/password-reset-redemptions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Set a new password with a reset link
         * @description Sets the password, verifies the address and signs the person out
         *     everywhere. Problem codes: `link_invalid` (401), `weak_password`
         *     (400), `email_disabled` (404).
         */
        post: operations["resetPassword"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/auth/passkey-challenges": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Start a passkey sign-in
         * @description Returns WebAuthn request options for `navigator.credentials.get`
         *     and sets a short-lived, HttpOnly ceremony cookie that
         *     `POST /v1/auth/passkey-sessions` consumes. Problem codes:
         *     `passkeys_disabled` (404), `no_passkeys` (404).
         */
        post: operations["beginPasskeySignIn"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/auth/passkey-sessions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Sign in with a passkey assertion
         * @description Problem codes: `passkey_invalid` (401).
         */
        post: operations["finishPasskeySignIn"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/auth/session": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post?: never;
        /** Sign out of this session */
        delete: operations["signOut"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/auth/sessions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post?: never;
        /**
         * Sign out of every session
         * @description Revokes all of the person's sessions on every device.
         */
        delete: operations["signOutEverywhere"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/me": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * The signed-in person and their tenants
         * @description Also accepts any open invitations addressed to the person's
         *     verified email.
         */
        get: operations["getMe"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/me/passkey-challenges": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Start adding a passkey
         * @description Returns WebAuthn creation options for
         *     `navigator.credentials.create` and sets the ceremony cookie.
         *     Problem codes: `passkeys_disabled` (404).
         */
        post: operations["beginPasskeyRegistration"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/me/passkeys": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * The signed-in person's passkeys
         * @description Every passkey registered to the person, oldest first, on any
         *     device — not only this browser's. Listed even while passkeys are
         *     not configured on the server.
         */
        get: operations["listPasskeys"];
        put?: never;
        /**
         * Add a passkey
         * @description Problem codes: `passkey_invalid` (400).
         */
        post: operations["finishPasskeyRegistration"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/me/passkeys/{passkey}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A passkey `id` (its credential ID, base64url). */
                passkey: components["parameters"]["PasskeyPath"];
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        post?: never;
        /**
         * Remove one of the signed-in person's passkeys
         * @description The passkey can't sign in any more. Sessions it started stay
         *     signed in (sign out everywhere to end them). Another person's
         *     passkey is `404`, like an unknown one.
         */
        delete: operations["deletePasskey"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/me/totp": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /**
         * Start enrolling an authenticator app
         * @description Creates a pending TOTP secret (replacing any earlier pending
         *     one). It takes effect once confirmed with a code. Problem codes:
         *     `totp_already_enabled` (409).
         */
        put: operations["beginTotpEnrollment"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/me/totp/confirmation": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Confirm the authenticator app with a code
         * @description Problem codes: `totp_invalid` (400), `totp_not_pending` (409).
         */
        post: operations["confirmTotpEnrollment"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/me/totp/deactivation": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Turn TOTP off with a current code
         * @description Problem codes: `totp_invalid` (400), `totp_not_enabled` (409).
         */
        post: operations["disableTotp"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/message-previews": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Parse, convert and format a message without storing it
         * @description Runs the server's MessageFormat kernel — the one ICU MF1
         *     converter (RFC 0002 §5) — on `source`: parses it in `syntax`
         *     (default `mf1`) for `locale` into the canonical MF2 data model
         *     (`message`), serializes that as MF2 (`mf2`), derives `arguments`
         *     and `markup`, and, when `values` are sent, formats it
         *     (`formatted`, with MF2 fallbacks for placeholders that failed).
         *     Source that doesn't parse is still `200`, with `valid: false`
         *     and the kernel's error codes in `errors`, so editors can show
         *     them inline; formatting problems are `errors` with stage
         *     `format`. Nothing is stored and no tenant data is read, but the
         *     caller must be signed in or send an API token. Limits: `source`
         *     at most 20 000 bytes; `values` at most 100 names of at most 64
         *     characters, each a string (at most 1 000 bytes), number or
         *     boolean; 10 requests a second per person or token, bursts of up
         *     to 120 (per server instance). Problem codes: `message_too_long`,
         *     `invalid_locale`, `invalid_syntax`, `invalid_values` (400),
         *     `rate_limited` (429).
         */
        post: operations["previewMessage"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Tenants the caller can act in
         * @description A person's active memberships; an API token's own tenant.
         */
        get: operations["listTenants"];
        put?: never;
        /**
         * Create an organization
         * @description The caller becomes its owner. Only people create organizations.
         *     Problem codes: `slug_taken` (409).
         */
        post: operations["createTenant"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
            };
            cookie?: never;
        };
        /**
         * A tenant
         * @description Needs `tenant.read`.
         */
        get: operations["getTenant"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/members": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
            };
            cookie?: never;
        };
        /**
         * Members and open invitations
         * @description Needs `members.read`.
         */
        get: operations["listMembers"];
        put?: never;
        /**
         * Invite someone by email
         * @description Opens an invitation that becomes an active membership when the
         *     address's owner signs in. Needs `members.manage`; the `owner`
         *     role needs `owners.manage`. Problem codes: `already_member`
         *     (409), `individual_tenant` (409), `owner_change_forbidden`
         *     (403), `locales_need_locale_role` (400).
         */
        post: operations["addMember"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/members/{member}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A member `id`. */
                member: components["parameters"]["MemberPath"];
            };
            cookie?: never;
        };
        /**
         * A member
         * @description Needs `members.read`.
         */
        get: operations["getMember"];
        put?: never;
        post?: never;
        /**
         * Remove a member or withdraw an invitation
         * @description Needs `members.manage` (`owners.manage` for an owner).
         *     `If-Match` is optional here; when sent it must match. Problem
         *     codes: `last_owner` (409), `owner_change_forbidden` (403).
         */
        delete: operations["removeMember"];
        options?: never;
        head?: never;
        /**
         * Change a member's roles or locales
         * @description Members omitted from the body keep their value. Needs
         *     `members.manage`; anything touching the `owner` role needs
         *     `owners.manage`. Problem codes: `last_owner` (409),
         *     `owner_change_forbidden` (403), `locales_need_locale_role` (400).
         */
        patch: operations["updateMember"];
        trace?: never;
    };
    "/v1/tenants/{tenant}/tokens": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
            };
            cookie?: never;
        };
        /**
         * API tokens, including revoked ones
         * @description Needs `tokens.read`.
         */
        get: operations["listTokens"];
        put?: never;
        /**
         * Create an API token
         * @description The response is the only time the secret is shown. A token's
         *     scopes can't exceed what its creator may do. Needs
         *     `tokens.manage`. Problem codes: `scope_exceeds_grant` (403).
         */
        post: operations["createToken"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/tokens/{token}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description An API token `id` (never its secret). */
                token: components["parameters"]["TokenPath"];
            };
            cookie?: never;
        };
        /**
         * An API token (never its secret)
         * @description Needs `tokens.read`.
         */
        get: operations["getToken"];
        put?: never;
        post?: never;
        /**
         * Revoke an API token
         * @description The token stops working at once and stays listed as revoked.
         *     Needs `tokens.manage`. Problem codes: `token_revoked` (409).
         */
        delete: operations["revokeToken"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
            };
            cookie?: never;
        };
        /**
         * Projects of the tenant
         * @description Needs `catalog.read`.
         */
        get: operations["listProjects"];
        put?: never;
        /**
         * Create a project
         * @description The source locale is fixed at creation. Needs `catalog.write`.
         *     Problem codes: `slug_taken` (409), `invalid_slug`, `invalid_name`,
         *     `invalid_locale`, `invalid_syntax` (400).
         */
        post: operations["createProject"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        /**
         * A project
         * @description Needs `catalog.read`.
         */
        get: operations["getProject"];
        put?: never;
        post?: never;
        /**
         * Delete a project and everything in it
         * @description Deletes its applications, messages, source history, locales and
         *     translations. Needs `tenant.manage`. `If-Match` is optional; when
         *     sent it must match.
         */
        delete: operations["deleteProject"];
        options?: never;
        head?: never;
        /**
         * Rename a project or change its settings
         * @description Members omitted from the body keep their value; the source locale
         *     can't change. Needs `catalog.write`. Problem codes: `slug_taken`
         *     (409).
         */
        patch: operations["updateProject"];
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/applications": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        /**
         * Applications of a project
         * @description Needs `catalog.read`.
         */
        get: operations["listApplications"];
        put?: never;
        /**
         * Add an application to a project
         * @description Needs `catalog.write`. Problem codes: `slug_taken` (409),
         *     `invalid_platform` (400).
         */
        post: operations["createApplication"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/applications/{application}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description An application `id`. */
                application: components["parameters"]["ApplicationPath"];
            };
            cookie?: never;
        };
        /**
         * An application
         * @description Needs `catalog.read`.
         */
        get: operations["getApplication"];
        put?: never;
        post?: never;
        /**
         * Remove an application
         * @description Needs `catalog.write`. `If-Match` is optional; when sent it must match.
         */
        delete: operations["deleteApplication"];
        options?: never;
        head?: never;
        /**
         * Change an application
         * @description Needs `catalog.write`. Problem codes: `slug_taken` (409).
         */
        patch: operations["updateApplication"];
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/messages": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        /**
         * Messages of a project, by key
         * @description Filters combine. `missing_in` and `outdated_in` (at most one) ask
         *     Localization which messages have no translation in a locale, or
         *     one made against an older source revision; they also need
         *     `translations.read` and see a new message once its event has been
         *     processed (usually within a second). Needs `catalog.read`.
         *     Problem codes: `coverage_filter_conflict` (400).
         */
        get: operations["listMessages"];
        put?: never;
        /**
         * Create a message
         * @description The source is parsed (ICU MF1 unless `syntax` or the project's
         *     default says MF2) into the canonical MF2 model; revision 1 starts
         *     the message's source log. Needs `catalog.write`. Problem codes:
         *     `key_taken` (409), `invalid_message` (400, the MessageFormat
         *     error code in `detail`), `invalid_message_key`,
         *     `invalid_namespace`, `invalid_details`, `invalid_syntax`,
         *     `message_too_long` (400).
         */
        post: operations["createMessage"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/message-upserts": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Create or revise messages in bulk (CLI push)
         * @description Up to 500 messages by key, in one transaction. Idempotent: a
         *     message whose source parses to the stored model is `unchanged`,
         *     so pushing the same catalog twice changes nothing. An obsolete
         *     message pushed again is reactivated. Items fail on their own
         *     (`results[].error.code`: `invalid_message_key`,
         *     `invalid_message`, `invalid_namespace`, `invalid_details`,
         *     `invalid_syntax`, `message_too_long`, `duplicate_key`,
         *     `source_revision_conflict` when `base_revision` is stale) without
         *     failing the batch. Needs `catalog.write`. Problem codes:
         *     `too_many_items` (400).
         */
        post: operations["upsertMessages"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/messages/{message}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A message `key` (`checkout.pay`). Keys are URL-safe as they are. */
                message: components["parameters"]["MessagePath"];
            };
            cookie?: never;
        };
        /**
         * A message
         * @description Needs `catalog.read`.
         */
        get: operations["getMessage"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        /**
         * Change a message's namespace, description or length limit
         * @description Not a source revision. `max_length: 0` removes the limit. Needs
         *     `catalog.write`.
         */
        patch: operations["updateMessage"];
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/messages/{message}/source": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A message `key` (`checkout.pay`). Keys are URL-safe as they are. */
                message: components["parameters"]["MessagePath"];
            };
            cookie?: never;
        };
        get?: never;
        /**
         * Revise a message's source text
         * @description Appends a source revision and publishes
         *     `catalog.message.source_revised`, which makes existing
         *     translations outdated. Text that parses to the current model is
         *     no revision (the message comes back unchanged). The key and ID
         *     never change. Needs `catalog.write`. Problem codes:
         *     `invalid_message` (400), `message_obsolete` (409).
         */
        put: operations["reviseMessageSource"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/messages/{message}/obsoletion": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A message `key` (`checkout.pay`). Keys are URL-safe as they are. */
                message: components["parameters"]["MessagePath"];
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Retire a message
         * @description The message keeps its history and translations but is no longer
         *     released. Pushing its key again reactivates it. Idempotent. Needs
         *     `catalog.write`. `If-Match` is optional; when sent it must match.
         */
        post: operations["obsoleteMessage"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/messages/{message}/renames": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A message `key` (`checkout.pay`). Keys are URL-safe as they are. */
                message: components["parameters"]["MessagePath"];
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Give a message a new key
         * @description The explicit way to change a message's identity as runtimes see
         *     it: the ID, source log and translations stay with the message.
         *     `Location` points at the new key. Needs `catalog.write`. `If-Match`
         *     is optional; when sent it must match. Problem codes: `key_taken`
         *     (409), `invalid_message_key` (400).
         */
        post: operations["renameMessage"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/messages/{message}/source-revisions": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A message `key` (`checkout.pay`). Keys are URL-safe as they are. */
                message: components["parameters"]["MessagePath"];
            };
            cookie?: never;
        };
        /**
         * A message's source log, newest first
         * @description Needs `catalog.read`.
         */
        get: operations["listSourceRevisions"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/locales": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        /**
         * A project's locales, the source locale included
         * @description Needs `translations.read`.
         */
        get: operations["listLocales"];
        put?: never;
        /**
         * Add a locale to a project
         * @description The code is canonicalized (`de_de` → `de-DE`); extensions and
         *     private use are refused. Adding an existing locale returns it
         *     with `200`. Needs `catalog.write`. Problem codes: `invalid_locale`
         *     (400).
         */
        post: operations["addLocale"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/locales/{locale}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A locale code; canonicalized before use. */
                locale: components["parameters"]["LocalePath"];
            };
            cookie?: never;
        };
        /**
         * A locale of a project
         * @description Needs `translations.read`. Problem codes: `locale_not_found` (404).
         */
        get: operations["getLocale"];
        put?: never;
        post?: never;
        /**
         * Remove a locale from a project
         * @description Its translations and their history are kept and come back if the
         *     locale is added again. Needs `catalog.write`. Problem codes:
         *     `source_locale` (409), `locale_in_fallback` (409, remove it from
         *     the fallback graph first), `locale_not_found` (404).
         */
        delete: operations["removeLocale"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/fallback-graph": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        /**
         * A project's fallback graph
         * @description Exactly the `fallback` member releases publish
         *     (runtimes/SPEC.md §1.1). Empty until set. Needs
         *     `translations.read`.
         */
        get: operations["getFallbackGraph"];
        /**
         * Replace a project's fallback graph
         * @description Keys are project locales or `*` (every locale without its own
         *     entry); chains name project locales, in order. Cycles are
         *     refused. The first write takes no `If-Match`; later ones need the
         *     current `ETag` (`428` without it). Needs `catalog.write`. Problem
         *     codes (400): `fallback_unknown_locale`, `fallback_self_reference`,
         *     `fallback_duplicate`, `fallback_cycle`, `fallback_invalid_locale`.
         */
        put: operations["putFallbackGraph"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/messages/{message}/translations": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A message `key` (`checkout.pay`). Keys are URL-safe as they are. */
                message: components["parameters"]["MessagePath"];
            };
            cookie?: never;
        };
        /**
         * A message's translations, by locale
         * @description Needs `translations.read`.
         */
        get: operations["listMessageTranslations"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/messages/{message}/translations/{locale}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A message `key` (`checkout.pay`). Keys are URL-safe as they are. */
                message: components["parameters"]["MessagePath"];
                /** @description A locale code; canonicalized before use. */
                locale: components["parameters"]["LocalePath"];
            };
            cookie?: never;
        };
        /**
         * A message's translation in a locale
         * @description Needs `translations.read`.
         */
        get: operations["getTranslation"];
        /**
         * Write a message's translation in a locale
         * @description Parses the text for the locale, checks it structurally against
         *     the source revision it was made from (`source_revision`, default
         *     the current one) and appends a revision with provenance
         *     (`origin`, default `human`, and `origin_detail`). Error findings
         *     reject the write with `422 structural_qa_failed` and the
         *     `findings`; warnings are stored and returned. New text gets the
         *     project's review policy (`needs_review` when review is required,
         *     else `approved`) unless `state` asks otherwise; approving needs
         *     `translations.review` for the locale. Unchanged text is no new
         *     revision. Creating takes no `If-Match`; changing needs the
         *     current `ETag` (`428` without it). Needs `translations.write` for
         *     the locale. Problem codes: `locale_not_found` (404),
         *     `source_locale` (409), `invalid_message`, `invalid_state`,
         *     `invalid_origin`, `invalid_origin_detail`,
         *     `invalid_source_revision`, `write_cannot_reject` (400),
         *     `review_forbidden` (403), `translation_conflict` (409).
         */
        put: operations["putTranslation"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/messages/{message}/translations/{locale}/revisions": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A message `key` (`checkout.pay`). Keys are URL-safe as they are. */
                message: components["parameters"]["MessagePath"];
                /** @description A locale code; canonicalized before use. */
                locale: components["parameters"]["LocalePath"];
            };
            cookie?: never;
        };
        /**
         * A translation's history with provenance, newest first
         * @description Needs `translations.read`.
         */
        get: operations["listTranslationRevisions"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/messages/{message}/translations/{locale}/reviews": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A message `key` (`checkout.pay`). Keys are URL-safe as they are. */
                message: components["parameters"]["MessagePath"];
                /** @description A locale code; canonicalized before use. */
                locale: components["parameters"]["LocalePath"];
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Record a review decision
         * @description Moves the translation to `state` and appends a `review` revision
         *     naming the reviewer; the text and its provenance stay. Approving
         *     and rejecting need `translations.review` for the locale, other
         *     states `translations.write`. Problem codes: `review_forbidden`
         *     (403), `invalid_transition` (409), `invalid_state` (400).
         */
        post: operations["reviewTranslation"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/translations": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        /**
         * A project's translations in one or more locales, by key
         * @description Every translation in the given locales (`locale`, repeatable, 1
         *     to 20), across messages, ordered by message key and then locale,
         *     with the message's `key`, `namespace` and `message_state`, the
         *     `source_revision` it was made against and the derived `outdated`.
         *     Filters combine: `state` (repeatable review states), `outdated`,
         *     `namespace`, `key_prefix` and `message_state`. Locales the
         *     project no longer has list nothing. Keys and namespaces come
         *     from Localization's view of the catalog, current once Catalog's
         *     events are processed (usually within a second). One query per
         *     page. Needs `translations.read`. Problem codes: `invalid_locale`,
         *     `too_many_locales`, `invalid_state`, `invalid_message_state`
         *     (400).
         */
        get: operations["listProjectTranslations"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/translation-stats": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        /**
         * Per-locale status of a project's translations
         * @description For every locale of the project (the source included, by code):
         *     how many of its active messages are translated (a usable — not
         *     rejected — translation exists), missing (none, or rejected) and
         *     outdated (usable but made against an older source revision), and
         *     its translations of active messages per review state. The source
         *     locale counts every active message as translated and approved.
         *     Computed in one query on each request, from Localization's view
         *     of the catalog. Needs `translations.read`.
         */
        get: operations["getTranslationStats"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/translation-imports": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Import translations in bulk
         * @description Up to 500 translations by message key and locale, written with
         *     provenance `import` (put the job and source system in
         *     `origin_detail`) in one transaction. Re-importing unchanged text
         *     is `unchanged`. Items fail on their own (`results[].error.code`:
         *     `message_not_found`, `locale_not_found`, `invalid_locale`,
         *     `forbidden` for a locale outside the caller's scope,
         *     `source_locale`, `structural_qa_failed` with `findings`,
         *     `invalid_message`, `review_forbidden`, `duplicate_item`, …)
         *     without failing the batch. Needs `translations.write` per
         *     locale. Problem codes: `too_many_items` (400).
         */
        post: operations["importTranslations"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/environments": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        /**
         * Environments of a project
         * @description By name. Every project has `development`, `preview`, `staging`
         *     and `production`; they are created on first use. Needs
         *     `releases.read`.
         */
        get: operations["listEnvironments"];
        put?: never;
        /**
         * Add a custom environment
         * @description A branch preview, a QA stage. Without `policy` it ships
         *     everything not rejected. Needs `releases.publish`. Problem codes:
         *     `environment_exists` (409), `invalid_environment`,
         *     `invalid_policy` (400).
         */
        post: operations["createEnvironment"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/environments/{environment}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description An environment `name`. */
                environment: components["parameters"]["EnvironmentPath"];
            };
            cookie?: never;
        };
        /**
         * An environment, its policy and the release it serves
         * @description Needs `releases.read`.
         */
        get: operations["getEnvironment"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        /**
         * Change an environment's eligibility policy
         * @description The policy decides what the next publish ships and which
         *     releases may be promoted here; the release served keeps serving.
         *     Needs `releases.publish`. Problem codes: `invalid_policy` (400).
         */
        patch: operations["updateEnvironment"];
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/environments/{environment}/promotions": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description An environment `name`. */
                environment: components["parameters"]["EnvironmentPath"];
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Point the environment at an existing release
         * @description Moves the pointer; nothing is rebuilt. The environment's policy
         *     must cover the policy the release was built under, so a
         *     development or preview release (which ships drafts) can't reach
         *     production; the default path is to publish to `staging` (approved
         *     text, like production) and promote that release to `production`.
         *     `release_ineligible`'s detail names both policies and what
         *     differs. Promoting the release already served changes nothing,
         *     so a retry is safe. Needs `releases.publish`. Problem codes:
         *     `release_not_found` (404), `release_ineligible` (409),
         *     `storage_unavailable` (503).
         */
        post: operations["promoteRelease"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/environments/{environment}/release-previews": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description An environment `name`. */
                environment: components["parameters"]["EnvironmentPath"];
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * What publishing to the environment would ship now (dry run)
         * @description Runs the build `publishRelease` runs, under the environment's
         *     policy, and stores nothing: no release, no artifacts, no
         *     deployment, no events (a default environment the project hasn't
         *     used yet is shown as it will be created). Returns per-locale
         *     message counts, the per-locale message IDs added, changed and
         *     removed compared with the release the environment serves now,
         *     and `new_artifacts`, what the publish would upload. A catalog
         *     that can't be released is a `200` with `releasable: false` and
         *     every `not_releasable` problem found (publishing would fail with
         *     `422 not_releasable`). The answer is only as current as the
         *     request: a translation saved in between changes what a publish
         *     ships. Needs `releases.read`. Problem codes:
         *     `storage_unavailable` (503).
         */
        post: operations["previewRelease"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/environments/{environment}/rollbacks": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description An environment `name`. */
                environment: components["parameters"]["EnvironmentPath"];
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Point the environment back at a release it served before
         * @description Without `release_id`, the newest release the environment served
         *     that is older than the current one: rolling back twice goes two
         *     steps back, so send `release_id` when a retry must not. Moves
         *     the pointer only. Needs `releases.publish`. Problem codes:
         *     `release_not_found` (404), `no_rollback_target`,
         *     `not_in_history` (409).
         */
        post: operations["rollbackEnvironment"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/environments/{environment}/deployments": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description An environment `name`. */
                environment: components["parameters"]["EnvironmentPath"];
            };
            cookie?: never;
        };
        /**
         * The environment's history of pointer moves
         * @description Newest first. Needs `releases.read`.
         */
        get: operations["listDeployments"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/releases": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        /**
         * Releases of a project
         * @description Newest first. Needs `releases.read`.
         */
        get: operations["listReleases"];
        put?: never;
        /**
         * Publish a release to an environment
         * @description Builds one artifact per locale and namespace from the active
         *     messages and the translations the environment's policy makes
         *     eligible, stores the ones storage doesn't have yet, records the
         *     immutable release and points the environment at it; the edge
         *     serves it within seconds. Publishing an unchanged catalog
         *     uploads nothing. Needs `releases.publish`. Problem codes:
         *     `invalid_environment`, `invalid_note` (400),
         *     `not_releasable` (422), `storage_unavailable` (503).
         */
        post: operations["publishRelease"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/releases/{release}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A release `id`. */
                release: components["parameters"]["ReleasePath"];
            };
            cookie?: never;
        };
        /**
         * A release, its counts and manifest digest
         * @description Needs `releases.read`.
         */
        get: operations["getRelease"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/releases/{release}/diff": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A release `id`. */
                release: components["parameters"]["ReleasePath"];
            };
            cookie?: never;
        };
        /**
         * What changed since another release, per locale
         * @description Message IDs added, changed and removed in each locale, compared
         *     with `base` (default: the release's parent, what its environment
         *     served before it; without one, everything is added). Needs
         *     `releases.read`. Problem codes: `release_not_found` (404),
         *     `storage_unavailable` (503).
         */
        get: operations["getReleaseDiff"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/releases/{release}/manifest": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A release `id`. */
                release: components["parameters"]["ReleasePath"];
            };
            cookie?: never;
        };
        /**
         * The signed manifest of a release for an environment
         * @description Byte for byte what glossa-edge serves when `environment` serves
         *     this release (runtimes/SPEC.md §1.1): what `glossa pull
         *     --release` writes as a bundle's `manifest.json`. Needs
         *     `releases.read`.
         */
        get: operations["getReleaseManifest"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/releases/{release}/artifacts/{digest}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A release `id`. */
                release: components["parameters"]["ReleasePath"];
                /** @description The artifact's SHA-256, as the manifest names it. */
                digest: string;
            };
            cookie?: never;
        };
        /**
         * One artifact of a release, as stored
         * @description The exact bytes the manifest's `sha256` covers
         *     (runtimes/SPEC.md §1.2), for bundling
         *     (`glossa pull --release` writes `a/<sha256>.json`). Needs
         *     `releases.read`. Problem codes: `storage_unavailable` (503).
         */
        get: operations["getReleaseArtifact"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/release-signing-keys": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        /**
         * The public keys manifests are signed with
         * @description Configure runtimes with these (runtimes/SPEC.md §1.3). Active
         *     keys sign every new manifest; retired ones are still listed
         *     while runtimes may hold manifests only they signed. Rotation:
         *     a new key is added (manifests carry both signatures), runtimes
         *     move to it, the old one is retired. Needs `releases.read`.
         */
        get: operations["listReleaseSigningKeys"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/delivery-keys": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        /**
         * Publishable delivery keys, including revoked ones
         * @description Needs `releases.read`.
         */
        get: operations["listDeliveryKeys"];
        put?: never;
        /**
         * Create a publishable delivery key
         * @description The key runtimes fetch releases from glossa-edge with
         *     (`/v1/{key}/{environment}/manifest.json`). It is public by
         *     design — it ships in browser bundles — scoped to this project,
         *     read-only, and grants nothing on this API. Needs
         *     `releases.publish`. Problem codes: `invalid_key_name` (400).
         */
        post: operations["createDeliveryKey"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/v1/tenants/{tenant}/projects/{project}/delivery-keys/{delivery_key}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A delivery key `id` (not the key itself). */
                delivery_key: components["parameters"]["DeliveryKeyPath"];
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        post?: never;
        /**
         * Revoke a delivery key
         * @description The edge answers 404 for it within its key cache TTL (30 s by
         *     default) plus any CDN max-age. It stays listed as revoked.
         *     Needs `releases.publish`. Problem codes: `key_revoked` (409).
         */
        delete: operations["revokeDeliveryKey"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
}
export type webhooks = Record<string, never>;
export interface components {
    schemas: {
        /** @description An opaque identifier. */
        Id: string;
        /**
         * Format: date-time
         * @description RFC 3339, UTC.
         */
        Timestamp: string;
        /** Format: email */
        Email: string;
        /**
         * @description A BCP 47 language tag. Stored and returned canonicalized
         *     (`en_us` → `en-US`, `iw` → `he`).
         * @example de
         * @example pt-BR
         * @example zh-Hant-TW
         */
        Locale: string;
        /**
         * @description `owner` everything; `admin` everything except owner changes;
         *     `developer` catalog, translations, releases, tokens;
         *     `translator` translates; `reviewer` translates and reviews.
         *     Translators and reviewers can be limited to `locales`.
         * @enum {string}
         */
        Role: "owner" | "admin" | "developer" | "translator" | "reviewer";
        /**
         * @description `read` reads the tenant; `write` pushes messages and
         *     translations; `publish` creates releases; `admin` manages the
         *     tenant, members and tokens (never owners). Every scope implies
         *     `read`.
         * @enum {string}
         */
        Scope: "read" | "write" | "publish" | "admin";
        /** @description RFC 9457 problem details. */
        Problem: {
            /**
             * Format: uri
             * @example urn:glossa:problem:slug_taken
             */
            type: string;
            title: string;
            status: number;
            /** @description Stable machine-readable code. */
            code: string;
            detail?: string;
            instance?: string;
            errors?: components["schemas"]["FieldError"][];
        };
        FieldError: {
            /** @description JSON Pointer into the request body. */
            pointer: string;
            detail: string;
        };
        Meta: {
            /**
             * @description How people can sign in here, strongest first: `passkey` when
             *     a WebAuthn relying party is configured, `password` always,
             *     `magic_link` when the server sends email.
             */
            sign_in_methods: ("passkey" | "password" | "magic_link")[];
            /**
             * @description Whether the server sends email. Without it magic links,
             *     email verification and password reset by email are
             *     unavailable (`email_disabled`), a password account works
             *     without a verified address, and invitations wait: only a
             *     verified address accepts one.
             */
            email_delivery: boolean;
            /**
             * Format: uri
             * @description glossa-edge's public base URL, what runtimes are configured
             *     with (`{edge_url}/v1/{key}/{environment}/manifest.json`).
             *     Absent when the deployment doesn't announce it
             *     (`GLOSSA_EDGE_PUBLIC_URL`).
             */
            edge_url?: string;
        };
        EmailRequest: {
            email: components["schemas"]["Email"];
        };
        TokenRedemption: {
            /** @description The token from the emailed link. */
            token: string;
        };
        Registration: {
            email: components["schemas"]["Email"];
            password: string;
            display_name?: string;
        };
        PasswordSignIn: {
            email: components["schemas"]["Email"];
            password: string;
            totp_code?: string;
        };
        PasswordReset: {
            token: string;
            password: string;
        };
        /** @description The `PublicKeyCredential` from the browser, serialized as JSON. */
        WebAuthnResponse: {
            [key: string]: unknown;
        };
        PasskeyRegistration: {
            /** @description A label such as "MacBook Touch ID". */
            name?: string;
            credential: components["schemas"]["WebAuthnResponse"];
        };
        PasskeyOptions: {
            /** @description WebAuthn options to pass to `navigator.credentials`. */
            options: {
                [key: string]: unknown;
            };
        };
        Passkey: {
            /** @description The credential ID, base64url. */
            id: string;
            name: string;
            created_at: components["schemas"]["Timestamp"];
            /** @description The last sign-in with it; absent until it signs in. */
            last_used_at?: components["schemas"]["Timestamp"];
        };
        PasskeyList: {
            items: components["schemas"]["Passkey"][];
            next_page_token?: string;
        };
        TotpEnrollment: {
            /** @description Base32 secret, for manual entry. */
            secret: string;
            /** @description `otpauth://` URI, for a QR code. */
            otpauth_uri: string;
        };
        TotpCode: {
            code: string;
        };
        Person: {
            id: components["schemas"]["Id"];
            email: components["schemas"]["Email"];
            display_name?: string;
            email_verified: boolean;
            totp_enabled: boolean;
            individual_tenant_id: components["schemas"]["Id"];
            created_at: components["schemas"]["Timestamp"];
        };
        Session: {
            person: components["schemas"]["Person"];
            /** @description Send as `X-CSRF-Token` on unsafe requests. */
            csrf_token: string;
            expires_at: components["schemas"]["Timestamp"];
        };
        Me: {
            person: components["schemas"]["Person"];
            csrf_token: string;
            memberships: components["schemas"]["Membership"][];
        };
        Membership: {
            member_id: components["schemas"]["Id"];
            tenant: components["schemas"]["Tenant"];
            roles: components["schemas"]["Role"][];
            locales: components["schemas"]["Locale"][];
        };
        Tenant: {
            id: components["schemas"]["Id"];
            /** @enum {string} */
            kind: "individual" | "organization";
            slug: string;
            name: string;
            created_at: components["schemas"]["Timestamp"];
        };
        TenantList: {
            items: components["schemas"]["Tenant"][];
            next_page_token?: string;
        };
        CreateTenant: {
            slug: string;
            name: string;
        };
        Member: {
            id: components["schemas"]["Id"];
            email: components["schemas"]["Email"];
            /** @description Set once the invitation is accepted. */
            person_id?: components["schemas"]["Id"];
            display_name?: string;
            /** @enum {string} */
            status: "invited" | "active";
            roles: components["schemas"]["Role"][];
            /** @description Locales a translator or reviewer works on; empty means all. */
            locales: components["schemas"]["Locale"][];
            created_at: components["schemas"]["Timestamp"];
            updated_at: components["schemas"]["Timestamp"];
        };
        MemberList: {
            items: components["schemas"]["Member"][];
            next_page_token?: string;
        };
        AddMember: {
            email: components["schemas"]["Email"];
            roles: components["schemas"]["Role"][];
            locales?: components["schemas"]["Locale"][];
        };
        UpdateMember: {
            roles?: components["schemas"]["Role"][];
            locales?: components["schemas"]["Locale"][];
        };
        Token: {
            id: components["schemas"]["Id"];
            name: string;
            /** @description The first characters of the secret, e.g. `glossa_api_Ab3x`. */
            hint: string;
            scopes: components["schemas"]["Scope"][];
            /** @description `person:<id>` or `token:<id>`. */
            created_by: string;
            created_at: components["schemas"]["Timestamp"];
            expires_at?: components["schemas"]["Timestamp"];
            last_used_at?: components["schemas"]["Timestamp"];
            revoked_at?: components["schemas"]["Timestamp"];
        };
        TokenList: {
            items: components["schemas"]["Token"][];
            next_page_token?: string;
        };
        CreateToken: {
            name: string;
            scopes: components["schemas"]["Scope"][];
            expires_at?: components["schemas"]["Timestamp"];
        };
        CreatedToken: {
            token: components["schemas"]["Token"];
            /** @description The token value (`glossa_api_…`). Shown only in the first response. */
            secret?: string;
        };
        Slug: string;
        /**
         * @description Authoring syntax: ICU MessageFormat 1 or Unicode MessageFormat 2.
         * @enum {string}
         */
        Syntax: "mf1" | "mf2";
        ProjectSettings: {
            default_syntax: components["schemas"]["Syntax"];
            /**
             * @description New translations wait for review (`needs_review`) unless a
             *     reviewer approves them as they write. When false, new text is
             *     `approved` on write.
             */
            review_required: boolean;
        };
        Project: {
            id: components["schemas"]["Id"];
            slug: components["schemas"]["Slug"];
            name: string;
            source_locale: components["schemas"]["Locale"];
            settings: components["schemas"]["ProjectSettings"];
            created_at: components["schemas"]["Timestamp"];
            updated_at: components["schemas"]["Timestamp"];
        };
        ProjectList: {
            items: components["schemas"]["Project"][];
            next_page_token?: string;
        };
        CreateProject: {
            slug: components["schemas"]["Slug"];
            name: string;
            source_locale: components["schemas"]["Locale"];
            settings?: components["schemas"]["ProjectSettings"];
        };
        UpdateProject: {
            slug?: components["schemas"]["Slug"];
            name?: string;
            settings?: components["schemas"]["ProjectSettings"];
        };
        /** @enum {string} */
        Platform: "web" | "api" | "ios" | "android" | "other";
        Application: {
            id: components["schemas"]["Id"];
            slug: components["schemas"]["Slug"];
            name: string;
            platform: components["schemas"]["Platform"];
            created_at: components["schemas"]["Timestamp"];
            updated_at: components["schemas"]["Timestamp"];
        };
        ApplicationList: {
            items: components["schemas"]["Application"][];
            next_page_token?: string;
        };
        CreateApplication: {
            slug: components["schemas"]["Slug"];
            name: string;
            platform: components["schemas"]["Platform"];
        };
        UpdateApplication: {
            slug?: components["schemas"]["Slug"];
            name?: string;
            platform?: components["schemas"]["Platform"];
        };
        /**
         * @description A dotted path of `[a-z0-9_-]` segments, unique in the project.
         * @example checkout.payment.submit
         */
        MessageKey: string;
        /** @description Groups messages into separately loadable bundles. Default `default`. */
        Namespace: string;
        /** @enum {string} */
        MessageState: "active" | "obsolete";
        /**
         * @description A message in the Unicode MessageFormat 2 data model, exactly as
         *     messageformat/testdata/unicode/data-model/message.schema.json
         *     defines it — the canonical form releases ship.
         */
        MF2Message: {
            [key: string]: unknown;
        };
        Argument: {
            name: string;
            /** @enum {string} */
            type: "string" | "number" | "integer" | "percent" | "currency" | "date" | "time" | "datetime" | "unit" | "select";
            /** @description The annotating function, if any. */
            function?: string;
            selector?: {
                /** @enum {string} */
                kind: "plural" | "ordinal" | "exact" | "string";
                /** @description Literal variant keys; the catch-all `*` always exists. */
                keys: string[];
            };
        };
        MarkupElement: {
            name: string;
            /** @enum {string} */
            kind: "open" | "standalone" | "close";
        };
        MessageContent: {
            /** @description What the author wrote. */
            text: string;
            syntax: components["schemas"]["Syntax"];
            model: components["schemas"]["MF2Message"];
            /** @description Derived from the model on every change. */
            arguments: components["schemas"]["Argument"][];
            markup: components["schemas"]["MarkupElement"][];
        };
        Message: {
            id: components["schemas"]["Id"];
            key: components["schemas"]["MessageKey"];
            namespace: components["schemas"]["Namespace"];
            description: string;
            /** @description Rendered length limit that translation QA checks. */
            max_length?: number;
            state: components["schemas"]["MessageState"];
            source: components["schemas"]["MessageContent"];
            /** @description The current source revision. */
            source_revision: number;
            created_at: components["schemas"]["Timestamp"];
            updated_at: components["schemas"]["Timestamp"];
        };
        MessageList: {
            items: components["schemas"]["Message"][];
            next_page_token?: string;
        };
        CreateMessage: {
            key: components["schemas"]["MessageKey"];
            text: string;
            syntax?: components["schemas"]["Syntax"];
            namespace?: components["schemas"]["Namespace"];
            description?: string;
            max_length?: number;
        };
        UpdateMessage: {
            namespace?: components["schemas"]["Namespace"];
            description?: string;
            /** @description `0` removes the limit. */
            max_length?: number;
        };
        ReviseSource: {
            text: string;
            syntax?: components["schemas"]["Syntax"];
        };
        RenameMessage: {
            key: components["schemas"]["MessageKey"];
        };
        SourceRevision: {
            revision: number;
            text: string;
            syntax: components["schemas"]["Syntax"];
            model: components["schemas"]["MF2Message"];
            /** @description `person:<id>` or `token:<id>`. */
            author: string;
            created_at: components["schemas"]["Timestamp"];
        };
        SourceRevisionList: {
            items: components["schemas"]["SourceRevision"][];
            next_page_token?: string;
        };
        MessagePreviewRequest: {
            /** @description The message as authored. */
            source: string;
            syntax?: components["schemas"]["Syntax"];
            /** @description Decides which plural keys MF1 accepts, and formatting. */
            locale: components["schemas"]["Locale"];
            /**
             * @description Argument values to format with, by name (without `$`): strings,
             *     numbers or booleans. Omit to only parse.
             */
            values?: {
                [key: string]: unknown;
            };
            /** @description Isolate placeholders with Unicode bidi marks (the MF2 default, `true`). */
            bidi_isolation?: boolean;
        };
        MessagePreviewError: {
            /** @enum {string} */
            stage: "parse" | "format";
            /** @description The MessageFormat kernel's stable error code: `mf1-syntax-error`, `syntax-error`, `unresolved-variable`, … */
            code: string;
            /** @description For humans; wording may change. */
            message: string;
        };
        MessagePreview: {
            /** @description Whether the source parsed into a valid message. */
            valid: boolean;
            message?: components["schemas"]["MF2Message"];
            /** @description The canonical model in MF2 syntax. */
            mf2?: string;
            arguments: components["schemas"]["Argument"][];
            markup: components["schemas"]["MarkupElement"][];
            /** @description The message formatted with `values`, when they were sent and it is valid. */
            formatted?: string;
            errors: components["schemas"]["MessagePreviewError"][];
        };
        ItemError: {
            /** @description Stable machine code. */
            code: string;
            detail: string;
            findings?: components["schemas"]["QAFinding"][];
        };
        MessageUpsertItem: {
            key: string;
            text: string;
            syntax?: components["schemas"]["Syntax"];
            /** @description Omitted: keep (new messages: `default`). */
            namespace?: string;
            /** @description Omitted: keep. */
            description?: string;
            /** @description Omitted: keep. */
            max_length?: number;
            /**
             * @description The source revision the client last saw (0: the message must
             *     not exist yet). A stale one fails the item with
             *     `source_revision_conflict` instead of overwriting.
             */
            base_revision?: number;
        };
        MessageUpsert: {
            items: components["schemas"]["MessageUpsertItem"][];
        };
        MessageUpsertItemResult: {
            key: string;
            /** @enum {string} */
            status: "created" | "revised" | "updated" | "unchanged" | "failed";
            message?: components["schemas"]["Message"];
            error?: components["schemas"]["ItemError"];
        };
        MessageUpsertResult: {
            results: components["schemas"]["MessageUpsertItemResult"][];
        };
        /**
         * @description Derived from the locale's (likely) script.
         * @enum {string}
         */
        Direction: "ltr" | "rtl";
        ProjectLocale: {
            code: components["schemas"]["Locale"];
            direction: components["schemas"]["Direction"];
            is_source: boolean;
            created_at: components["schemas"]["Timestamp"];
        };
        LocaleList: {
            items: components["schemas"]["ProjectLocale"][];
            next_page_token?: string;
        };
        AddLocale: {
            code: components["schemas"]["Locale"];
        };
        FallbackGraph: {
            /** @description A locale (or `*`) to its ordered fallback locales: `{"de-AT": ["de"], "*": ["en"]}`. */
            fallback: {
                [key: string]: components["schemas"]["Locale"][];
            };
        };
        PutFallbackGraph: {
            fallback: {
                [key: string]: components["schemas"]["Locale"][];
            };
        };
        /** @enum {string} */
        ReviewState: "draft" | "needs_review" | "approved" | "rejected";
        /** @enum {string} */
        Origin: "human" | "ai" | "translation_memory" | "machine_translation" | "import" | "adaptation";
        QAFinding: {
            /** @description Stable finding code, e.g. `missing-argument`, `max-length-exceeded`. */
            code: string;
            /** @enum {string} */
            severity: "error" | "warning";
            /** @description The argument or markup element concerned. */
            subject?: string;
            detail?: string;
            /** @description For humans; wording may change. */
            message: string;
        };
        QAProblem: components["schemas"]["Problem"] & {
            findings?: components["schemas"]["QAFinding"][];
        };
        Translation: {
            id: components["schemas"]["Id"];
            message_id: components["schemas"]["Id"];
            locale: components["schemas"]["Locale"];
            text: string;
            syntax: components["schemas"]["Syntax"];
            model: components["schemas"]["MF2Message"];
            state: components["schemas"]["ReviewState"];
            origin: components["schemas"]["Origin"];
            /** @description Who wrote the current text. */
            author: string;
            /** @description The source revision the text was made against. */
            source_revision: number;
            /** @description The message's current source revision as Localization knows it. */
            current_source_revision: number;
            /** @description `source_revision < current_source_revision`. */
            outdated: boolean;
            warnings: components["schemas"]["QAFinding"][];
            revision: number;
            created_at: components["schemas"]["Timestamp"];
            updated_at: components["schemas"]["Timestamp"];
        };
        TranslationList: {
            items: components["schemas"]["Translation"][];
            next_page_token?: string;
        };
        /** @description A translation with the message it belongs to. */
        ProjectTranslation: components["schemas"]["Translation"] & {
            key: components["schemas"]["MessageKey"];
            namespace: components["schemas"]["Namespace"];
            message_state: components["schemas"]["MessageState"];
        };
        ProjectTranslationList: {
            items: components["schemas"]["ProjectTranslation"][];
            next_page_token?: string;
        };
        ReviewStateCounts: {
            draft: number;
            needs_review: number;
            approved: number;
            rejected: number;
        };
        LocaleStats: {
            code: components["schemas"]["Locale"];
            direction: components["schemas"]["Direction"];
            is_source: boolean;
            /** @description Active messages with a usable (not rejected) translation. */
            translated: number;
            /** @description Active messages without one: untranslated or rejected. */
            missing: number;
            /** @description Usable translations made against an older source revision. */
            outdated: number;
            /** @description Translations of active messages per review state. */
            states: components["schemas"]["ReviewStateCounts"];
        };
        TranslationStats: {
            /** @description The project's active messages. */
            messages: number;
            locales: components["schemas"]["LocaleStats"][];
        };
        PutTranslation: {
            text: string;
            syntax?: components["schemas"]["Syntax"];
            state?: components["schemas"]["ReviewState"];
            origin?: components["schemas"]["Origin"];
            /** @description Provenance specifics — model, prompt version, TM match, source job… */
            origin_detail?: {
                [key: string]: unknown;
            };
            source_revision?: number;
        };
        ReviewTranslation: {
            state: components["schemas"]["ReviewState"];
        };
        TranslationRevision: {
            revision: number;
            /** @enum {string} */
            kind: "content" | "review";
            text: string;
            syntax: components["schemas"]["Syntax"];
            state: components["schemas"]["ReviewState"];
            origin: components["schemas"]["Origin"];
            origin_detail: {
                [key: string]: unknown;
            };
            author: string;
            source_revision: number;
            findings: components["schemas"]["QAFinding"][];
            created_at: components["schemas"]["Timestamp"];
        };
        TranslationRevisionList: {
            items: components["schemas"]["TranslationRevision"][];
            next_page_token?: string;
        };
        TranslationImportItem: {
            key: string;
            locale: string;
            text: string;
            syntax?: components["schemas"]["Syntax"];
            state?: components["schemas"]["ReviewState"];
            origin_detail?: {
                [key: string]: unknown;
            };
            source_revision?: number;
        };
        TranslationImport: {
            items: components["schemas"]["TranslationImportItem"][];
        };
        TranslationImportItemResult: {
            key: string;
            locale: string;
            /** @enum {string} */
            status?: "created" | "revised" | "reviewed" | "unchanged";
            translation?: components["schemas"]["Translation"];
            error?: components["schemas"]["ItemError"];
        };
        TranslationImportResult: {
            results: components["schemas"]["TranslationImportItemResult"][];
        };
        /**
         * @description Which translations ship to an environment: those in `states`
         *     (never `rejected`), and outdated ones (made against an older
         *     source revision) only with `include_outdated`. Production and
         *     staging start with `approved`, the others with everything.
         */
        EnvironmentPolicy: {
            states: ("draft" | "needs_review" | "approved")[];
            include_outdated: boolean;
        };
        Environment: {
            name: components["schemas"]["EnvironmentName"];
            policy: components["schemas"]["EnvironmentPolicy"];
            current_release_id?: components["schemas"]["Id"];
            created_at: components["schemas"]["Timestamp"];
            updated_at: components["schemas"]["Timestamp"];
        };
        /** @description `development`, `preview`, `staging`, `production` or a custom name (not `a`). */
        EnvironmentName: string;
        EnvironmentList: {
            items: components["schemas"]["Environment"][];
            next_page_token?: string;
        };
        CreateEnvironment: {
            name: components["schemas"]["EnvironmentName"];
            policy?: components["schemas"]["EnvironmentPolicy"];
        };
        UpdateEnvironment: {
            policy: components["schemas"]["EnvironmentPolicy"];
        };
        Promotion: {
            release_id: components["schemas"]["Id"];
        };
        Rollback: {
            release_id?: components["schemas"]["Id"];
        };
        Deployment: {
            /** @description Counts the environment's deployments from 1. */
            number: number;
            release_id: components["schemas"]["Id"];
            previous_release_id?: components["schemas"]["Id"];
            /** @enum {string} */
            action: "publish" | "promote" | "rollback";
            /** @description `person:<id>` or `token:<id>`. */
            author: string;
            created_at: components["schemas"]["Timestamp"];
        };
        DeploymentList: {
            items: components["schemas"]["Deployment"][];
            next_page_token?: string;
        };
        PublishRelease: {
            environment: components["schemas"]["EnvironmentName"];
            note?: string;
        };
        ReleaseLocale: {
            code: components["schemas"]["Locale"];
            direction: components["schemas"]["Direction"];
        };
        ReleaseLocaleCounts: {
            messages: number;
            /** @description Outdated translations shipped (the policy allowed them). */
            outdated: number;
        };
        ReleaseCounts: {
            /** @description Source messages. */
            messages: number;
            artifacts: number;
            bytes: number;
            /** @description Artifacts this publish uploaded; the rest were already stored. */
            new_artifacts: number;
            /** @description Per locale code. */
            locales: {
                [key: string]: components["schemas"]["ReleaseLocaleCounts"];
            };
        };
        Release: {
            id: components["schemas"]["Id"];
            /** @description Counts the project's releases from 1. */
            version: number;
            /** @description What its environment served before it. */
            parent_id?: components["schemas"]["Id"];
            environment: components["schemas"]["EnvironmentName"];
            policy: components["schemas"]["EnvironmentPolicy"];
            /**
             * @description SHA-256 of the RFC 8785 canonical JSON of what every
             *     environment's manifest of this release carries (`sourceLocale`,
             *     `locales`, `fallback`, `artifacts`). Equal digests ship
             *     exactly the same text.
             */
            manifest_digest: string;
            source_locale: components["schemas"]["Locale"];
            locales: components["schemas"]["ReleaseLocale"][];
            counts: components["schemas"]["ReleaseCounts"];
            note?: string;
            /** @description `person:<id>` or `token:<id>`. */
            author: string;
            created_at: components["schemas"]["Timestamp"];
        };
        ReleaseList: {
            items: components["schemas"]["Release"][];
            next_page_token?: string;
        };
        LocaleDiff: {
            locale: components["schemas"]["Locale"];
            added: string[];
            changed: string[];
            removed: string[];
        };
        ReleaseDiff: {
            release_id: components["schemas"]["Id"];
            base_release_id?: components["schemas"]["Id"];
            locales: components["schemas"]["LocaleDiff"][];
        };
        /** @description One reason the catalog can't be released. */
        ReleaseProblem: {
            /** @enum {string} */
            code: "not_releasable";
            detail: string;
            /** @description The message key, when one message is the cause. */
            key?: string;
            /** @description The locale, when one locale or translation is the cause. */
            locale?: string;
        };
        ReleasePreview: {
            environment: components["schemas"]["EnvironmentName"];
            /** @description The environment's policy, which the build used. */
            policy: components["schemas"]["EnvironmentPolicy"];
            /** @description The release the environment serves now, which `changes` compare with; absent when it serves none (everything is added). */
            base_release_id?: components["schemas"]["Id"];
            /** @description `false` when `problems` lists why publishing would fail. */
            releasable: boolean;
            problems: components["schemas"]["ReleaseProblem"][];
            /** @description The release's manifest digest; equal to the served release's when nothing would change. Present when releasable. */
            manifest_digest?: string;
            source_locale?: components["schemas"]["Locale"];
            locales?: components["schemas"]["ReleaseLocale"][];
            /** @description What the release would ship; `new_artifacts` is what publishing would upload. Present when releasable. */
            counts?: components["schemas"]["ReleaseCounts"];
            /** @description Per locale, compared with `base_release_id`. Present when releasable. */
            changes?: components["schemas"]["LocaleDiff"][];
        };
        /** @description A `glossa.manifest/v1` manifest (runtimes/testdata/schemas/manifest.schema.json), as served. */
        ReleaseManifest: {
            [key: string]: unknown;
        };
        /** @description A `glossa.artifact/v1` artifact (runtimes/testdata/schemas/artifact.schema.json), as stored. */
        ReleaseArtifact: {
            [key: string]: unknown;
        };
        SigningKey: {
            /** @description The manifest's `signatures[].keyId`. */
            key_id: string;
            /** @enum {string} */
            algorithm: "Ed25519";
            /** @description The raw 32-byte public key, base64url without padding. */
            public_key: string;
            /** @description Whether it signs new manifests. */
            active: boolean;
        };
        SigningKeys: {
            keys: components["schemas"]["SigningKey"][];
        };
        DeliveryKey: {
            id: components["schemas"]["Id"];
            name: string;
            /** @description Publishable by design; shown on every read. */
            key: string;
            /** @description `person:<id>` or `token:<id>`. */
            created_by: string;
            created_at: components["schemas"]["Timestamp"];
            revoked_at?: components["schemas"]["Timestamp"];
        };
        DeliveryKeyList: {
            items: components["schemas"]["DeliveryKey"][];
            next_page_token?: string;
        };
        CreateDeliveryKey: {
            /** @description What uses it, e.g. "web" or "go-emails". */
            name: string;
        };
    };
    responses: {
        /** @description Signed in. The session cookie is set. */
        SessionStarted: {
            headers: {
                "Set-Cookie": components["headers"]["SessionCookie"];
                [name: string]: unknown;
            };
            content: {
                "application/json": components["schemas"]["Session"];
            };
        };
        /** @description The request is malformed or invalid. */
        BadRequest: {
            headers: {
                [name: string]: unknown;
            };
            content: {
                "application/problem+json": components["schemas"]["Problem"];
            };
        };
        /** @description No valid credentials. */
        Unauthenticated: {
            headers: {
                [name: string]: unknown;
            };
            content: {
                "application/problem+json": components["schemas"]["Problem"];
            };
        };
        /** @description Authenticated, but not allowed (or missing the CSRF token). */
        Forbidden: {
            headers: {
                [name: string]: unknown;
            };
            content: {
                "application/problem+json": components["schemas"]["Problem"];
            };
        };
        /** @description No such resource in this tenant. */
        NotFound: {
            headers: {
                [name: string]: unknown;
            };
            content: {
                "application/problem+json": components["schemas"]["Problem"];
            };
        };
        /** @description The request conflicts with the resource's state. */
        Conflict: {
            headers: {
                [name: string]: unknown;
            };
            content: {
                "application/problem+json": components["schemas"]["Problem"];
            };
        };
        /** @description `If-Match` doesn't match the current `ETag`. */
        PreconditionFailed: {
            headers: {
                [name: string]: unknown;
            };
            content: {
                "application/problem+json": components["schemas"]["Problem"];
            };
        };
        /** @description `If-Match` is required. */
        PreconditionRequired: {
            headers: {
                [name: string]: unknown;
            };
            content: {
                "application/problem+json": components["schemas"]["Problem"];
            };
        };
        /** @description The `Idempotency-Key` was used for a different request. */
        UnprocessableEntity: {
            headers: {
                [name: string]: unknown;
            };
            content: {
                "application/problem+json": components["schemas"]["Problem"];
            };
        };
        /** @description Slow down. */
        TooManyRequests: {
            headers: {
                [name: string]: unknown;
            };
            content: {
                "application/problem+json": components["schemas"]["Problem"];
            };
        };
        /** @description A dependency (object storage) is unavailable; retry. */
        Unavailable: {
            headers: {
                [name: string]: unknown;
            };
            content: {
                "application/problem+json": components["schemas"]["Problem"];
            };
        };
        /**
         * @description `structural_qa_failed`: the translation is structurally
         *     incompatible with its source (`findings`, errors first).
         */
        StructuralQAFailed: {
            headers: {
                [name: string]: unknown;
            };
            content: {
                "application/problem+json": components["schemas"]["QAProblem"];
            };
        };
    };
    parameters: {
        /** @description A tenant `id`. */
        TenantPath: components["schemas"]["Id"];
        /** @description A member `id`. */
        MemberPath: components["schemas"]["Id"];
        /** @description An API token `id` (never its secret). */
        TokenPath: components["schemas"]["Id"];
        /** @description A project `id`. */
        ProjectPath: components["schemas"]["Id"];
        /** @description An application `id`. */
        ApplicationPath: components["schemas"]["Id"];
        /** @description A message `key` (`checkout.pay`). Keys are URL-safe as they are. */
        MessagePath: components["schemas"]["MessageKey"];
        /** @description A locale code; canonicalized before use. */
        LocalePath: components["schemas"]["Locale"];
        /** @description An environment `name`. */
        EnvironmentPath: components["schemas"]["EnvironmentName"];
        /** @description A release `id`. */
        ReleasePath: components["schemas"]["Id"];
        /** @description A passkey `id` (its credential ID, base64url). */
        PasskeyPath: string;
        /** @description A delivery key `id` (not the key itself). */
        DeliveryKeyPath: components["schemas"]["Id"];
        PageSize: number;
        /** @description The `next_page_token` of the previous page. */
        PageToken: string;
        /** @description The `ETag` the change is based on. */
        IfMatch: string;
        /** @description When sent, the `ETag` the change is based on. */
        IfMatchOptional: string;
        IdempotencyKey: string;
        /** @description WebAuthn ceremony state set by the matching challenge operation. */
        CeremonyCookie: string;
    };
    requestBodies: never;
    headers: {
        /** @description Opaque version of the resource, for `If-Match`. */
        ETag: string;
        /** @description URL of the created resource. */
        Location: string;
        /** @description `true` when this response replays an earlier request with the same `Idempotency-Key`. */
        IdempotentReplayed: "true";
        /** @description `__Host-glossa_session=…; Path=/; HttpOnly; Secure; SameSite=Lax` */
        SessionCookie: string;
        /** @description Expires the session cookie. */
        ClearedSessionCookie: string;
        /** @description `__Host-glossa_webauthn=…; Path=/; HttpOnly; Secure; SameSite=Strict; Max-Age=300` */
        CeremonyCookie: string;
    };
    pathItems: never;
}
export type $defs = Record<string, never>;
export interface operations {
    getMeta: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description The deployment's facts. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Meta"];
                };
            };
        };
    };
    requestMagicLink: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EmailRequest"];
            };
        };
        responses: {
            /** @description Accepted; a link is on its way if the address can receive one. */
            202: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            400: components["responses"]["BadRequest"];
            404: components["responses"]["NotFound"];
            429: components["responses"]["TooManyRequests"];
        };
    };
    redeemMagicLink: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["TokenRedemption"];
            };
        };
        responses: {
            200: components["responses"]["SessionStarted"];
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            404: components["responses"]["NotFound"];
        };
    };
    register: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["Registration"];
            };
        };
        responses: {
            /** @description Accepted; a verification link is on its way. */
            202: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            400: components["responses"]["BadRequest"];
            429: components["responses"]["TooManyRequests"];
        };
    };
    signInWithPassword: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["PasswordSignIn"];
            };
        };
        responses: {
            200: components["responses"]["SessionStarted"];
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            429: components["responses"]["TooManyRequests"];
        };
    };
    requestPasswordReset: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EmailRequest"];
            };
        };
        responses: {
            /** @description Accepted. */
            202: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            400: components["responses"]["BadRequest"];
            404: components["responses"]["NotFound"];
            429: components["responses"]["TooManyRequests"];
        };
    };
    resetPassword: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["PasswordReset"];
            };
        };
        responses: {
            /** @description Password changed; every session was revoked. */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            404: components["responses"]["NotFound"];
        };
    };
    beginPasskeySignIn: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["EmailRequest"];
            };
        };
        responses: {
            /** @description Request options. */
            200: {
                headers: {
                    "Set-Cookie": components["headers"]["CeremonyCookie"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["PasskeyOptions"];
                };
            };
            400: components["responses"]["BadRequest"];
            404: components["responses"]["NotFound"];
        };
    };
    finishPasskeySignIn: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: {
                /** @description WebAuthn ceremony state set by the matching challenge operation. */
                "__Host-glossa_webauthn"?: components["parameters"]["CeremonyCookie"];
            };
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["WebAuthnResponse"];
            };
        };
        responses: {
            200: components["responses"]["SessionStarted"];
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
        };
    };
    signOut: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Signed out. */
            204: {
                headers: {
                    "Set-Cookie": components["headers"]["ClearedSessionCookie"];
                    [name: string]: unknown;
                };
                content?: never;
            };
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
        };
    };
    signOutEverywhere: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Signed out everywhere. */
            204: {
                headers: {
                    "Set-Cookie": components["headers"]["ClearedSessionCookie"];
                    [name: string]: unknown;
                };
                content?: never;
            };
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
        };
    };
    getMe: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description The person. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Me"];
                };
            };
            401: components["responses"]["Unauthenticated"];
        };
    };
    beginPasskeyRegistration: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Creation options. */
            200: {
                headers: {
                    "Set-Cookie": components["headers"]["CeremonyCookie"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["PasskeyOptions"];
                };
            };
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    listPasskeys: {
        parameters: {
            query?: {
                page_size?: components["parameters"]["PageSize"];
                /** @description The `next_page_token` of the previous page. */
                page_token?: components["parameters"]["PageToken"];
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description A page of passkeys. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["PasskeyList"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
        };
    };
    finishPasskeyRegistration: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: {
                /** @description WebAuthn ceremony state set by the matching challenge operation. */
                "__Host-glossa_webauthn"?: components["parameters"]["CeremonyCookie"];
            };
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["PasskeyRegistration"];
            };
        };
        responses: {
            /** @description The passkey was added. */
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Passkey"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
        };
    };
    deletePasskey: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A passkey `id` (its credential ID, base64url). */
                passkey: components["parameters"]["PasskeyPath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Removed. */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    beginTotpEnrollment: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description The secret to add to an authenticator app. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TotpEnrollment"];
                };
            };
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            409: components["responses"]["Conflict"];
        };
    };
    confirmTotpEnrollment: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["TotpCode"];
            };
        };
        responses: {
            /** @description TOTP is now required for password sign-in. */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            409: components["responses"]["Conflict"];
        };
    };
    disableTotp: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["TotpCode"];
            };
        };
        responses: {
            /** @description TOTP is off. */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            409: components["responses"]["Conflict"];
        };
    };
    previewMessage: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["MessagePreviewRequest"];
            };
        };
        responses: {
            /** @description What the kernel made of the source. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MessagePreview"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            429: components["responses"]["TooManyRequests"];
        };
    };
    listTenants: {
        parameters: {
            query?: {
                page_size?: components["parameters"]["PageSize"];
                /** @description The `next_page_token` of the previous page. */
                page_token?: components["parameters"]["PageToken"];
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description A page of tenants. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TenantList"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
        };
    };
    createTenant: {
        parameters: {
            query?: never;
            header?: {
                "Idempotency-Key"?: components["parameters"]["IdempotencyKey"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["CreateTenant"];
            };
        };
        responses: {
            /** @description The organization. */
            201: {
                headers: {
                    Location: components["headers"]["Location"];
                    "Idempotent-Replayed": components["headers"]["IdempotentReplayed"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Tenant"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            409: components["responses"]["Conflict"];
            422: components["responses"]["UnprocessableEntity"];
        };
    };
    getTenant: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description The tenant. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Tenant"];
                };
            };
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
        };
    };
    listMembers: {
        parameters: {
            query?: {
                page_size?: components["parameters"]["PageSize"];
                /** @description The `next_page_token` of the previous page. */
                page_token?: components["parameters"]["PageToken"];
            };
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description A page of members. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MemberList"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
        };
    };
    addMember: {
        parameters: {
            query?: never;
            header?: {
                "Idempotency-Key"?: components["parameters"]["IdempotencyKey"];
            };
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["AddMember"];
            };
        };
        responses: {
            /** @description The invitation. */
            201: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    Location: components["headers"]["Location"];
                    "Idempotent-Replayed": components["headers"]["IdempotentReplayed"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Member"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            409: components["responses"]["Conflict"];
            422: components["responses"]["UnprocessableEntity"];
        };
    };
    getMember: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A member `id`. */
                member: components["parameters"]["MemberPath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description The member. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Member"];
                };
            };
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    removeMember: {
        parameters: {
            query?: never;
            header?: {
                /** @description When sent, the `ETag` the change is based on. */
                "If-Match"?: components["parameters"]["IfMatchOptional"];
            };
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A member `id`. */
                member: components["parameters"]["MemberPath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Removed. */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
            412: components["responses"]["PreconditionFailed"];
        };
    };
    updateMember: {
        parameters: {
            query?: never;
            header: {
                /** @description The `ETag` the change is based on. */
                "If-Match": components["parameters"]["IfMatch"];
            };
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A member `id`. */
                member: components["parameters"]["MemberPath"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["UpdateMember"];
            };
        };
        responses: {
            /** @description The updated member. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Member"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
            412: components["responses"]["PreconditionFailed"];
            428: components["responses"]["PreconditionRequired"];
        };
    };
    listTokens: {
        parameters: {
            query?: {
                page_size?: components["parameters"]["PageSize"];
                /** @description The `next_page_token` of the previous page. */
                page_token?: components["parameters"]["PageToken"];
            };
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description A page of tokens. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TokenList"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
        };
    };
    createToken: {
        parameters: {
            query?: never;
            header?: {
                "Idempotency-Key"?: components["parameters"]["IdempotencyKey"];
            };
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["CreateToken"];
            };
        };
        responses: {
            /**
             * @description The token and its secret. A replayed request returns the token
             *     without `secret`.
             */
            201: {
                headers: {
                    Location: components["headers"]["Location"];
                    "Idempotent-Replayed": components["headers"]["IdempotentReplayed"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["CreatedToken"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            422: components["responses"]["UnprocessableEntity"];
        };
    };
    getToken: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description An API token `id` (never its secret). */
                token: components["parameters"]["TokenPath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description The token. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Token"];
                };
            };
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    revokeToken: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description An API token `id` (never its secret). */
                token: components["parameters"]["TokenPath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Revoked. */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
        };
    };
    listProjects: {
        parameters: {
            query?: {
                page_size?: components["parameters"]["PageSize"];
                /** @description The `next_page_token` of the previous page. */
                page_token?: components["parameters"]["PageToken"];
            };
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description A page of projects. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectList"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
        };
    };
    createProject: {
        parameters: {
            query?: never;
            header?: {
                "Idempotency-Key"?: components["parameters"]["IdempotencyKey"];
            };
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["CreateProject"];
            };
        };
        responses: {
            /** @description The project. */
            201: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    Location: components["headers"]["Location"];
                    "Idempotent-Replayed": components["headers"]["IdempotentReplayed"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Project"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            409: components["responses"]["Conflict"];
            422: components["responses"]["UnprocessableEntity"];
        };
    };
    getProject: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description The project. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Project"];
                };
            };
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    deleteProject: {
        parameters: {
            query?: never;
            header?: {
                /** @description When sent, the `ETag` the change is based on. */
                "If-Match"?: components["parameters"]["IfMatchOptional"];
            };
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Deleted. */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            412: components["responses"]["PreconditionFailed"];
        };
    };
    updateProject: {
        parameters: {
            query?: never;
            header: {
                /** @description The `ETag` the change is based on. */
                "If-Match": components["parameters"]["IfMatch"];
            };
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["UpdateProject"];
            };
        };
        responses: {
            /** @description The updated project. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Project"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
            412: components["responses"]["PreconditionFailed"];
            428: components["responses"]["PreconditionRequired"];
        };
    };
    listApplications: {
        parameters: {
            query?: {
                page_size?: components["parameters"]["PageSize"];
                /** @description The `next_page_token` of the previous page. */
                page_token?: components["parameters"]["PageToken"];
            };
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description A page of applications. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ApplicationList"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    createApplication: {
        parameters: {
            query?: never;
            header?: {
                "Idempotency-Key"?: components["parameters"]["IdempotencyKey"];
            };
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["CreateApplication"];
            };
        };
        responses: {
            /** @description The application. */
            201: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    Location: components["headers"]["Location"];
                    "Idempotent-Replayed": components["headers"]["IdempotentReplayed"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Application"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
            422: components["responses"]["UnprocessableEntity"];
        };
    };
    getApplication: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description An application `id`. */
                application: components["parameters"]["ApplicationPath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description The application. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Application"];
                };
            };
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    deleteApplication: {
        parameters: {
            query?: never;
            header?: {
                /** @description When sent, the `ETag` the change is based on. */
                "If-Match"?: components["parameters"]["IfMatchOptional"];
            };
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description An application `id`. */
                application: components["parameters"]["ApplicationPath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Removed. */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            412: components["responses"]["PreconditionFailed"];
        };
    };
    updateApplication: {
        parameters: {
            query?: never;
            header: {
                /** @description The `ETag` the change is based on. */
                "If-Match": components["parameters"]["IfMatch"];
            };
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description An application `id`. */
                application: components["parameters"]["ApplicationPath"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["UpdateApplication"];
            };
        };
        responses: {
            /** @description The updated application. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Application"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
            412: components["responses"]["PreconditionFailed"];
            428: components["responses"]["PreconditionRequired"];
        };
    };
    listMessages: {
        parameters: {
            query?: {
                page_size?: components["parameters"]["PageSize"];
                /** @description The `next_page_token` of the previous page. */
                page_token?: components["parameters"]["PageToken"];
                namespace?: components["schemas"]["Namespace"];
                state?: components["schemas"]["MessageState"];
                /** @description Keys starting with this, e.g. `checkout.`. */
                key_prefix?: string;
                /** @description Only messages without a translation in this locale. */
                missing_in?: components["schemas"]["Locale"];
                /** @description Only messages whose translation in this locale is outdated. */
                outdated_in?: components["schemas"]["Locale"];
            };
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description A page of messages. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MessageList"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    createMessage: {
        parameters: {
            query?: never;
            header?: {
                "Idempotency-Key"?: components["parameters"]["IdempotencyKey"];
            };
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["CreateMessage"];
            };
        };
        responses: {
            /** @description The message. */
            201: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    Location: components["headers"]["Location"];
                    "Idempotent-Replayed": components["headers"]["IdempotentReplayed"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Message"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
            422: components["responses"]["UnprocessableEntity"];
        };
    };
    upsertMessages: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["MessageUpsert"];
            };
        };
        responses: {
            /** @description One result per item, in request order. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MessageUpsertResult"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    getMessage: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A message `key` (`checkout.pay`). Keys are URL-safe as they are. */
                message: components["parameters"]["MessagePath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description The message. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Message"];
                };
            };
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    updateMessage: {
        parameters: {
            query?: never;
            header: {
                /** @description The `ETag` the change is based on. */
                "If-Match": components["parameters"]["IfMatch"];
            };
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A message `key` (`checkout.pay`). Keys are URL-safe as they are. */
                message: components["parameters"]["MessagePath"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["UpdateMessage"];
            };
        };
        responses: {
            /** @description The updated message. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Message"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            412: components["responses"]["PreconditionFailed"];
            428: components["responses"]["PreconditionRequired"];
        };
    };
    reviseMessageSource: {
        parameters: {
            query?: never;
            header: {
                /** @description The `ETag` the change is based on. */
                "If-Match": components["parameters"]["IfMatch"];
            };
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A message `key` (`checkout.pay`). Keys are URL-safe as they are. */
                message: components["parameters"]["MessagePath"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ReviseSource"];
            };
        };
        responses: {
            /** @description The message. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Message"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
            412: components["responses"]["PreconditionFailed"];
            428: components["responses"]["PreconditionRequired"];
        };
    };
    obsoleteMessage: {
        parameters: {
            query?: never;
            header?: {
                /** @description When sent, the `ETag` the change is based on. */
                "If-Match"?: components["parameters"]["IfMatchOptional"];
            };
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A message `key` (`checkout.pay`). Keys are URL-safe as they are. */
                message: components["parameters"]["MessagePath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description The obsolete message. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Message"];
                };
            };
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            412: components["responses"]["PreconditionFailed"];
        };
    };
    renameMessage: {
        parameters: {
            query?: never;
            header?: {
                /** @description When sent, the `ETag` the change is based on. */
                "If-Match"?: components["parameters"]["IfMatchOptional"];
            };
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A message `key` (`checkout.pay`). Keys are URL-safe as they are. */
                message: components["parameters"]["MessagePath"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["RenameMessage"];
            };
        };
        responses: {
            /** @description The renamed message. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    Location: components["headers"]["Location"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Message"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
            412: components["responses"]["PreconditionFailed"];
        };
    };
    listSourceRevisions: {
        parameters: {
            query?: {
                page_size?: components["parameters"]["PageSize"];
                /** @description The `next_page_token` of the previous page. */
                page_token?: components["parameters"]["PageToken"];
            };
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A message `key` (`checkout.pay`). Keys are URL-safe as they are. */
                message: components["parameters"]["MessagePath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description A page of source revisions. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["SourceRevisionList"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    listLocales: {
        parameters: {
            query?: {
                page_size?: components["parameters"]["PageSize"];
                /** @description The `next_page_token` of the previous page. */
                page_token?: components["parameters"]["PageToken"];
            };
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description A page of locales, by code. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["LocaleList"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    addLocale: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["AddLocale"];
            };
        };
        responses: {
            /** @description The locale already existed. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectLocale"];
                };
            };
            /** @description The locale. */
            201: {
                headers: {
                    Location: components["headers"]["Location"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectLocale"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    getLocale: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A locale code; canonicalized before use. */
                locale: components["parameters"]["LocalePath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description The locale. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectLocale"];
                };
            };
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    removeLocale: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A locale code; canonicalized before use. */
                locale: components["parameters"]["LocalePath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Removed. */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
        };
    };
    getFallbackGraph: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description The fallback graph. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["FallbackGraph"];
                };
            };
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    putFallbackGraph: {
        parameters: {
            query?: never;
            header?: {
                /** @description When sent, the `ETag` the change is based on. */
                "If-Match"?: components["parameters"]["IfMatchOptional"];
            };
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["PutFallbackGraph"];
            };
        };
        responses: {
            /** @description The fallback graph. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["FallbackGraph"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            412: components["responses"]["PreconditionFailed"];
            428: components["responses"]["PreconditionRequired"];
        };
    };
    listMessageTranslations: {
        parameters: {
            query?: {
                page_size?: components["parameters"]["PageSize"];
                /** @description The `next_page_token` of the previous page. */
                page_token?: components["parameters"]["PageToken"];
            };
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A message `key` (`checkout.pay`). Keys are URL-safe as they are. */
                message: components["parameters"]["MessagePath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description A page of translations. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TranslationList"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    getTranslation: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A message `key` (`checkout.pay`). Keys are URL-safe as they are. */
                message: components["parameters"]["MessagePath"];
                /** @description A locale code; canonicalized before use. */
                locale: components["parameters"]["LocalePath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description The translation. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Translation"];
                };
            };
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    putTranslation: {
        parameters: {
            query?: never;
            header?: {
                /** @description When sent, the `ETag` the change is based on. */
                "If-Match"?: components["parameters"]["IfMatchOptional"];
            };
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A message `key` (`checkout.pay`). Keys are URL-safe as they are. */
                message: components["parameters"]["MessagePath"];
                /** @description A locale code; canonicalized before use. */
                locale: components["parameters"]["LocalePath"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["PutTranslation"];
            };
        };
        responses: {
            /** @description The revised (or unchanged) translation. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Translation"];
                };
            };
            /** @description The new translation. */
            201: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    Location: components["headers"]["Location"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Translation"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
            412: components["responses"]["PreconditionFailed"];
            422: components["responses"]["StructuralQAFailed"];
            428: components["responses"]["PreconditionRequired"];
        };
    };
    listTranslationRevisions: {
        parameters: {
            query?: {
                page_size?: components["parameters"]["PageSize"];
                /** @description The `next_page_token` of the previous page. */
                page_token?: components["parameters"]["PageToken"];
            };
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A message `key` (`checkout.pay`). Keys are URL-safe as they are. */
                message: components["parameters"]["MessagePath"];
                /** @description A locale code; canonicalized before use. */
                locale: components["parameters"]["LocalePath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description A page of revisions. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TranslationRevisionList"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    reviewTranslation: {
        parameters: {
            query?: never;
            header: {
                /** @description The `ETag` the change is based on. */
                "If-Match": components["parameters"]["IfMatch"];
            };
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A message `key` (`checkout.pay`). Keys are URL-safe as they are. */
                message: components["parameters"]["MessagePath"];
                /** @description A locale code; canonicalized before use. */
                locale: components["parameters"]["LocalePath"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ReviewTranslation"];
            };
        };
        responses: {
            /** @description The reviewed translation. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Translation"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
            412: components["responses"]["PreconditionFailed"];
            428: components["responses"]["PreconditionRequired"];
        };
    };
    listProjectTranslations: {
        parameters: {
            query: {
                page_size?: components["parameters"]["PageSize"];
                /** @description The `next_page_token` of the previous page. */
                page_token?: components["parameters"]["PageToken"];
                /** @description A locale to list; repeat for several (`locale=de&locale=fr`). */
                locale: components["schemas"]["Locale"][];
                /** @description Only translations in these review states; repeatable. */
                state?: components["schemas"]["ReviewState"][];
                /** @description `true`: only outdated translations; `false`: only current ones. */
                outdated?: boolean;
                namespace?: components["schemas"]["Namespace"];
                /** @description Keys starting with this, e.g. `checkout.`. */
                key_prefix?: string;
                /** @description Only translations of `active` (or `obsolete`) messages. */
                message_state?: components["schemas"]["MessageState"];
            };
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description A page of translations. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ProjectTranslationList"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    getTranslationStats: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description The summary. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TranslationStats"];
                };
            };
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    importTranslations: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["TranslationImport"];
            };
        };
        responses: {
            /** @description One result per item, in request order. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TranslationImportResult"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    listEnvironments: {
        parameters: {
            query?: {
                page_size?: components["parameters"]["PageSize"];
                /** @description The `next_page_token` of the previous page. */
                page_token?: components["parameters"]["PageToken"];
            };
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description A page of environments. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EnvironmentList"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    createEnvironment: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["CreateEnvironment"];
            };
        };
        responses: {
            /** @description The environment. */
            201: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    Location: components["headers"]["Location"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Environment"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
        };
    };
    getEnvironment: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description An environment `name`. */
                environment: components["parameters"]["EnvironmentPath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description The environment. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Environment"];
                };
            };
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    updateEnvironment: {
        parameters: {
            query?: never;
            header: {
                /** @description The `ETag` the change is based on. */
                "If-Match": components["parameters"]["IfMatch"];
            };
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description An environment `name`. */
                environment: components["parameters"]["EnvironmentPath"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["UpdateEnvironment"];
            };
        };
        responses: {
            /** @description The updated environment. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Environment"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
            412: components["responses"]["PreconditionFailed"];
            428: components["responses"]["PreconditionRequired"];
        };
    };
    promoteRelease: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description An environment `name`. */
                environment: components["parameters"]["EnvironmentPath"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["Promotion"];
            };
        };
        responses: {
            /** @description The environment, now serving the release. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Environment"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
        };
    };
    previewRelease: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description An environment `name`. */
                environment: components["parameters"]["EnvironmentPath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description The preview. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ReleasePreview"];
                };
            };
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            503: components["responses"]["Unavailable"];
        };
    };
    rollbackEnvironment: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description An environment `name`. */
                environment: components["parameters"]["EnvironmentPath"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["Rollback"];
            };
        };
        responses: {
            /** @description The environment, now serving the earlier release. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Environment"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
        };
    };
    listDeployments: {
        parameters: {
            query?: {
                page_size?: components["parameters"]["PageSize"];
                /** @description The `next_page_token` of the previous page. */
                page_token?: components["parameters"]["PageToken"];
            };
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description An environment `name`. */
                environment: components["parameters"]["EnvironmentPath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description A page of deployments. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["DeploymentList"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    listReleases: {
        parameters: {
            query?: {
                page_size?: components["parameters"]["PageSize"];
                /** @description The `next_page_token` of the previous page. */
                page_token?: components["parameters"]["PageToken"];
            };
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description A page of releases. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ReleaseList"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    publishRelease: {
        parameters: {
            query?: never;
            header?: {
                "Idempotency-Key"?: components["parameters"]["IdempotencyKey"];
            };
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["PublishRelease"];
            };
        };
        responses: {
            /** @description The release. */
            201: {
                headers: {
                    Location: components["headers"]["Location"];
                    "Idempotent-Replayed": components["headers"]["IdempotentReplayed"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Release"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
            422: components["responses"]["UnprocessableEntity"];
            503: components["responses"]["Unavailable"];
        };
    };
    getRelease: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A release `id`. */
                release: components["parameters"]["ReleasePath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description The release. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Release"];
                };
            };
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    getReleaseDiff: {
        parameters: {
            query?: {
                /** @description The release `id` to compare with. */
                base?: components["schemas"]["Id"];
            };
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A release `id`. */
                release: components["parameters"]["ReleasePath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description The diff. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ReleaseDiff"];
                };
            };
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            503: components["responses"]["Unavailable"];
        };
    };
    getReleaseManifest: {
        parameters: {
            query: {
                environment: string;
            };
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A release `id`. */
                release: components["parameters"]["ReleasePath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description The manifest (`glossa.manifest/v1`). */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ReleaseManifest"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    getReleaseArtifact: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A release `id`. */
                release: components["parameters"]["ReleasePath"];
                /** @description The artifact's SHA-256, as the manifest names it. */
                digest: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description The artifact (`glossa.artifact/v1`). */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ReleaseArtifact"];
                };
            };
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            503: components["responses"]["Unavailable"];
        };
    };
    listReleaseSigningKeys: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description The keys, active first. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["SigningKeys"];
                };
            };
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    listDeliveryKeys: {
        parameters: {
            query?: {
                page_size?: components["parameters"]["PageSize"];
                /** @description The `next_page_token` of the previous page. */
                page_token?: components["parameters"]["PageToken"];
            };
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description A page of keys. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["DeliveryKeyList"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    createDeliveryKey: {
        parameters: {
            query?: never;
            header?: {
                "Idempotency-Key"?: components["parameters"]["IdempotencyKey"];
            };
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["CreateDeliveryKey"];
            };
        };
        responses: {
            /** @description The key. */
            201: {
                headers: {
                    Location: components["headers"]["Location"];
                    "Idempotent-Replayed": components["headers"]["IdempotentReplayed"];
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["DeliveryKey"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            422: components["responses"]["UnprocessableEntity"];
        };
    };
    revokeDeliveryKey: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                /** @description A tenant `id`. */
                tenant: components["parameters"]["TenantPath"];
                /** @description A project `id`. */
                project: components["parameters"]["ProjectPath"];
                /** @description A delivery key `id` (not the key itself). */
                delivery_key: components["parameters"]["DeliveryKeyPath"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Revoked. */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            401: components["responses"]["Unauthenticated"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
        };
    };
}
