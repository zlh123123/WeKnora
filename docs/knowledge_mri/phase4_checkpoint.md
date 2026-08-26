# Knowledge MRI Phase 4 Checkpoint — Quiz Attempt and Verified Mastery

Date: 2026-08-25
Phase plan: `WeKnora_Knowledge_MRI_Codex_Phased_Plan_2026-08-23/05_PHASE_4_MASTERY.md`

## Outcome

Phase 4 is complete. The caller-scoped answer path is now:

```text
active MCQ in the caller's active scan
  -> server compares selected_option with stored correct_option
  -> append quiz_attempt (score 1 or 0)
  -> select the current-source, non-stale recent-four window
  -> append quiz_attempt learning_evidence
  -> atomically update the mastery projection
  -> return grading, explanation, previous state and new state
```

The attempt, evidence and projection update share one database transaction.
The learning-profile row is locked for the caller+KB scope, so exposure and
assessment writers are serialized. A process-local keyed lock supplies the
same behavior in SQLite tests, where `FOR UPDATE` is a no-op.

## API and ownership

```http
POST /api/v1/knowledgebase/:kb_id/learning/quiz/:item_id/attempt
```

Request:

```json
{
  "scan_id": "uuid",
  "selected_option": 0,
  "idempotency_key": "client-request-key"
}
```

The request has no tenant or subject field. The service derives the Web
principal's `StorageID`, resolves the KB owner tenant, and hashes the client
key together with tenant, subject, KB, scan and item. An item must belong to
the requested KB, be active and non-stale, and appear in a pending or active
scan owned by that same subject. Tracking must already be enabled.

The response includes `is_correct`, the server-held explanation,
`previous_state`, `new_state`, `verified_mastery`, `mastery_confidence`, the
attempt ID and whether the response was an idempotent replay.

## Deterministic state machine

Only `selected_option == correct_option` is correct. The explanation and all
LLM output are excluded from grading.

For the answered concept, the projection reads at most the newest four
attempts whose quiz item:

- has the same `source_hash` as the answered current item;
- remains active; and
- is not stale.

The state transition is:

```text
no quiz attempts:       unseen or exposed (unchanged by Phase 4)
one valid attempt:      uncertain
two to four attempts:
  correct rate >= 0.80  verified_strong
  correct rate <= 0.40  verified_weak
  otherwise             uncertain
```

`verified_mastery` is the valid-window correct rate. The explainable MVP
confidence is `valid_attempt_count / 4`, capped naturally by the four-attempt
window. `quiz_attempt_count` is the total append-only count for the concept;
it is separate from the current-source mastery window.

## Real demo A — RAG, two correct answers

Knowledge base: `测试wiki`
KB ID: `379a24f9-9443-4247-aae7-3d41b0bf3bb4`
Concept: `RAG` (`887aef4d-cf5b-473f-88a3-9e7dc79d1afc`)

The baseline was seeded as a Phase-2-shaped `displayed_reference` exposure
using the real Wiki page and real cited chunk, producing `status=exposed`,
`exposure_count=1`, and `exposure_weight=1`.

1. Question `f3adf6a4-bf05-4954-945f-343e96575af5`, selected `2`, stored correct
   option `2`: correct. Attempt `01013f22-4ae1-4338-b324-c4b90b61e230` changed
   `exposed -> uncertain`, mastery `1.0`, confidence `0.25`.
2. Question `525dedde-f33a-4715-b4ce-e982f749de3a`, selected `1`, stored correct
   option `1`: correct. Attempt `62a34cc0-b7e7-468d-8388-c16cb4e1b596` changed
   `uncertain -> verified_strong`, mastery `1.0`, confidence `0.50`.

Final database projection: two attempts, `verified_strong`; the original
exposure count and weight both remain `1`.

## Real demo B — 嵌入模型, two wrong answers

Concept: `嵌入模型` (`a05ccbfa-c130-4ca8-8b98-28263e73d706`)

This concept received the same real-page/real-chunk exposed baseline.

1. Question `6d47972a-6a0d-4e50-8595-fe42563d8b8b`, selected `1`, stored correct
   option `0`: wrong. Attempt `5e75dbcc-7edc-423b-89c7-5071a8efc9f1` changed
   `exposed -> uncertain`, mastery `0.0`, confidence `0.25`.
2. Question `a522e1e7-82fe-4bb0-b112-87df4254c7b0`, selected `1`, stored correct
   option `0`: wrong. Attempt `4e59435a-43f1-41cf-ae78-cbb4d02a567e` changed
   `uncertain -> verified_weak`, mastery `0.0`, confidence `0.50`.

Final database projection: two attempts, `verified_weak`; exposure count and
weight again remain `1`.

Across both live scans the database contains four attempts and four joined
`event_type=quiz_attempt` evidence rows, with zero missing evidence rows.

## Idempotency and concurrency

The live RAG first answer was replayed with the same idempotency key but a
different selected option. The API returned the original attempt ID and
original correct result with `idempotent=true`; the database remained at four
attempts and four assessment evidence rows.

Automated concurrency submits two distinct correct answers simultaneously for
one subject and concept. Under the race detector both requests succeed and the
final state contains two attempts with `verified_strong`; no projection update
is lost. The idempotency test also verifies identical previous/new snapshots
on replay and a 1:1 attempt/evidence count.

## Mastery write-entry audit

Production writes to `verified_mastery`, `mastery_confidence`,
`quiz_attempt_count`, `last_assessed_at`, or a quiz-derived status occur only
inside `learningRepository.SubmitQuizAttempt`.

- `AppendExposureEvidence` writes only exposure count, exposure weight,
  last-exposed time and the `unseen -> exposed` status transition.
- displayed references cannot overwrite an uncertain/verified status.
- there is no Memory-to-mastery writer.
- there is no LLM delta-to-mastery writer.
- the former generic concept-state upsert entry point was removed from the
  repository interface so production callers cannot bypass quiz grading.

## Verification

Automated coverage includes correct and wrong grading, strong/weak/uncertain
thresholds, idempotent replay, concurrent updates, foreign-subject scans,
inactive and stale items, current-source filtering, tracking disabled,
attempt/evidence 1:1, and preservation of exposure fields.

Commands completed successfully:

```text
go test ./internal/application/repository ./internal/application/service/learning ./internal/handler ./internal/router -count=1
go test -race ./internal/application/service/learning -run 'TestConcurrentQuizAttemptsDoNotLoseStateUpdates|TestQuizAttemptIsIdempotentAndEvidenceIsOneToOne' -count=1
go test ./... -count=1
go vet ./...
```

The Phase 4 Linux/arm64 binary was installed into the existing latest-image
`WeKnora-app` container. The previous binary is retained at
`/app/WeKnora.pre-phase4`; the restarted container is healthy and migration
version remains `86` with `dirty=false`.

Graph UI, MRI selection and recommendation remain outside this phase.
