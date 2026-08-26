# Knowledge MRI Phase 0 Checkpoint

## 1. Phase

`PHASE 0 COMPLETE`

## 2. 本阶段目标

在不修改业务代码的前提下，核验 Knowledge MRI 的真实代码落点、数据边界、迁移入口和最小回归基线。

## 3. 修改文件

- `docs/knowledge_mri/phase0_baseline.md` -> 记录 Wiki、引用、Principal、Memory、迁移与 Phase 1–7 落点 -> 锁定后续实现依据。
- `docs/knowledge_mri/phase0_checkpoint.md` -> 保存阶段交接结果 -> 满足分阶段审阅要求。

## 4. 数据库 / API 变化

NONE。

## 5. 自动测试

以下最小 Go 回归全部通过：

```bash
go test ./internal/types -run 'Test(PrincipalFromContextFallsBackToWebUser|PrincipalStorageID|WikiSourceKnowledgeID|WikiGraphDataJSON)$' -count=1
go test ./internal/logger -run '^TestCloneContextPreservesPrincipal$' -count=1
go test ./internal/application/service -run 'TestComputeGraphSubset_(OverviewTruncatesByLinkCount|MarksFamiliarSourcePages|OverviewTypeFilter|EgoDepth1|EgoRejectsMissingCenter)$' -count=1
go test ./internal/application/service/memory -run 'Test(ListItemsIsScopedToTheCaller|ExtractionRebuildsScopeFromPayload|ExtractionDroppedWhenPayloadHasNoScope)$' -count=1
go test ./internal/router -run 'TestWiki(ReadRoutesDenyCrossTenantKB|OperationLogRouteIsRemoved|WriteRoutesDenyOutOfScopeAPIKeyKB)$' -count=1
```

前端 Docker 运行环境已验证可正常提供 WeKnora 页面。生产 UI 镜像为 nginx 静态资源镜像；Phase 0 没有建立宿主机前端源码测试环境，也没有修改 lockfile。

## 6. 人工测试步骤

1. 确认 `WeKnora-app` 与 `WeKnora-frontend` 容器处于 Up/healthy。
2. 打开 `http://localhost/platform/knowledge-bases`。
3. 登录后确认知识库页面可见。
4. 进入 Wiki 知识库，确认 Wiki 与图谱页面能够打开。

## 7. 实际结果

- 当前基线 commit：`412dcc41c662c9b45698959e0c3c37db5b8dc9d3`。
- 当前版本：`0.7.2`。
- Wiki Concept、Chunk provenance、caller principal、双 migration 和现有 Graph UI 均可作为主线基础。
- Phase 1 必须使用 scope-level `learning_profiles` 保存 tracking lifecycle 与 `cleared_at`。

## 8. 本阶段没有做什么

- 没有新增业务表、API、后端逻辑或前端 UI。
- 没有实现 Exposure、Quiz、Mastery 或 Graph overlay。

## 9. 已知问题 / 风险

- Wiki rename 会更换 page UUID，因此 Learning 不能直接把 Wiki page ID 当稳定身份。
- chunk 到 Concept 是一对多映射，后续 Exposure 必须保留置信度。
- tracking 开关与 clear 不能只存在 Concept state 行中。

## 10. 下一阶段前建议人工确认

- 已于 2026-08-23 获得用户指令继续实施。
- Phase 1 采用独立 `learning_profiles`，并保持 Memory/Wiki/messages 不承载 mastery。

## 11. STOP

```text
Stopped after Phase 0.
Manual continuation approval received.
```
