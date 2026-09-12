# 运行与演示

课题四 / 张凌浩（zlh123123）。

## 1. 最快检查成果：离线复算

无需登录、无需模型、无需导入资料。评估指标、数据来源和复现边界见 [evaluation/benchmark_plan.md](evaluation/benchmark_plan.md)。阅读 [technical_report.md](technical_report.md) 了解设计和结论。

## 2. 从源码启动交互原型

依赖：Docker Desktop/Engine 与 Compose v2，Node.js 22 和 npm；本地 Go 测试需要 Go 1.26（见 go.mod）。使用本课题源码构建 app/frontend，上游预编译镜像不包含本次修改。

```bash
git clone https://github.com/zlh123123/WeKnora.git
cd WeKnora
git checkout rhino-2026-final-4
cp .env.example .env
```

按 `.env.example` 注释设置数据库密码、JWT/AES 等本机配置，不提交密钥。模型可通过界面添加兼容的对话模型和嵌入模型；需要可用的服务地址与 API Key。Wiki 和出题依赖对话模型，语义检索依赖嵌入模型，离线评估不依赖这些服务。

```bash
./scripts/build_frontend_dist.sh
docker compose build --build-arg GOPROXY_ARG=https://goproxy.cn,direct app frontend
docker compose up -d --pull never
curl --fail http://localhost:8080/health
```

如果基础依赖镜像尚未存在，先按 Compose 内的名称拉取依赖镜像，或首次使用 `docker compose up -d --build`。不要再执行针对 app/frontend 的 `docker compose pull` 覆盖本地构建。入口为 `http://localhost`；端口占用时在 `.env` 调整 FRONTEND_PORT / APP_PORT。构建包含 AnyDoc Rust 组件，首次下载和编译可能较久。

说明：本机已有 Docker 环境构建、启动和浏览器流程已验证；没有在全新机器独立验收。模型提供商与网络会影响首次出题的耗时和可用性。

## 3. 从导入到复测

1. 注册/登录，设置可用对话与嵌入模型；创建文档知识库，在设置中启用 Wiki 并选定模型。
2. 导入 `docs/knowledge_mri/demo/rag_demo.md`，等待解析和 Wiki 生成完成。新演示文档用于快速试跑，生成的节点/题目会随模型变化，不应期待与冻结评估数值相同。
3. 先在知识库图谱入口切换“我的知识地图”并开启学习追踪，再新建对话并选择该知识库，提问“文本分块为什么需要重叠？”。检查回答是否展示来源；无引用时不要期待 Exposure 增加。
4. 返回“我的知识地图”，查看引用展示产生的节点状态。
5. 点击“快速知识扫描”，完成六题；若可用 Concept 不足，先检查 Wiki 是否生成至少三个有有效来源的 Concept。
6. 打开一个 Concept，查看来源、状态和证据，再点击“当前知识点验证”。应显示两题，关闭后再进入应恢复进度。
7. 完成后查看地图及抽屉状态，刷新页面检查持久化；有推荐时打开相关 Wiki 阅读，再复测。
8. 通过个人地图图例中的“导出学习数据”下载画像，旁边有“停用学习追踪”和“清除我的学习数据”。清除和停用用于用户主动管理画像；演示时不要清除自己的重要历史记录。

## 4. 两分钟讲解顺序

- 0:00–0:20：介绍公共知识网络和个人学习状态的分离，打开个人地图。
- 0:20–0:40：打开节点，解释“接触过”来自引用展示，“通过验证”来自判分。
- 0:40–1:20：完成当前知识点两题验证，展示关闭恢复和证据更新。首次生成较慢时先生成题库再开始讲解，说明预热步骤。
- 1:20–1:40：说明推荐是规则排序的下一步巩固建议，不是完整先修路径。
- 1:40–2:00：展示报告中的真实审核结果和局限，以及画像导出/清除入口。

该脚本是演示讲解提纲，不是已录制视频。

## 5. 回归命令

```bash
go test ./... -count=1
go test -race ./internal/application/service/learning ./internal/application/repository -count=1
cd frontend
npm test
npm run type-check
npm run build
```

2026-09-10 已通过全仓 Go、前端 441 项、类型检查和构建（早先功能验收）；最终新增画像导出隔离测试后，再次通过 learning/repository/handler/router 四包。测试数量不等于用户研究。

## 6. 失败处理

- 页面只有上游功能：检查是否使用本地构建，重新构建 frontend/app。
- Wiki 为空：检查资料解析、知识库 Wiki 开关及对话模型配置。
- 冷出题失败：保留状态，确认模型支持所需 JSON 输出与网络正常后重试；不要伪造题目充数。
- 检索回退：检查模型 endpoint 是否是真实地址而非 `${...}` 字面量，查看应用日志中的 embedding 错误。不能把关键词回退作为向量检索成功证据。
- 切换端口后接口异常：核对 frontend 的 APP_HOST/APP_BACKEND_PORT 与 app 实际监听配置。
