# Phase 6 checkpoint — Knowledge MRI scan

更新时间：2026-08-25（Asia/Shanghai）

## 实现状态

Phase 6 的扫描闭环已实现并部署：

- 服务端 heuristic candidate selection：`0.50 * uncertainty + 0.35 * exposure + 0.15 * graph degree`。
- 仅选择 learning-eligible concept 且必须能解析到有效 source chunks。
- 每个 Concept 选择 2 道未作答题；题库不足时懒生成，生成失败时跳过该 Concept 并继续补足候选。
- 默认 3 个 Concept、共 6 道题。
- `learning_scans` 保存 item IDs、当前进度和状态；`quiz_attempts.scan_id` 绑定上下文并支持幂等提交。
- 前端在“我的知识地图”内使用 Modal 完成六题，不新增顶层 Quiz 页面。
- 完成后返回 strong / weak / uncertain 摘要，并刷新 overlay。

## 自动化证据

通过：

```text
go test ./internal/application/service/learning ./internal/handler ./internal/router/... ./internal/database/...
npm run type-check
npm test
npm run build
```

最终前端结果：`409 passed, 0 failed`。

Phase 6 前端测试覆盖：

- learning mode 暴露可恢复 scan；
- scan 使用 start/resume、active、complete API；
- 不新增顶层页面。

## 实际部署证据

已执行：

```text
docker compose build app frontend
docker compose up -d app frontend
```

结果：

- `WeKnora-app` healthy，监听 `:8080`。
- `WeKnora-frontend` running，监听 `:80`。
- `GET http://127.0.0.1:8080/health` 返回 `{"status":"ok"}`。
- 未认证访问 MRI scan API 返回 `401`，说明路由已注册且认证保护生效。

## 真实浏览器验收

2026-08-25 在已登录的本地浏览器中，使用 `测试wiki`
（`379a24f9-9443-4247-aae7-3d41b0bf3bb4`）完成了一次真实六题扫描：

- RAG 两题全对 -> `verified_strong`；
- 嵌入模型两题全错 -> `verified_weak`；
- 文本分块一对一错 -> `uncertain`。

完成页正确显示：验证掌握 1、验证薄弱 1、待确认 1。数据库对应为
6 个 attempt、6 个 quiz evidence、1 个 completed scan（`current_index=6`）
和 3 个 state；三种状态的 mastery/confidence 分别为 `1/0.5`、
`0/0.5`、`0.5/0.5`。关闭完成页后 overlay 已刷新。

真实测试还发现并修复了一个此前源码断言未覆盖的问题：空 active-scan
响应为 `{data:null}`，旧的 `data || response` 解包会把 wrapper 当成 scan，
导致点击扫描时 Vue 读取不存在的 `items`。现在按 `data` 字段是否存在来
解包，并新增回归测试。

按照用户之前“不保留验收 fixture”的要求，验收结束后已删除本轮产生的
6 个 attempt、6 个 evidence、1 个 scan 和 3 个 state，同时更新
`learning_profiles.cleared_at`。tracking、3 个 concept identity 和 12 道
quiz item 保留；当前个人学习数据再次为零。
