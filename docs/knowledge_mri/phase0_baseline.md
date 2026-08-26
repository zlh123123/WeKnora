# Knowledge MRI Phase 0 Baseline

> 核验日期：2026-08-23
> 阶段范围：只读代码核验、落点设计和最小基线测试；未修改业务代码。

## 1. 仓库基线

| 项目 | 当前值 |
|---|---|
| 仓库 | `Tencent/WeKnora` |
| 分支 | `main` |
| Commit | `412dcc41c662c9b45698959e0c3c37db5b8dc9d3` |
| `VERSION` | `0.7.2` |
| `go.mod` | Go `1.26.0` |
| Git 状态（开始时） | `## main...origin/main`，无源码修改 |
| Knowledge MRI checkpoint | 无 |

本报告以当前 checkout 为准。执行包中记录的 commit、branch、版本和主要路径与当前代码一致。

## 2. 需求口径确认

当前实现应遵守以下范围：

- Learning node 是 Wiki `concept` page，不是 Document、Entity 或 GraphRAG entity。
- Learning scope 是 `(tenant_id, subject_id, knowledge_base_id)`。
- `subject_id` 必须由服务端 `Principal.StorageID()` 得到，客户端不得传入 subject 作为授权依据。
- MVP 产品入口只对已登录 Web 用户开放；IM、API external user 和 Embed visitor 暂不启用。
- `displayed_reference` 是弱 Exposure：表示最终 assistant message 保存/展示了知识库 reference，不表示用户阅读、理解或明确点赞。
- `displayed_reference -> chunk -> concept` 是 one-to-many candidate mapping，不能假设一个 chunk 只属于一个 Concept。
- Exposure 不得更新 verified mastery；verified mastery 只能由 evidence-bound MCQ attempt 更新。
- 现有 Wiki links 只表示页面链接，不能解释成 prerequisite。
- GraphRAG、Memory topic mapping、prerequisite、SM-2、Beta/BKT 不阻塞 MVP 主线。

## 3. Wiki Page、Graph 后端真实落点

### 3.1 类型

主要文件：`internal/types/wiki_page.go`

- `types.WikiPage`：约第 167 行。
- `types.WikiPageTypeConcept = "concept"`：约第 117 行。
- `types.WikiGraphRequest`：约第 650 行。
- `types.WikiGraphData`、`WikiGraphMeta`：约第 663 行。
- `types.WikiGraphNode`：约第 685 行。
- `types.WikiGraphEdge`：约第 698 行。
- `WikiSourceKnowledgeID`、`WikiPage.SourceKnowledgeIDs`：约第 799 行。

`WikiPage` 已包含 Knowledge MRI 所需的主要 source provenance：

```text
ID
TenantID
KnowledgeBaseID
Slug
Title
PageType
Aliases
SourceRefs
ChunkRefs
InLinks
OutLinks
Version
DeletedAt
```

其中：

- `SourceRefs` 保存文档级来源，格式是 `knowledge_id` 或 `knowledge_id|title`。
- `ChunkRefs` 保存生成该 Wiki page 所依据的 source chunk UUID。
- `InLinks` / `OutLinks` 保存 slug；它们形成普通 Wiki link graph，不带 prerequisite 类型。
- `WikiPage.ID` 是当前页面行 UUID，不适合作为跨 rename/rebuild 的唯一长期学习身份。

### 3.2 Interface、Repository、Service

- Service interface：`internal/types/interfaces/wiki_page.go` 的 `WikiPageService`。
- Repository interface：同文件的 `WikiPageRepository`。
- Repository implementation：`internal/application/repository/wiki_page.go`。
- Repository constructor：`repository.NewWikiPageRepository`。
- Service implementation：`internal/application/service/wiki_page.go` 的 `wikiPageService`。
- Service constructor：`service.NewWikiPageService`。
- Graph service：`wikiPageService.GetGraph`，约第 576 行。
- Graph pure subset logic：`computeGraphSubset`，约第 592 行。

`GetGraph` 当前先通过 `repo.ListAll` 读取当前 KB 的非 archived pages，再在服务层计算 overview/ego 子图：

- `overview`：按 link count 排序，HTTP 默认最多 500 个节点。
- `ego`：围绕 center slug 做无向 BFS，HTTP depth 最大 3，节点最大 2000。
- Graph edge 由 `WikiPage.OutLinks` 生成，source/target 都是 slug。

### 3.3 Handler 和 API

- Handler：`internal/handler/wiki_page.go` 的 `WikiPageHandler`。
- KB 校验：`WikiPageHandler.validateWikiKB`，约第 48 行。
- Graph handler：`WikiPageHandler.GetGraph`，约第 801 行。
- 路由注册：`internal/router/routes_knowledge.go` 的 `RegisterWikiPageRoutes`。
- API：`GET /api/v1/knowledgebase/:kb_id/wiki/graph`。
- Route guard：`Viewer()` + `KBAccessRead("kb_id")`。

Graph handler 已从当前 request context 调用 `memoryService.FamiliarKnowledgeIDs`，并把结果放进 `WikiGraphRequest.FamiliarKnowledgeIDs`。`computeGraphSubset` 根据 `WikiPage.SourceRefs` 把节点标为 `familiar`。

这是一条已经运行的“个人数据 overlay 到同一张 Wiki Graph”范式，但它只表示用户反复使用某些文档，不是 Concept exposure，更不是 verified mastery。

### 3.4 依赖注入

当前 Wiki 依赖注入在：

- `internal/container/container.go`：
  - `repository.NewWikiPageRepository`
  - `service.NewWikiPageService`
  - `handler.NewWikiPageHandler`
- `internal/router/router.go`：持有并注册 `WikiPageHandler`。

后续 Learning repository/service/handler 应沿用这条依赖注入路径。

## 4. 前端真实落点

### 4.1 “文档 / Wiki / 图谱”控制位置

父组件：`frontend/src/views/knowledge/KnowledgeBase.vue`

- `validTabs = ['documents', 'wiki', 'graph']`：约第 88 行。
- `activeKbTab`：约第 91 行。
- breadcrumb tab 模板：约第 2324 行。
- `WikiBrowser` 挂载：约第 2376 行。
- Graph 通过 `<WikiBrowser :view="'graph'">` 进入，不是独立顶层页面。

“图谱⌄ / 知识图谱 / 我的知识地图”最合适的入口是 `KnowledgeBase.vue` 当前 Graph breadcrumb tab（约第 2336 行）。父组件应拥有 `graphViewMode`，再把它作为 prop 传给 `WikiBrowser`。这样不会把产品导航状态混入 SVG 实现细节，也能保留现有 `tab=graph` URL 语义。

建议把个人模式同步到独立 query，例如 `graph_view=learning`；不要复用现有后端 `mode=overview|ego`，因为该 `mode` 已表示 Graph topology slice。

### 4.2 WikiBrowser、Graph 和 Drawer

主要文件：`frontend/src/views/knowledge/wiki/WikiBrowser.vue`

- Graph template：约第 3 行。
- Graph search：约第 8 行。
- Legend：约第 49 行。
- Graph page drawer：约第 129 行。
- Graph state：约第 986 行。
- Graph page type filters：约第 998 行。
- `graphDrawerPage`：约第 1140 行。
- `openGraphDrawer`：约第 1538 行。
- Graph renderer：约第 3759 行。
- Page type color map：约第 3738 行。

当前没有独立的 `ConceptDrawer.vue`。Graph Drawer 是 `WikiBrowser.vue` 内联的 `<t-drawer>`，点击节点后通过 `getWikiPage(kbID, slug)` 加载完整页面。Phase 5 应优先在这段 drawer 模板顶部加入 learning status card；是否以后拆组件不应阻塞 MVP。

当前普通图谱颜色：

```text
summary    #0052d9
entity     #2ba471
concept    #e37318
synthesis  #0594fa
comparison #d54941
index      #8c8c8c
```

现有 Graph 还会为 `familiar` 节点绘制蓝色实线外圈。个人学习模式必须明确处理该外圈，避免同时用两套接近的视觉语义表达“常用资料”和“学习状态”。推荐个人模式隐藏 familiar 外圈，只用节点颜色表达 learning status；普通图谱保持现状。

### 4.3 前端 API

文件：`frontend/src/api/wiki/index.ts`

- `WikiGraphMeta`：约第 72 行。
- `WikiGraphData`：约第 82 行。
- `WikiGraphQueryParams`：约第 302 行。
- `getWikiGraph`：约第 315 行。

Graph node 当前只返回：

```text
slug, title, page_type, link_count, familiar
```

它不返回 `wiki_page_id` 或 `concept_key`。Phase 5 应通过独立、批量的 learning overlay API 返回 `concept_key/current_wiki_page_id/slug/status`，前端按 current page ID 优先、slug 兜底合并；不得按节点逐个请求。

## 5. Assistant completion 与 KnowledgeReferences

### 5.1 Message 持久化字段

文件：`internal/types/message.go`

- `types.Message`：约第 254 行。
- `Message.KnowledgeReferences`：约第 266 行，数据库列 `knowledge_references`。

`KnowledgeReferences` 的元素是 `types.SearchResult`：

- `SearchResult.ID` 是 chunk ID。
- `SearchResult.KnowledgeID` 是 source knowledge/document ID。
- `SearchResult.KnowledgeBaseID` 是 source KB ID。
- 还包含 title、chunk index、content、metadata 等展示数据。

### 5.2 正常 RAG 路径

关键路径：

```text
sessionService KnowledgeQA
  -> emitKnowledgeReferencesEvent
  -> EventAgentReferences
  -> AgentStreamHandler.handleReferences
  -> assistantMessage.KnowledgeReferences
  -> Handler.completeAssistantMessage
  -> messageService.UpdateMessage
```

真实文件：

- `internal/application/service/session_knowledge_qa.go`
  - `emitKnowledgeReferencesEvent`，约第 1201 行。
- `internal/handler/session/agent_stream_handler.go`
  - `handleReferences`，约第 386 行。
- `internal/handler/session/qa.go`
  - normal mode final-answer completion，约第 964 行。
  - `completeAssistantMessage`，约第 1422 行。

正常 RAG 在 answer 之前把 `chatManage.MergeResult` 作为 references 发出；即使 inline citations 关闭，这些 references 仍可保存在 assistant message 中。因此 `displayed_reference` 必须继续被描述为弱 exposure，不能声称 LLM 明确引用了该 chunk。

### 5.3 Agent 路径

关键路径：

```text
AgentEngine state.KnowledgeRefs
  -> AgentCompleteData.KnowledgeRefs
  -> AgentStreamHandler.handleComplete
  -> assistantMessage.KnowledgeReferences
  -> executeQA defer
  -> Handler.completeAssistantMessage
  -> messageService.UpdateMessage
```

真实文件：

- `internal/agent/finalize.go` 的 `emitCompletionEvent`，约第 181 行。
- `internal/handler/session/agent_stream_handler.go` 的 `handleComplete`，约第 614 行。
- `internal/handler/session/qa.go` 的 Agent defer completion，约第 1006 行。

正常 RAG 与 Agent 最终都汇合到 `Handler.completeAssistantMessage`。Phase 2 的 displayed-reference hook 应接在这里的“assistant message 成功持久化之后”，而不是另造聊天链路，也不能复用 `recordAnswerSources` 作为唯一入口。

### 5.4 现有 document affinity

`internal/handler/session/qa.go` 的 `recordAnswerSources` 会：

- 按 `KnowledgeID` 去重 references。
- 写入 `MemoryDocAffinity`。
- 为 rerank 和现有 Graph familiar 外圈提供个性化信号。

它受 Memory workspace/user/agent 开关和 retrieval-conditioning 配置影响，粒度也是 document。Learning tracking 有独立开关和生命周期，Phase 2 不应把它当作 Knowledge MRI exposure store。

### 5.5 Completion 接入风险

- `completeAssistantMessage` 当前忽略 `UpdateMessage` 返回错误。Phase 2 必须保证只有 assistant message 确认持久化后才异步写 learning event，否则会产生找不到 source message 的 evidence。
- completion、网络重试和异步 worker 可能重放，`learning_evidence.idempotency_key` 必须兜底。
- stop 路径也会保存已产生的 partial assistant message。按照“最终保存/展示 reference”的锁定定义，只要 message 和 references 成功保存，partial turn 也可产生弱 exposure；实现时不能误绑到 `userQuery != ""` 的 Memory 分支而静默漏记。
- shared-agent 场景会临时把 `TenantIDContextKey` 切到 agent owner；completion 持久化又切回 session tenant。Learning profile 的 tenant 应是 caller/session tenant，KB ID 可以指向当前 caller 有权访问的 shared KB；不能把 agent owner tenant 当成 learner tenant。

## 6. Principal、Memory scope 与 topic 入口

### 6.1 Principal

文件：`internal/types/principal.go`

- `Principal`：约第 45 行。
- `Principal.StorageID()`：约第 62 行，格式为 `type:id`。
- `PrincipalFromContext`：约第 78 行；缺少显式 Principal 时会从 UserID 回退为 `web_user`。

Principal 常量包括：

```text
web_user
api_tenant
api_platform
api_external_user
im_user
embed_channel
embed_session
embed_visitor
```

Learning MVP 的 resolver 必须在得到 Principal 后额外检查 `principal.Type == web_user`。仅复用 Memory `ResolveScope` 不够，因为 Memory 明确支持 IM/API/Embed，而 Learning MVP 明确不支持。

### 6.2 Memory scope 可复用模式

文件：`internal/application/service/memory/scope.go`

- `memory.ResolveScope` 只从 context 读取 tenant 和 principal。
- 返回 `interfaces.MemoryScope{TenantID, SubjectID}`。
- repository 每个方法都显式接收 scope，避免 background worker 猜 ambient caller。

Learning 应复用这种 caller-scoped pattern，但建立独立的 `LearningScope`：

```text
TenantID
SubjectID
KnowledgeBaseID
```

`PrincipalContextKey` 已列入 `types.ContextKeysClonedAcrossDetach`，且有 `logger.CloneContext` 测试，当前 QA async context 能保留 principal。真正的异步任务 payload 仍应显式携带 resolved scope 和 source IDs。

### 6.3 Memory topic/interest 数据读取入口

类型：

- `internal/types/memory.go` 的 `MemoryTopicStat`，约第 711 行。
- `MemoryTopicStat` 保存 `Topic`、`NormalizedKey`、`Aliases`、`Hits`、`PromotedAt`。

Repository：

- `internal/types/interfaces/memory.go`：`TopTopics`、`TopicByID`、`ListUnpromotedTopics`。
- `internal/application/repository/memory.go`：对应实现。

Service/API：

- `internal/application/service/memory/service.go`：`ListTopics`。
- `internal/handler/memory.go`：`MemoryHandler.ListTopics`。
- `GET /api/v1/memory/topics`，路由在 `internal/router/routes_memory.go`。
- 已提升的长期 interest 以 `MemoryItem(kind=interest)` 存储和读取。

Memory topic mapping 属于 Phase 2B/SHOULD，不进入当前 Phase 1–7 主线。以后若做，必须经过 Learning caller/KB scope，不能直接把 workspace-level Memory topic 匹配到任意 KB Concept。

## 7. Knowledge、Chunk、WikiPage ID 关系

真实关系如下：

```text
Knowledge
  ID
  TenantID
  KnowledgeBaseID
       |
       | Chunk.KnowledgeID
       v
Chunk
  ID                      <- SearchResult.ID
  TenantID
  KnowledgeID             <- SearchResult.KnowledgeID
  KnowledgeBaseID         <- SearchResult.KnowledgeBaseID
       |
       | WikiPage.ChunkRefs JSON array (无 FK)
       v
WikiPage
  ID                      <- 当前页面行 UUID
  KnowledgeBaseID
  Slug
  PageType
  SourceRefs              <- knowledge_id 或 knowledge_id|title
  ChunkRefs               <- source Chunk.ID 数组
```

结论：

- displayed reference 到 Concept 的主 join key 是 `SearchResult.ID == Chunk.ID`，再反查包含该 chunk ID 的 `WikiPage.ChunkRefs`。
- 必须同时约束 tenant、KB 和 `page_type=concept`；不能只按 chunk UUID 扫全库。
- `ChunkRefs` 是 JSON 字段且当前 `WikiPageRepository` 没有 `chunk IDs -> concept pages` 的反向 batch method。
- Phase 2 需要新增 repository batch query，并分别验证 PostgreSQL JSONB 与 SQLite JSON1 语义；禁止每个 chunk 或每个 Concept 发一次查询。
- 一个 chunk 可以出现在多个 Concept 的 `ChunkRefs`，因此每个候选 confidence 使用 `1/N`。
- Graph node 目前以 slug 寻址；Learning 长期状态不能只依赖 slug。

## 8. Concept identity 必要性核验

稳定 `concept_key` 是当前代码下的必要层，不是可选优化。

证据：`internal/agent/tools/wiki_rename_page.go` 的 rename 流程会：

1. 用新 slug 创建一个新的 `WikiPage`；
2. 新页面获得新的 UUID；
3. 重写 incoming links；
4. 软删除旧页面。

Wiki ingest 对同 slug 会更新现有页面，但 rename/slug 漂移可能生成新 page ID。建议 resolver 顺序与锁定决策保持一致：

```text
current_wiki_page_id
  -> normalized slug
  -> aliases / normalized title
  -> ambiguity => orphaned
```

禁止在 title/alias 多义时自动迁移，也不在 MVP 处理 Concept merge/split。

## 9. Migration 入口

Runner：`internal/database/migration.go`

- PostgreSQL 默认目录：`migrations/versioned/`。
- SQLite DSN 使用：`migrations/sqlite/`。
- 当前 PostgreSQL 最新编号：`000084_memory`。
- 当前 SQLite 最新编号：`000011_principal_model`。

若 Phase 1 开始时 main 未新增 migration，下一组建议为：

```text
migrations/versioned/000089_learning_profile.up.sql
migrations/versioned/000089_learning_profile.down.sql
migrations/sqlite/000013_learning_profile.up.sql
migrations/sqlite/000013_learning_profile.down.sql
```

实施前仍须重新列目录，避免并行主干占号。

## 10. Phase 1–7 建议代码落点

### Phase 1：Concept Identity + Data Model

建议新增：

```text
internal/types/learning.go
internal/types/interfaces/learning.go
internal/application/repository/learning.go
internal/application/service/learning/
internal/handler/learning.go
internal/router/routes_learning.go
```

并修改：

```text
internal/container/container.go
internal/router/router.go
migrations/versioned/
migrations/sqlite/
```

Learning API path 建议挂在 caller 可访问的 KB 下，例如：

```text
/api/v1/knowledgebase/:kb_id/wiki/learning/...
```

路由沿用 `Viewer()` + `KBAccessRead("kb_id")`，handler/service 再强制 `web_user` principal；写操作仍只写 caller 自己的 profile。

Phase 1 必须新增 scope-level `learning_profiles`（或等价名称）表：

```text
tenant_id
subject_id
knowledge_base_id
tracking_enabled
enabled_at
cleared_at
created_at
updated_at
```

原因：tracking 开关和 `cleared_at` 属于 subject+KB，不属于单个 Concept。若把它们只存在 `user_concept_state`：

- 尚无 Concept state 时无法保存“已开启”；
- clear 删除 state 后会丢失 `cleared_at`；
- 迟到异步事件无法执行 `occurred_at <= cleared_at` 防回写。

这是进入 Phase 1 前必须落实的数据模型修正。

### Phase 2：Displayed Reference Exposure

主要修改/新增点：

```text
internal/handler/session/qa.go                 # 成功持久化后的统一 hook
internal/application/service/learning/         # exposure orchestration/aggregation
internal/application/repository/learning.go    # evidence 幂等、state 原子聚合
internal/application/repository/wiki_page.go   # chunk IDs -> Concepts batch query
internal/types/interfaces/wiki_page.go         # 对应 repository/service contract
```

不得使用 raw retrieved context 或 `MemoryDocAffinity` 直接更新 learning state。事件必须携带已解析的 caller scope、message/session、KB、knowledge/chunk IDs 和 occurred_at。

### Phase 3：Evidence-bound Quiz Bank

建议新增：

```text
internal/application/service/learning/quiz_bank.go
internal/application/service/learning/quiz_prompt.go
internal/handler/learning_quiz.go（或并入 learning.go）
```

复用：

- `WikiPage.ChunkRefs` 选择 source chunks。
- `ChunkRepository.ListChunksByID` 批量取原文。
- `interfaces.ModelService` 解析模型。
- `internal/models/chat` + `chat.ChatOptions.Format` JSON schema 模式；Memory extract/topic resolver 已有可参考实现。

每道题保存 source chunk IDs、source hash、prompt version 和 model info。生成结果进入服务端 validation，前端不接触 `correct_option`。

### Phase 4：Quiz Attempt + Verified Mastery

主要落点：

```text
internal/application/service/learning/mastery.go
internal/application/repository/learning.go
internal/handler/learning_quiz.go
internal/router/routes_learning.go
```

attempt insert、quiz evidence append、最近有效 attempts 查询、state 更新必须处于同一事务或等价原子边界。后端确定性判分；所有 mastery 写入口集中在该 service。

### Phase 5：Graph Overlay

后端：

```text
internal/handler/learning.go
internal/application/service/learning/overlay.go
internal/application/repository/learning.go
internal/router/routes_learning.go
```

优先独立 API，例如：

```text
GET /api/v1/knowledgebase/:kb_id/wiki/learning-overlay
```

前端：

```text
frontend/src/api/learning.ts（或 frontend/src/api/wiki/learning.ts）
frontend/src/views/knowledge/KnowledgeBase.vue
frontend/src/views/knowledge/wiki/WikiBrowser.vue
frontend/src/i18n/locales/*.ts
```

普通 Graph API、overview/ego、pan/zoom/search/bloom 行为保持不变。个人模式批量 merge overlay，不能 N+1。

### Phase 6：Knowledge MRI Scan

后端：

```text
internal/application/service/learning/scan.go
internal/application/service/learning/quiz_bank.go
internal/handler/learning_scan.go
internal/router/routes_learning.go
```

前端建议新增一个轻量组件，避免继续扩大 `WikiBrowser.vue`：

```text
frontend/src/views/knowledge/wiki/KnowledgeMRIScan.vue
```

由 `WikiBrowser.vue` 在 learning mode 打开 Modal/Drawer。`learning_scans` 持久化 item IDs、current index 和 status，刷新后可恢复。

### Phase 7：Evidence Trail + Recommendation

后端：

```text
internal/application/service/learning/evidence.go
internal/application/service/learning/recommend.go
internal/handler/learning.go
```

前端继续复用当前内联 Graph Drawer，必要时再把 learning card/timeline 抽为：

```text
frontend/src/views/knowledge/wiki/ConceptLearningCard.vue
frontend/src/views/knowledge/wiki/LearningEvidenceTrail.vue
```

推荐候选必须由 deterministic rule 产生；LLM 最多润色 reason。

## 11. 当前代码与执行包的差异

| 项目 | 执行包假设 | 当前代码事实 | 影响 |
|---|---|---|---|
| Concept Drawer | 可复用现有 Concept Drawer | Drawer 内联在 `WikiBrowser.vue`，无独立组件 | Phase 5 直接改内联区域，或仅抽 learning 子卡 |
| Graph 个人化 | 尚未有个人 overlay | 已有 document-affinity `familiar` 外圈 | 可复用后端/前端范式，但必须与 mastery 分离 |
| Graph 数据量 | 复用当前 Graph | 当前已有 top-500 overview + ego/Bloom | Overlay 必须适配子图和增量加载，不能假设一次拿全图 |
| Graph node identity | 可按 page/Concept overlay | API node 只有 slug，无 page ID | 独立 overlay 应返回 page ID + concept key + slug |
| Wiki rename | Page ID 可能变化 | Agent rename 明确创建新 UUID、软删旧页 | 稳定 concept identity 为 MUST |
| Reference 语义 | final displayed reference | 正常 RAG 保存 pre-answer `MergeResult`，inline citations 可关闭 | 只能称弱 exposure，不能称明确阅读/引用 |
| Reverse mapping | chunk 可映射 Concept | 当前无 chunk IDs -> Concept batch repository method | Phase 2 必须补 PostgreSQL/SQLite 查询 |
| Tracking lifecycle | state 可保存开关/clear | clear state 后无法保留 `cleared_at` | Phase 1 必须增加 scope-level profile 表 |

## 12. 风险与处理建议

### R1：Learning profile scope 表缺失（Phase 1 前必须修正）

风险：默认关闭、首次开启、不回填、disable、clear 和 late-event suppression 无法同时正确实现。

处理：增加 `learning_profiles`，唯一键 `(tenant_id, subject_id, knowledge_base_id)`；`cleared_at` 保留在 profile 行，clear 只删除 evidence/state/attempt/scan。

### R2：Shared KB / shared agent tenant 语义

风险：把 effective/owner tenant 写成 learner tenant，导致 profile 不属于 caller。

处理：LearningScope 的 tenant 固定为 session/caller tenant；KB access 仍由现有 RBAC/KBAccessRead 校验；异步 payload 显式保存 scope。

### R3：JSON reverse lookup 跨数据库差异

风险：PostgreSQL JSONB 和 SQLite JSON1 查询语法不同。

处理：repository 内按 GORM Dialector 分支或用兼容策略实现 batch query，并为两种数据库分别测试；禁止 handler 内拼 SQL。

### R4：Graph UI 单文件过大

风险：`WikiBrowser.vue` 已包含 browser、graph、drawer、SVG simulation 和大量交互，Phase 5–7 继续堆逻辑容易回归。

处理：Phase 5 只复用并最小改动；Phase 6 的 scan 和 Phase 7 的 timeline 优先抽轻量子组件，不重写 Graph renderer。

### R5：异步 completion 的保存顺序和重放

风险：message 保存失败却写 evidence，或 completion 重试导致重复 exposure。

处理：先确认 message update 成功，再调 Learning；唯一 idempotency key + transaction/atomic aggregation；late event 比对 profile `cleared_at`。

### R6：现有 familiar 与 learning status 视觉冲突

风险：用户把“常用文档”误解成“已掌握”。

处理：普通图谱保留 familiar；个人知识地图隐藏 familiar 外圈，颜色只表示 learning status。

## 13. 最小基线测试

执行命令：

```bash
go test ./internal/types -run 'Test(PrincipalFromContextFallsBackToWebUser|PrincipalStorageID|WikiSourceKnowledgeID|WikiGraphDataJSON)$' -count=1
go test ./internal/logger -run '^TestCloneContextPreservesPrincipal$' -count=1
go test ./internal/application/service -run 'TestComputeGraphSubset_(OverviewTruncatesByLinkCount|MarksFamiliarSourcePages|OverviewTypeFilter|EgoDepth1|EgoRejectsMissingCenter)$' -count=1
go test ./internal/application/service/memory -run 'Test(ListItemsIsScopedToTheCaller|ExtractionRebuildsScopeFromPayload|ExtractionDroppedWhenPayloadHasNoScope)$' -count=1
go test ./internal/router -run 'TestWiki(ReadRoutesDenyCrossTenantKB|OperationLogRouteIsRemoved|WriteRoutesDenyOutOfScopeAPIKeyKB)$' -count=1
```

结果：全部通过。

Docker 运行基线已核对：`WeKnora-app`（`wechatopenai/weknora-app:latest`）健康，`WeKnora-frontend`（`wechatopenai/weknora-ui:latest`）正常提供 `http://localhost/platform/knowledge-bases`；页面可加载 WeKnora 的知识库/Wiki 相关界面，受保护 API 在无会话请求下按预期返回 `401`。因此，当前容器化产品运行环境不存在前端依赖缺失问题。

源码测试环境与运行镜像需分开记录：生产 UI 镜像是只包含已构建静态资源的 nginx 镜像，本来不包含 Node/npm/tsx；宿主机 checkout 当前也没有 `frontend/node_modules`。所以 Phase 0 只完成了后端最小回归，尚未执行前端源码的 `npm test` / `npm run type-check`。进入涉及前端源码的 Phase 前，应在独立开发/测试环境执行 `npm ci` 后再运行这两项检查；这不是当前 Docker 运行环境的 blocker。

## 14. Phase 0 结论

- 当前 main 支持 Knowledge MRI MVP 所需的 Wiki Concept、Chunk provenance、caller principal、双 migration、assistant reference 持久化和可复用 Graph UI。
- 主执行路线成立：Concept identity -> displayed-reference exposure -> evidence-bound MCQ -> verified mastery -> Graph overlay -> MRI scan。
- 当前没有阻止 Phase 0 完成的代码 blocker。
- 进入 Phase 1 前必须采用 scope-level `learning_profiles`（或等价结构）承载 tracking/clear lifecycle。
- Phase 1 不应修改 MemoryItem、wiki_pages、messages 或 memory_doc_affinity 来保存 mastery。
- 本阶段未修改任何业务代码，未进入 Phase 1。
