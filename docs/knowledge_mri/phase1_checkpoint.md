# Knowledge MRI Phase 1 Checkpoint

## 1. Phase

`PHASE 1 COMPLETE`

## 2. 本阶段目标

建立独立于 Memory/Wiki/messages 的 Knowledge MRI 数据地基，并锁定 caller、tenant、KB 隔离以及 tracking/clear 生命周期。

## 3. 修改文件

- `internal/types/learning.go` -> 定义 profile、Concept identity、evidence、Concept state、Quiz item/attempt、scan 类型与状态常量 -> 为后续阶段提供稳定领域模型。
- `internal/types/interfaces/learning.go` -> 定义显式 `LearningScope`、identity resolver 结果、repository/service 契约 -> 防止客户端传 subject 选择他人画像。
- `internal/application/repository/learning.go` -> 实现 scope query、tracking、clear、identity resolver、state 读写和 evidence 幂等落库 -> 集中实现数据隔离与生命周期规则。
- `internal/application/service/learning/service.go` -> 从 context 的 tenant 和 `Principal.StorageID()` 推导 scope，并限制 MVP 只允许 `web_user` -> 落实 caller-scoped 产品口径。
- `internal/application/repository/learning_test.go` -> 覆盖 tenant/subject/KB 隔离、幂等、tracking、clear、late event 和 resolver -> 固化核心数据保证。
- `internal/application/service/learning/service_test.go` -> 覆盖 Web principal 推导、非 Web principal 拒绝和歧义 orphan -> 固化产品入口规则。
- `migrations/versioned/000089_learning_profile.{up,down}.sql` -> PostgreSQL 七表迁移及回滚。
- `migrations/sqlite/000013_learning_profile.{up,down}.sql` -> SQLite 对等七表迁移及回滚。
- `internal/database/migration_sqlite_versioned_schema_test.go` -> 将 SQLite 预期版本提升到 12，并检查七张 Learning 表。
- `docs/knowledge_mri/phase0_checkpoint.md` -> 补齐已获人工确认的 Phase 0 checkpoint。
- `docs/knowledge_mri/phase1_checkpoint.md` -> 保存本阶段结果。

## 4. 数据库 / API 变化

新增表：

| 表 | Scope / identity | 作用 |
|---|---|---|
| `learning_profiles` | tenant + subject + KB UNIQUE | tracking 默认关闭、`enabled_at`、`cleared_at`；clear 后保留 |
| `learning_concept_identities` | tenant + KB + stable `concept_key` | Wiki Concept 稳定身份、slug/title/aliases、active/orphaned |
| `learning_evidence` | tenant + subject + KB | append-only evidence；全局 idempotency key 唯一 |
| `user_concept_states` | tenant + subject + KB + concept UNIQUE | exposure/mastery/status 聚合槽位 |
| `quiz_items` | tenant + KB + concept | Phase 3 使用的 evidence-bound item bank 槽位 |
| `quiz_attempts` | tenant + subject + KB | Phase 4 使用的确定性答题记录；关联 scan |
| `learning_scans` | tenant + subject + KB | Phase 6 使用的可恢复 scan 进度 |

API：NONE。本阶段没有提前暴露正式 Quiz、Graph overlay 或 debug API，也没有新增前端入口。

## 5. Resolver 行为

顺序固定为：

```text
current_wiki_page_id
  -> normalized slug
  -> normalized alias
  -> normalized title
  -> unresolved
```

- 唯一命中：复用原 `concept_key` 并更新当前 Wiki page/slug/title/aliases。
- 首次发现且无命中：生成新的 immutable UUID `concept_key`，状态为 `active`。
- alias/title 出现多个候选：不自动迁移到任一旧 Concept；为当前页面生成新 identity 并标记 `orphaned`。
- normalization：slug 去首尾空白与 `/` 并转小写；title/alias 合并空白并转小写。

## 6. Tracking / Clear 行为

- 首次读取 profile 创建 `tracking_enabled = false` 的 scope 行，不回填历史数据。
- false -> true 时记录新的 `enabled_at`；disable 后 evidence append 直接丢弃。
- evidence `occurred_at < enabled_at` 时丢弃。
- clear 在同一事务删除当前 scope 的 evidence、state、attempt、scan，保留 profile 并写 `cleared_at`。
- clear 保留用户当前 tracking 选择；`occurred_at <= cleared_at` 的迟到事件丢弃，之后的新事件仍可进入。
- Concept identity 与 Quiz item 是 tenant + KB 资源，不随某个用户 clear 删除。

## 7. 自动测试

通过：

```bash
go test ./internal/application/repository ./internal/application/service/learning ./internal/database -count=1
go vet ./internal/application/repository ./internal/application/service/learning ./internal/types ./internal/types/interfaces
git diff --check
```

覆盖结果：

- A 用户看不到 B 用户状态。
- A tenant 看不到 B tenant。
- 同 subject 不同 KB 不串数据。
- evidence idempotency key 重复只保留一行。
- tracking 默认关闭，disable 拒绝新事件。
- clear 只删除当前 subject + KB 数据并保留 `cleared_at`。
- `occurred_at <= cleared_at` 的迟到事件无法重新写回。
- page ID、slug、alias、title、unresolved、ambiguous/orphaned resolver 路径均通过。
- SQLite 从空库迁移到 version 12，七张 Learning 表存在。

PostgreSQL 实际验证：

- 在独立临时数据库 `weknora_phase1_migration_20260823_codex` 执行 `000085` up migration，七张表全部创建。
- 执行 down migration，七张表全部删除。
- 验证后已删除该临时数据库；未修改当前 WeKnora 运行数据库。

全仓 `go test ./... -count=1` 已在当前 Fake-IP DNS 环境中通过。测试不再依赖本机对公共示例域名的真实解析：SSRF 核心测试使用确定的公网解析结果，各业务测试只在自身测试进程/用例范围内放行明确的示例域名。生产 SSRF 策略未放宽，`198.18.0.0/15` 仍为受限网段。另修复了 `LOG_FORMAT=json` 被误当作字面模板的问题，Feishu Wiki 日志断言恢复通过。

## 8. 人工测试步骤

1. 在仓库根目录运行 `go test ./internal/application/repository ./internal/application/service/learning -count=1 -v`。
2. 运行 `go test ./internal/database -run '^TestSQLiteMigrationsCreateVersionedSchema$' -count=1 -v`，确认版本为 12。
3. 在一个隔离测试数据库执行 PostgreSQL `000089_learning_profile.up.sql`。
4. 查询 `learning_profiles`，确认新 scope 默认 `tracking_enabled = false`。
5. 为同 tenant/KB 写入两个不同 `subject_id` 的 state，分别按完整 scope 查询，确认互不可见。
6. 对一个 scope 执行 clear，确认该 scope 的 evidence/state/attempt/scan 删除、另一个用户保留，且 profile 的 `cleared_at` 非空。
7. 在测试数据库执行对应 down migration，确认七张表可完整回滚。

## 9. 实际结果

- PostgreSQL up/down：PASS。
- SQLite fresh migration to v12：PASS。
- Learning repository/service tests：PASS。
- Full repository `go test ./... -count=1`：PASS。
- Fake-IP DNS regression 与 JSON logger regression：PASS。
- Targeted vet 与 diff check：PASS。
- 当前 Docker 产品容器未重建、当前业务数据库未迁移；本阶段只修改源码与 migration 文件。

## 10. 本阶段没有做什么

- 没有采集 `displayed_reference`。
- 没有把 evidence 聚合为 exposure。
- 没有生成 Quiz、提交 attempt 或计算 mastery。
- 没有 Learning API、handler、router、container wiring 或前端 UI。
- 没有修改 MemoryItem、wiki_pages、messages、memory_doc_affinity。

## 11. 已知问题 / 风险

- Identity service 尚未挂入 Wiki ingest/rebuild；这是后续调用点，不应在 Phase 1 偷接 Exposure/UI。
- alias resolver 为保持 PostgreSQL/SQLite 一致，目前在 KB scope 内加载 identity 后做确定性规范化匹配；MVP Concept 规模可接受，超大 Wiki 后续可增加规范化 alias 索引表。
- 当前本机使用 Fake-IP DNS；相关 SSRF 测试已改为确定性解析并纳入全仓回归门禁，生产仍严格拦截 `198.18.0.0/15`。

## 12. 下一阶段前建议人工确认

- 确认七表命名和 scope-level `learning_profiles` 可接受。
- 确认 clear 保留 tracking 开关、仅重置数据并记录 `cleared_at` 的语义。
- 确认 ambiguous identity 创建 orphaned 行、等待人工/后续 reconcile，而不是自动绑定候选。
- 确认下一阶段只接 `displayed_reference -> chunk -> candidate concepts`，不接 Memory topic mapping。

## 13. STOP

```text
Stopped after Phase 1.
Waiting for manual review.
I will not start Phase 2 until explicitly requested.
```
