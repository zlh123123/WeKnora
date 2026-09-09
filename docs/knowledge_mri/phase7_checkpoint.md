# Knowledge MRI Phase 7 Checkpoint

> Date: 2026-08-29 (Asia/Shanghai)
> Status: implemented, automated verification complete, and authenticated Edge acceptance completed against the running deployment.

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
- `cd frontend && npm test` (441 passed)
- `cd frontend && npm run type-check`
- `git diff --check`

## Manual acceptance

1. Open `我的知识地图` and select a Concept node.
2. Confirm the Drawer shows `证据轨迹`; empty evidence is a valid state.
3. Seed or create a high-exposure weak Concept and confirm `潜在知识盲区` appears.
4. Confirm an unseen Concept is not labelled as a gap.
5. Confirm `下一步建议` opens the recommended Wiki Concept.
6. Confirm `重新验证` still opens the existing two-question scan flow.

Completed on 2026-08-29 in Edge using KB `测试wiki`: switched to `我的知识地图`, opened
`扫描我的知识状态`, answered all six questions, observed the completion summary
(`验证掌握 1`, `待确认 2`), and returned to the personal graph successfully.

### 2026-09-01 clean-KB acceptance

Authenticated in-app-browser acceptance was repeated from the beginning against KB `test2`
(`5ceda8d8-24b4-4a6b-8e5a-0f091560a7ee`). A cited chat answer completed, the personal map
was enabled, and the cold scan produced three concepts and six grounded questions. All six
answers were submitted, the completion summary reported three verified-strong concepts, and
the personal map remained available after a full page reload. The `目标检测` Drawer showed
status, mastery, confidence, last assessment, and two caller-scoped quiz evidence entries.

This run found and fixed a compatibility gap for knowledge bases whose Wiki predates learning:
`scanCandidates` now lazily creates learning concept identities from published Concept pages
when the scoped identity set is empty. `test2` initialized 86 eligible identities without
resetting the Wiki or user learning profile.
