# Knowledge MRI Phase 8 Checkpoint

Status: implemented for caller-scoped export, clear/disable guards, and migration-backed learning tables.

## Lifecycle

| Event | Behavior |
|---|---|
| create | Profile is created disabled and scoped by tenant, subject, KB. |
| read | All learning reads derive subject from the authenticated Web principal. |
| export | `GET /api/v1/knowledgebase/:kb_id/learning/export` returns JSON for the caller and KB only. |
| clear | Deletes caller state, evidence, attempts and scans, retaining `cleared_at` to reject late events. |
| disable | New exposure and quiz writes are discarded/rejected. |
| KB delete | Existing platform delete path removes KB-owned rows; deployment must verify physical cleanup for soft-delete retention. |
| tenant delete | Existing platform tenant cleanup applies; deployment must verify physical cleanup for soft-delete retention. |
| wiki rebuild | Stable concept identity is retained; unresolved identities are orphaned and omitted from overlay. |

## Verification

Targeted Go learning/repository/handler/router tests pass. Export is caller-scoped and does not include correct answers.

## Remaining deployment check

Physical cleanup after soft-deleted KB/tenant retention requires a database-specific integration run against the deployment configuration.

## 2026-09-01 live verification

The authenticated in-app browser invoked `GET /learning/export` for `test2`; the deployment
returned HTTP 200. The JSON contained the caller profile, three concept states, six evidence
records, and six quiz attempts. The UI download action completed without an error toast.
The cold scan request now has a route-specific three-minute client timeout because generating
three grounded quiz banks can legitimately exceed the global 30-second API timeout; all other
requests retain the existing default.
