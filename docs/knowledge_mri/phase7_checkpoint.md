# Knowledge MRI Phase 7 Checkpoint

> Date: 2026-08-25 (Asia/Shanghai)
> Status: implemented, automated verification complete; browser deployment pending final manual confirmation.

## Scope

Phase 7 adds caller-scoped Concept insights to the existing personal knowledge-map Drawer:

- evidence timeline from `learning_evidence`, newest first;
- deterministic potential knowledge-gap classification;
- deterministic next-concept recommendation;
- Drawer actions to open the recommended Wiki concept and start re-verification.

## API

```text
GET /api/v1/knowledgebase/:kb_id/learning/concepts/:concept_key/insights
```

The service derives `tenant_id` and `subject_id` from the authenticated Web principal. The response exposes only safe evidence identifiers and timestamps; raw metadata is not returned.

## Rules

- `verified_weak` plus at least two exposures (`exposure_count >= 2` or `exposure_weight >= 1.5`) is a potential gap.
- `uncertain` plus at least three exposures and two quiz attempts is a potential gap.
- `unseen` is never labelled a gap.
- Recommendations exclude the current concept and verified-strong concepts, then rank weak, uncertain, exposed, exposure count, and concept key deterministically.

## Verification

- `go test ./internal/application/service/learning ./internal/application/repository ./internal/handler ./internal/router -count=1`
- `go test ./... -count=1`
- `cd frontend && npm test` (410 passed)
- `cd frontend && npm run type-check`
- `git diff --check`

## Manual acceptance

1. Open `我的知识地图` and select a Concept node.
2. Confirm the Drawer shows `证据轨迹`; empty evidence is a valid state.
3. Seed or create a high-exposure weak Concept and confirm `潜在知识盲区` appears.
4. Confirm an unseen Concept is not labelled as a gap.
5. Confirm `下一步建议` opens the recommended Wiki Concept.
6. Confirm `重新验证` still opens the existing two-question scan flow.
