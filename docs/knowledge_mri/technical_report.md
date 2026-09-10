# 课题四：知识网络与引导式学习

## 1. 项目摘要

WeKnora 已有两类能力：Wiki 会把知识库文档整理成互链页面和知识图谱，系统也能从对话中保存长期记忆。本项目选择 Wiki Concept 作为学习节点，在现有图谱上增加一个按用户隔离的学习状态层，用可追溯的交互证据记录“接触过什么”，再用基于原始 Chunk 的四选一题验证“当前验证中会什么”。

项目的核心判断是：**引用展示可以作为弱 Exposure 信号，但不能当作掌握证明。** 掌握状态只由服务端对 evidence-bound Quiz Attempt 确定性判分后更新。这样可以把知识网络、用户状态和验证证据分开，避免 LLM 或使用频率直接改写学习结论。

当前代码版本：`9c1d1f15`，远端分支：`origin/main`。

## 2. 用户完整流程

1. 用户创建或打开一个包含 Wiki 的知识库。
2. 用户正常进行带知识库引用的问答。只有已经持久化并完成展示的最终 Assistant Message 引用，才会写入 `displayed_reference` 证据。
3. 用户从“图谱”下拉切换到“我的知识地图”。公共 Wiki Graph 不变，Concept 节点叠加当前用户的学习状态。
4. 用户打开 Concept Drawer，查看状态、接触次数、验证结果、证据时间线和下一步建议。
5. 用户可以启动“快速知识扫描”：系统按确定性规则选 3 个 Concept，每个 Concept 选择 2 道题，共 6 道题。
6. 用户也可以从当前 Concept 直接启动“当前知识点验证”：系统创建或恢复该 Concept 的 2 道题复测。
7. 每题由服务端判分，答题记录、证据和状态投影在同一事务内更新。重复提交不会重复计数。
8. 完成后重新加载地图和当前 Drawer，用户可以看到 strong、weak 或 uncertain 状态以及证据变化。
9. 用户可以打开推荐的 Wiki Concept，阅读后再次启动当前 Concept 的两题复测。
10. 用户可以导出自己的学习画像，或清除/停用当前知识库范围内的学习数据。

## 3. 架构与数据模型

学习层独立于长期 Memory，范围固定为：

```text
tenant_id + subject_id + knowledge_base_id
```

`subject_id` 由服务端从已登录 Web principal 的 `Principal.StorageID()` 推导，客户端不能选择其他用户。

主要数据表：

| 表 | 作用 |
|---|---|
| `learning_profiles` | 当前用户/知识库的追踪开关和清除时间 |
| `learning_concept_identities` | Wiki Concept 与稳定 `concept_key` 的映射 |
| `learning_evidence` | 引用展示和答题验证的 append-only 证据 |
| `user_concept_states` | Exposure、Mastery、Confidence 和状态投影 |
| `quiz_items` | 绑定 Concept、Chunk、source hash 的题库 |
| `quiz_attempts` | 用户答题、判分、扫描和幂等记录 |
| `learning_scans` | 六题扫描或两题定向复测的恢复状态 |

个人地图是公共 Wiki Graph 的 overlay。`unseen`、`exposed`、`uncertain`、`verified_strong`、`verified_weak` 只表示当前用户在当前知识库范围内的状态。

## 4. 关键设计

### Exposure

最终 Assistant Message 的 `KnowledgeReferences` 先校验知识库和 Chunk，再映射到引用这些 Chunk 的 Concept。一个 Chunk 关联多个 Concept 时，每个 Concept 获得 `1/N` 的 exposure confidence。检索候选、原始上下文、打开 Wiki 页面或点击引用不会自动产生 Exposure。

### Quiz grounding

题目从当前已发布 Concept 的有效文本 Chunk 生成。服务端要求严格 JSON、恰好四个不同选项、唯一答案索引、非空解释、来源 Chunk 属于输入集合、题目不重复，并保存 `source_hash`。题目前端 DTO 不返回正确答案。

### Mastery

服务端使用当前 source hash、非 stale 题目的最近四次有效作答：少于两次为 `uncertain`；正确率不低于 0.80 为 `verified_strong`；不高于 0.40 为 `verified_weak`；其余为 `uncertain`。`mastery_confidence` 表示有效作答数量相对于四次窗口的比例，不是统计学置信概率。

Exposure 永远不会覆盖已验证掌握，LLM/Agent 没有直接写入 mastery delta 的入口。

### 扫描与复测

全库扫描按以下优先级排序：

```text
0.50 * (1 - mastery_confidence)
+ 0.35 * normalized(exposure_weight)
+ 0.15 * normalized(graph_degree)
```

已接触或已验证但不确定的 Concept 优先于未接触 Concept。扫描状态由服务端保存，按 `scan_id + item_id` 幂等推进。

当前 Concept 复测使用独立的两题 scan。全库六题扫描、不同 Concept 复测分别恢复，互不覆盖。打开个人地图时会补全缺失的已发布 Concept identity；同名或共享别名的另一个仍存页面不能抢占已有 identity。

### Recommendation

推荐候选由规则确定，不由 LLM 选择。当前 Concept 排除，`verified_weak`、`uncertain`、`exposed` 按状态、接触次数和稳定 key 排序，最多推荐一个下一知识点。推荐是巩固建议，不宣称已经构建完整 prerequisite 学习路径。

## 5. 隐私与生命周期

- 所有读取和写入都从服务端 caller scope 推导用户身份。
- 导出只包含当前用户和当前知识库的数据，不包含题库正确答案。
- 清除会删除当前 scope 的状态、证据、作答和扫描，并保留 `cleared_at` 拒绝迟到事件。
- 停用后不接受新的 Exposure 或 Quiz 写入。
- KB 和租户删除路径包含学习表清理；仍需在实际部署的软删除保留策略下做数据库级物理清理验收。
- Wiki 重建保留可解析的稳定 identity，无法解析的 identity 标记为 orphaned 并从 overlay 隐藏。

## 6. 评估方法与当前结果

### 工程正确性

已执行并通过：

```bash
go test ./... -count=1
go test -race ./internal/application/service/learning ./internal/application/repository -count=1
cd frontend && npm test
cd frontend && npm run type-check
cd frontend && npm run build
git diff --check
```

前端测试为 441 项。相关测试覆盖身份补全、状态隔离、Evidence 幂等、题目来源约束、判分阈值、扫描恢复、定向复测、并发创建和 handler 路径参数边界。

### 真实浏览器验收

2026-09-10 在本机 Docker、PostgreSQL、Edge 的“测试wiki”知识库中完成：

- 个人地图入口和状态图例可见。
- RAG 节点打开“当前知识点验证”，题数显示 `1 / 2`。
- 第一题提交后显示 `2 / 2`；关闭再打开恢复第二题。
- 完成后摘要显示 strong 1、weak 0、uncertain 0。
- 当前 Drawer 的最近验证时间和两条答题证据立即刷新。
- 原有六题扫描仍保持独立的 `pending/current_index=0/total_items=6` 状态。

详细记录见 [retest_checkpoint_2026-09-10.md](retest_checkpoint_2026-09-10.md)。这次答题用于功能验收，不作为学习效果样本。

### 模拟评估

脚本：`scripts/knowledge_mri_eval.py`，随机种子 `20260829`，500 个合成 Concept、每个 6 次作答。结果为：Exposure-only Brier `0.3026`，Verified mastery × confidence Brier `0.2003`。

该结果只说明在脚本设定的 latent mastery 过程下，当前状态规则可以比独立的 Exposure 基线更接近下一次答题结果。Exposure 在脚本中与 latent mastery 独立生成，因此该结果不能外推为真实用户效果，也不能替代人工题目审核或用户试用。

### 尚未完成的有效性证据

- 尚未完成 20–30 个真实 Concept-Chunk 映射的人工抽样审核。
- 尚未完成 30–50 道真实生成题目的 grounding、唯一答案、材料外事实和重复性审核。
- 尚未完成 3–5 人定性试用；若无法招募，应在提交材料中明确写成“无用户研究”。
- 尚未有足够自然用户作答数据支持真实 Brier、Accuracy 或 AUC 结论。

## 7. 已知限制

Exposure 只能表示系统展示了引用，不能证明用户阅读或理解。两题复测是轻量测量探针，不代表对 Concept 的全面掌握。题库复用和模型生成质量仍需要人工抽样控制。当前推荐不使用 typed prerequisite、SM-2、时间衰减或 GraphRAG 学习边。长期 Memory 尚未与 Learning Profile 合并，这属于明确的后续方向而不是本次 MVP 的隐含能力。

运行环境曾出现语义检索 endpoint 配置未展开和 SiliconFlow embedding EOF，导致部分请求回退到关键词检索并增加冷启动时间。提交前应按固定 Demo 配置重新核验；不能把回退环境下的成功问答当作完整语义检索质量证据。

## 8. 复现与提交

固定评估：

```bash
cd /path/to/WeKnora
python3 scripts/knowledge_mri_eval.py --seed 20260829 --concepts 500 --attempts 6
```

完整部署、测试、Demo、截图、最终 Tag 和 `submission.yaml` 的检查项见 [completion_checklist.md](completion_checklist.md)。

最终提交时应同时提供：代码仓库链接、最终 Tag 和完整 SHA、本技术报告 PDF/DOCX 或可访问链接、评估材料、运行说明和已知问题。不要把模拟结果表述为用户研究结论。
