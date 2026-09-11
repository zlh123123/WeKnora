# 冻结数据与审核说明

日期：2026-09-10。该目录属于作者侧 AI 辅助离线审核，不是独立人工标注，也没有用户研究。

- snapshot.json：从本机 PostgreSQL 两个公开教材知识库只读导出。KB1 为 all-in-rag 前三节（3 文档）；KB2 为 d2l-zh 计算机视觉材料（30 文档，含中文与 `_origin.md` 版本，存在内容重叠）。
- review.json：Codex 逐条对照冻结片段给出的 yes/no/uncertain 与理由；uncertain 不计通过。
- results.json：审核脚本机械聚合的计数。
- sensitivity.json：纯合成状态规则敏感性实验。没有使用真实用户答题记录。

## 复算

在仓库根目录运行（仅 Python 3 标准库，无需模型与数据库）：

```bash
python3 scripts/knowledge_mri_audit.py report --input docs/knowledge_mri/evaluation/snapshot.json --review docs/knowledge_mri/evaluation/review.json --output /tmp/mri-audit.json
python3 scripts/knowledge_mri_sensitivity.py --seed 20260910 --samples 10000 --output /tmp/mri-sensitivity.json
diff /tmp/mri-audit.json docs/knowledge_mri/evaluation/results.json
diff /tmp/mri-sensitivity.json docs/knowledge_mri/evaluation/sensitivity.json
```

重新导出需要本机 PostgreSQL Docker 容器和自己有权访问的 KB UUID。命令为 `python3 scripts/knowledge_mri_audit.py export --kb YOUR_KB_UUID --output /tmp/new-snapshot.json`。数据库变化后样本会变化；本提交的精确复算应使用冻结文件。

抽样单位是 Concept-Chunk 引用对，每库无放回随机抽 15 对，种子 20260910；题目取当时全部 active 且 non-stale 的 24 道，涉及六个 Concept，并非从生成历史随机抽题。失败生成、已禁用题不在分母，因此不能推导生成成功率。历史生成模型 ID/版本不在 quiz_items 中保存，不能可靠追溯；仅保存实际 prompt_version 与 source_hash。

映射的 yes 表示片段独立支持至少一个实质属性，并不表示整个 Concept 摘要都受该片段支持。结构可解析率与语义支持率分开报告。题目按来源支持、答案无歧义、概念相关、不重复四项审核；跨 Concept 的同义题也计重复。单一 AI 审核可能误判，保留完整材料便于评审复核。

## 语料来源与许可

KB1 片段来自 Datawhale [all-in-rag](https://github.com/datawhalechina/all-in-rag)，原仓库 README 声明 CC BY-NC-SA 4.0。本目录 snapshot.json 中 KB1 片段及其整理适用 [CC BY-NC-SA 4.0](https://creativecommons.org/licenses/by-nc-sa/4.0/)，仅用于非商业课程评估，保留署名与相同方式共享条件；不将其改授本项目代码许可。

KB2 片段来自 [d2l-ai/d2l-zh](https://github.com/d2l-ai/d2l-zh)（《动手学深度学习》）。保留本地检出的 LICENSE 副本为 D2L-LICENSE.txt；原书作者及贡献者保有相关权利。中英文/预处理版本混合，不视为独立数据集。

导出未包含账户、密码、模型密钥、聊天正文或用户学习画像。题目答案仅位于离线审核文件，面向审阅者；产品做题接口仍不预先返回答案。
