# Concept 补全与定向复测验收

日期：2026-09-10。

## 修复的问题

1. 旧代码只在 identity 数量为零时补全；聊天已经生成部分 identity 的知识库会遗漏其余 Concept。
2. 节点的“验证掌握程度”按钮启动全库六题扫描，无法定向测量当前节点。
3. 未完成扫描只按时间取最近一条，无法区分全库扫描与不同 Concept 的复测。
4. 扫描展示逐个查询全库 Concept 的题库；仅展示两题也会产生随 Concept 数增加的查询。
5. 复测完成只刷新地图，没有刷新已打开的证据抽屉；快速点击也缺少提交中的重入保护。

## 当前实现

- 打开个人地图或创建全库扫描时，补全缺失的已发布 Concept identity；不重置已有学习状态。
- 已有页面 key 保留；另一仍存在的页面即使同名或共享别名，也不能抢占该 key。旧页面消失后的重建仍沿用原有 resolver。
- `POST /api/v1/knowledgebase/:kb_id/learning/concepts/:concept_key/scans` 创建或恢复当前 Concept 两题复测。
- 全库六题扫描与各 Concept 两题复测独立恢复；目标从已保存的题目归属判断，不添加数据库列，不迁移或清理历史扫描。
- 复用原有题库、来源约束、判分、幂等、事务和掌握度聚合；新一轮优先选未答题目。
- 扫描题目通过 tenant + KB + item IDs 批量加载，只向前端返回安全 DTO。
- 有限数量的进程内锁串行化同 KB 的扫描创建和身份初始化，避免同进程重复创建。此措施不是多实例分布式锁。
- 复测完成同步刷新 overlay 和当前 Drawer insights；前端阻止加载/提交中的重复操作。

## 自动验证

新增测试覆盖：

- 全无、部分已有、后续新增的 Concept identity 补全及重复补全幂等。
- 打开个人地图即可补全，同时不创建扫描。
- 同名现存页面不会抢占原 identity。
- 定向两题的归属、恢复、完成和只更新目标 Concept。
- 全库扫描、不同 Concept 复测互不覆盖。
- 下一轮优先选未答题；非法 Concept、其他 KB、tracking disabled 被拒绝。
- 同进程并发创建同一 Concept 复测只产生一个 scan。
- HTTP handler 采用路径 Concept，不接受请求体中的伪造 Concept/用户/租户范围。

验证命令：

```bash
go test ./internal/application/service/learning ./internal/application/repository ./internal/handler ./internal/router -count=1
go test -race ./internal/application/service/learning ./internal/application/repository -count=1
go test ./... -count=1
cd frontend
npm test
npm run type-check
npm run build
```

## 真实浏览器与持久化证据

验收环境：本机 Docker、PostgreSQL、Edge，使用已有“测试wiki”知识库。不清理用户原有记录。

- RAG 节点按钮显示“当前知识点验证”，题量为 `1 / 2`。
- 第一题提交后显示 `2 / 2`；关闭后重新从节点进入，仍恢复第二题。
- 完成两题后摘要为 strong 1、weak 0、uncertain 0。
- Drawer 自动出现本次两条证据，最近验证时间更新为 2026-09-10 12:38:36（北京时间）。
- 数据库新 scan 为 `completed/current_index=2/total_items=2`。
- 原 2026-08-31 的全库 scan 仍为 `pending/current_index=0/total_items=6`；刷新页面后全库入口显示 `1 / 6`，没有被复测替换。
- 此次答题是功能验收，不是用户学习效果实验，也不能作为题目质量通过率。

## 仍需后续完成

- 真实题目质量审核：验收中的一个 RAG 题目询问虚拟环境名称，有来源但对核心概念的测量价值较弱。来源绑定不能替代人工相关性与唯一正确项审核。
- 跨实例并发、来源变化时的扫描失效恢复、完整删除/导出生命周期及规模评估仍属于整体收尾清单，不在本次结果中宣称完成。
- 原前端测试主要包含源码约束检查，因此本次另行执行真实浏览器操作；测试数量不能替代用户验收。
