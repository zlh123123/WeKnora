# Knowledge MRI Phase 2 Checkpoint

## 1. Phase

`PHASE 2 COMPLETE`

## 2. 本阶段目标

把最终成功持久化的 Assistant Message 上的 `KnowledgeReferences` 投影为 caller-scoped Exposure：

```text
persisted final assistant message
  -> deduplicated displayed chunk
  -> Wiki concept candidates
  -> displayed_reference evidence
  -> atomic exposure projection
```

本阶段没有接 Memory topic mapping，也没有更新 verified mastery。

## 3. 代码落点

- `internal/application/service/learning/service.go`
  - 只接受完成且已持久化的 assistant message。
  - 去重 `KB + chunk`，验证 chunk 仍存在且属于相同 KB/knowledge。
  - 从 KB owner tenant 构造 scope，subject 始终来自当前 Web principal。
  - 查询 Wiki Concept `ChunkRefs`，按候选数计算 `confidence = 1/N`。
  - 生成稳定哈希 idempotency key，并调用单一 repository 聚合入口。
- `internal/application/repository/learning.go`
  - `AppendExposureEvidence` 在同一事务内检查 tracking 生命周期、幂等插入并更新 exposure projection。
  - `exposure_count` 按不同 assistant message 计数；同一 message 的多个 chunk 不重复加 count。
  - `exposure_weight` 累加每条 evidence 的 confidence。
  - 只更新 exposure 字段，完全不写 verified mastery、mastery confidence、attempt count。
- `internal/handler/session/qa.go`
  - RAG 与 Agent 共用的 `completeAssistantMessage` 在消息持久化成功后调用 Learning Service。
  - 消息写入失败时不产生 exposure；IM/API/Embed 等非 Web principal 被明确跳过。
- `internal/handler/learning.go`
  - caller-scoped profile、tracking、clear 和 concept states API；不存在 subject 请求参数。
- `internal/router/routes_knowledge.go`
  - 新增私有 Learning 路由，复用 KB Viewer + `KBAccessRead` 权限。
  - 未声明 API-key capability，因此 MVP 仅 Web JWT principal 可用。
- `internal/container/container.go`
  - 注册 Learning repository/service/handler，并注入 session completion 链路。

## 4. API

```text
GET    /api/v1/knowledgebase/:kb_id/learning/profile
PUT    /api/v1/knowledgebase/:kb_id/learning/tracking   {"enabled": true|false}
DELETE /api/v1/knowledgebase/:kb_id/learning/data
GET    /api/v1/knowledgebase/:kb_id/learning/concept-states
```

所有 subject 都从 `Principal.StorageID()` 推导；客户端传入 `subject_id` 不参与任何授权或查询。

## 5. Exposure 规则

- 事件：`displayed_reference`。
- 输入只来自最终保存/展示的 `KnowledgeReferences`，raw retrieved context 不进入 evidence。
- 缺失、已删除、KB/knowledge 不匹配的 chunk 被跳过。
- 每个 displayed chunk 映射所有引用它的非 archived Wiki Concept。
- 一个 chunk 有 N 个候选 Concept 时，每条 evidence 的 confidence 为 `1/N`。
- idempotency key 覆盖 subject、KB、Concept、assistant message、event type、chunk，并使用 SHA-256 控制长度。
- tracking disabled、发生时间早于本次 enabled、或 `occurred_at <= cleared_at` 时直接丢弃。
- Shared KB 按 KB owner tenant 存储，按当前 Web principal subject 隔离；与 `KBAccessRead` 的 effective tenant 一致。

## 6. 真实“测试wiki”只读映射核验

当前运行库中的 KB：

```text
id:   379a24f9-9443-4247-aae7-3d41b0bf3bb4
name: 测试wiki
tenant_id: 10000
concept pages: 30
distinct referenced chunks: 47
```

one-to-many 分布：

| 每个 chunk 的 Concept 候选数 | chunk 数 |
|---:|---:|
| 1 | 27 |
| 2 | 11 |
| 3 | 7 |
| 4 | 1 |
| 8 | 1 |

20/47 个 chunk 映射多个 Concept，最高候选数为 8，与方案前提一致。

以下是从当前运行库只读查询得到的真实 candidate evidence 投影样例；当对应 chunk 出现在最终 assistant references 时，服务会按表中 confidence 写入 caller-scoped evidence：

| chunk_id | Concept | candidate_count | confidence |
|---|---|---:|---:|
| `0e34bad6-fd2e-4e28-b61a-7949bdc98361` | 高级RAG (`concept/advanced-rag`) | 8 | 0.125 |
| `0e34bad6-fd2e-4e28-b61a-7949bdc98361` | 多路融合 (`concept/fusion`) | 8 | 0.125 |
| `0e34bad6-fd2e-4e28-b61a-7949bdc98361` | 模块化RAG (`concept/modular-rag`) | 8 | 0.125 |
| `0e34bad6-fd2e-4e28-b61a-7949bdc98361` | 初级RAG (`concept/naive-rag`) | 8 | 0.125 |
| `0e34bad6-fd2e-4e28-b61a-7949bdc98361` | 查询重写 (`concept/query-rewrite`) | 8 | 0.125 |
| `0e34bad6-fd2e-4e28-b61a-7949bdc98361` | 查询转换 (`concept/query-transformation`) | 8 | 0.125 |
| `0e34bad6-fd2e-4e28-b61a-7949bdc98361` | 结果重排 (`concept/rerank`) | 8 | 0.125 |
| `0e34bad6-fd2e-4e28-b61a-7949bdc98361` | 动态路由 (`concept/routing`) | 8 | 0.125 |
| `341ba4ad-8fd9-4198-8dbe-baa501adbfe9` | 非参数化知识 (`concept/non-parametric-knowledge`) | 4 | 0.25 |
| `341ba4ad-8fd9-4198-8dbe-baa501adbfe9` | 参数化知识 (`concept/parametric-knowledge`) | 4 | 0.25 |

当前 Docker 镜像尚未包含本地 Phase 1/2 源码，当前业务库也没有 Learning 表。因此这里只对现有 Wiki 数据做了只读核验，没有修改或迁移当前业务库。隔离 SQLite 集成测试实际持久化了 3 条 evidence：单候选 confidence 1.0，以及双候选各 0.5。

## 7. 自动测试

通过：

```bash
go test ./internal/application/repository ./internal/application/service/learning \
  ./internal/handler/session ./internal/handler ./internal/router ./internal/container -count=1
go vet ./internal/application/repository ./internal/application/service/learning \
  ./internal/handler/session ./internal/handler ./internal/router ./internal/container \
  ./internal/types ./internal/types/interfaces
go test ./... -count=1
git diff --check
```

覆盖：

- 单 chunk -> 单 Concept。
- 单 chunk -> 多 Concept，confidence = `1/N`。
- 同一 message 的多 chunk：weight 累加、count 只加一次。
- completion 重放不重复 evidence/count/weight。
- 缺失或已删除 chunk 不产生 evidence/state。
- Shared KB owner tenant 与 caller subject 隔离。
- 另一用户看不到当前用户 state，且 tracking disabled 不写入。
- clear 后迟到 completion 不恢复数据。
- 已有 verified mastery、mastery confidence、attempt count、verified status 完全不变。
- assistant message 持久化失败时不产生 exposure。
- tracking API 能正确接受显式 `false`。
- 全仓 Fake-IP DNS 环境回归继续全绿。

## 8. Idempotency 结果

同一 `message + chunk + concept` 连续处理两次：

```text
first pass:  2 evidence, two states weight=0.5/count=1
second pass: 2 evidence, two states weight=0.5/count=1
```

结果没有重复累计。幂等插入与 projection 更新位于同一事务内。

## 9. 错误映射观察

- 未观察到跨 KB、错误 knowledge 或已删除 chunk 被写入的路径；这些情况在写入前都有明确校验。
- 真实数据存在 8-way candidate mapping。它不是系统自动选错 Concept，而是按 8 条 confidence=0.125 的候选 evidence 保留歧义。
- 本阶段不对候选做 LLM/embedding 二次判定，避免把弱 exposure 伪装成确定归属。

## 10. 人工测试步骤

在将当前源码构建为测试镜像并让迁移 `000085` 生效后：

1. 以 Web 用户登录，选择“测试wiki”。
2. `PUT /api/v1/knowledgebase/<kb_id>/learning/tracking`，body 为 `{"enabled":true}`。
3. 正常提问并等待带 `KnowledgeReferences` 的 assistant answer 完成保存。
4. `GET /api/v1/knowledgebase/<kb_id>/learning/concept-states`，确认相关 Concept 为 `exposed`。
5. 查询 `learning_evidence`，确认 event 为 `displayed_reference`，同一共享 chunk 可产生多条 `1/N` evidence。
6. 确认 `verified_mastery`、`mastery_confidence`、`quiz_attempt_count` 没有变化。
7. 关闭 tracking 后再提问，确认不新增 evidence；重新开启不回填关闭期间回答。
8. `DELETE /api/v1/knowledgebase/<kb_id>/learning/data` 后重放旧 completion，确认旧数据不恢复。

## 11. 本阶段没有做什么

- 没有 Memory interest/topic mapping。
- 没有 Quiz item、attempt、判分或 mastery 更新。
- 没有 Graph overlay、Concept Drawer、MRI scan 或推荐。
- 没有修改当前 Docker 镜像或业务数据库。

## 12. 下一阶段前人工确认

- 确认 `exposure_count = distinct assistant message`、`exposure_weight = confidence sum`。
- 确认 8-way mapping 保留 8 条低 confidence evidence，而不是自动挑一个 Concept。
- 确认 Learning API 仅 Web principal 使用，Shared KB 数据按 KB owner tenant + caller subject 保存。
- 确认 Phase 3 只进入 evidence-bound MCQ item bank，不提前实现 attempt/mastery。

## 13. STOP

```text
Stopped after Phase 2.
Waiting for manual review.
I will not start Phase 3 until explicitly requested.
```
