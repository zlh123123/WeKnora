# Knowledge MRI Final Report

## Implemented

Phases 0-7 are implemented: displayed-reference exposure, evidence-bound MCQ bank, deterministic mastery, personal graph overlay, six-question scan, Evidence Trail, gap classification, and deterministic recommendation. Phase 8 now includes caller-scoped JSON export and lifecycle-safe clear/disable behavior. Phase 9 includes a reproducible latent-mastery pilot script.

## Verification

`go test ./... -count=1` passes. Frontend `npm test` (441 tests), type-check, build, and
`git diff --check` pass. The app image was rebuilt with `GOPROXY=https://goproxy.cn,direct`
and deployed alongside the rebuilt frontend; the app health check is green and the export
endpoint returned HTTP 200 with `Content-Disposition: attachment` for the authenticated user.
Authenticated Edge acceptance confirmed KB navigation, personal map mode, the six-question
scan modal (question 1/6 with grounded options), and return to the map. The Edge extension
does not expose a download event for this programmatic Blob download, so the API response and
UI invocation are the authoritative export checks.

On 2026-09-01 the complete journey was repeated in the in-app browser against a newly built
KB named `test2`: cited question, personal-map enablement, cold scan creation, all six answers,
completion, full-page refresh, Concept Drawer evidence, and export. The run created 86 scoped
concept identities, 12 bank items across three selected concepts, six attempts, and three
verified-strong states. It also exposed and fixed two first-use defects: old Wiki concepts had
no learning identities, and cold model-backed scan creation exceeded the frontend's global
30-second timeout.

Final commands passed:

```text
go test ./... -count=1
go test -race ./internal/application/service/learning ./internal/application/repository -count=1
cd frontend && npm test                    # 441 passed
cd frontend && npm run type-check
cd frontend && npm run build
git diff --check
```

## Limitations

Exposure is a displayed-reference signal, not proof of reading. Learning is isolated from long-term Memory. The evaluation simulator is not a user study. Prerequisite relations, SM-2 scheduling, self-report, GraphRAG learning edges, and open-ended questions remain intentionally out of scope.

The current deployment intermittently fails semantic retrieval because one configured endpoint
is preserved literally as `https://${SILICONFLOW_BASE_URL}` and the SiliconFlow embedding call
has also returned EOF. Keyword fallback allowed the cited chat journey to complete, but this is
an environment/configuration risk and can increase question and scan latency.

## Demo

Open a test KB, switch to `我的知识地图`, create exposure through a cited chat answer, open a Concept Drawer, run `扫描我的知识状态`, answer six questions, then confirm the overlay and Evidence Trail refresh.
