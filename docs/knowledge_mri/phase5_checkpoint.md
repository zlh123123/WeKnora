# Knowledge MRI Phase 5 Checkpoint — Personal Knowledge Map Overlay

Date: 2026-08-25
Phase plan: `WeKnora_Knowledge_MRI_Codex_Phased_Plan_2026-08-23/06_PHASE_5_GRAPH_OVERLAY.md`

## Outcome

Phase 5 is complete and deployed to the current local Docker environment.
The existing 71-node Wiki graph now has two explicitly separate display modes:

- **Knowledge Graph** keeps the existing page-type colors, legend, search,
  pan/zoom, node drawer, neighbor expansion and graph APIs unchanged.
- **My Knowledge Map** reuses the same graph and layout, highlights concept
  pages with caller-scoped learning state, and dims non-concept pages.

The personal mode has exactly five learning colors: `unseen`, `exposed`,
`uncertain`, `verified_strong`, and `verified_weak`. A concept with no
materialized state is shown as `unseen`; no mastery value is invented.

The mode selector is attached to the top breadcrumb as **Graph ˅**, matching
the adjacent knowledge-base switcher. The graph canvas itself contains no
separate mode button; it keeps only graph search, help and the legend. The
currently selected mode is highlighted inside the breadcrumb dropdown.

## Caller-scoped batch overlay API

```http
GET /api/v1/knowledgebase/:kb_id/wiki/learning-overlay
```

The request has no tenant or subject parameter. The service derives the Web
principal, resolves the knowledge-base owner scope, and performs one batch read
for all current active concept identities plus one batch read for the caller's
materialized states. Orphaned identities and concepts belonging to another KB
are excluded.

The response contains the tracking flag and, per concept, the stable concept
key, current Wiki page ID and slug, status, exposure count/weight, verified
mastery, confidence, attempt count, and recent exposure/assessment timestamps.
The frontend indexes this response once by slug and page ID; it does not issue
per-node requests.

## Privacy lifecycle

Learning tracking remains default-off. The first attempt to enter My Knowledge
Map fetches the caller-scoped profile and shows an explicit confirmation:

- cancelling leaves the graph in ordinary Knowledge Graph mode;
- confirming calls the existing tracking endpoint, refetches the overlay, and
  only then enters the personal mode;
- earlier use is not backfilled.

The live browser acceptance test exercised both cancel and confirm paths. The
test KB was left with tracking enabled after the successful confirm path.

## Concept drawer

Selecting a concept in personal mode adds a learning card above the existing
Wiki body. The card shows:

- current status;
- exposure count and weight;
- verified mastery and confidence;
- most recent exposure and assessment times;
- a **Verify mastery** button that is intentionally a Phase 6 placeholder.

The original Wiki content remains visible and interactive beneath the card.
No scan, recommendation, evidence timeline or learning path was added in this
phase.

## Live acceptance data

> Current-state note (2026-08-25): the rows below document the Phase 5 browser
> acceptance fixture at the time of testing. At the user's request, the
> caller-scoped fixture was subsequently cleared: 7 evidence rows, 4 attempts,
> 2 scans and 3 materialized states were deleted. Tracking remains enabled,
> while the 3 concept identities and 12 quiz items remain. The live overlay now
> returns these concepts as `unseen` until genuine user activity creates new
> evidence or attempts.

Knowledge base: `测试wiki`
KB ID: `379a24f9-9443-4247-aae7-3d41b0bf3bb4`

The real overlay returned three current concept identities:

| Concept | Status | Exposure | Quiz attempts |
| --- | --- | ---: | ---: |
| 嵌入模型 | `verified_weak` | 1 | 2 |
| RAG | `verified_strong` | 1 | 2 |
| 文本分块 | `exposed` | 1 | 0 |

The text-chunking exposure is an explicit Phase 5 acceptance fixture backed by
one append-only `displayed_reference` evidence row and its matching materialized
state. It lets the live map show exposed, strong and weak states together.

## Browser acceptance

The deployed page at `http://localhost/platform/knowledge-bases` was verified
in an authenticated browser session:

1. ordinary mode displayed all `71 / 71` nodes and the unchanged page-type
   legend;
2. personal mode displayed the five-state legend, dimmed non-concepts, and
   visibly rendered exposed (blue), strong (green), and weak (red) concepts;
3. switching back restored the ordinary legend and original graph colors;
4. the RAG drawer showed `verified_strong`, mastery `100%`, confidence `50%`,
   exposure `1 · 1.00`, both timestamps, and the original RAG Wiki body;
5. first-use cancellation did not change mode; explicit confirmation enabled
   tracking and entered the personal map.
6. the top `Graph ˅` dropdown switched both modes, highlighted the active item,
   and the former canvas-level mode button was absent.

Screenshots:

- `docs/knowledge_mri/screenshots/phase5_ordinary_graph.png`
- `docs/knowledge_mri/screenshots/phase5_personal_map.png`
- `docs/knowledge_mri/screenshots/phase5_learning_drawer.png`
- `docs/knowledge_mri/screenshots/phase5_breadcrumb_mode_dropdown.png`

## Automated verification

Commands completed successfully:

```text
go test ./internal/application/repository ./internal/application/service/learning ./internal/handler ./internal/router -count=1
go test ./... -count=1
npm run type-check
npm test
npm run build
```

Frontend results after the final breadcrumb-dropdown alignment: 407 tests
passed, zero failed. The focused Phase 5 tests cover
default-unseen normalization, the five approved colors, batch indexing,
ordinary/personal mode separation, no subject input, and preservation of the
drawer body. Backend tests cover caller isolation, batch behavior, current-KB
scope, missing-state defaults, orphan filtering, and the handler response.

## Deployment

The Phase 5 backend binary and frontend production build are installed in the
existing latest-image containers. `WeKnora-app` is healthy and the frontend
returns HTTP 200. Rollback copies are retained inside the containers:

- backend: `/app/WeKnora.pre-phase5`
- frontend: `/usr/share/nginx/html.pre-phase5`

Phase 6 is the first phase that will make the **Verify mastery** action a real
end-to-end scan flow: select questions, answer six items, and refresh mastery
back into the map. Phase 7 recommendations and evidence UI are not required to
try the scan loop.
