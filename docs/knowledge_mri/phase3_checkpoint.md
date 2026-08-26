# Knowledge MRI Phase 3 Checkpoint — Evidence-bound MCQ Quiz Bank

Date: 2026-08-23
Phase plan: `WeKnora_Knowledge_MRI_Codex_Phased_Plan_2026-08-23/04_PHASE_3_QUIZ_BANK.md`

## Outcome

Phase 3 is complete. A caller with read access to a knowledge base can lazily
build or read a four-item MCQ bank for a stable learning concept:

```text
learning concept identity
  -> current published Wiki concept page
  -> page ChunkRefs
  -> 1–4 enabled text chunks from the owning KB (bounded to 14,000 runes)
  -> KB summary chat model
  -> deterministic JSON validation
  -> one transactional quiz_items write
```

Only single-answer multiple choice is implemented. Every stored item has four
distinct, non-empty options. Answer submission, mastery updates, MRI scans and
frontend work remain out of scope.

## API and caller scope

```http
GET  /api/v1/knowledgebase/:kb_id/learning/concepts/:concept_key/quiz-items
POST /api/v1/knowledgebase/:kb_id/learning/concepts/:concept_key/quiz-items/generate
```

- There is no tenant or subject request field.
- Both routes require a signed-in Web principal plus KB read permission.
- The quiz bank is a KB-level derived resource, while the route remains
  permission-scoped to its caller.
- Pre-answer responses intentionally omit both `correct_option` and
  `explanation`.
- A live request from a different user without access to the test KB returned
  HTTP 403 (`Permission denied to access this knowledge base`). The temporary
  access token created for this check was revoked immediately afterward.

## Grounding and selection

The service resolves the KB owner tenant before reading Wiki pages, chunks or
models. It rejects a concept unless its identity, current Wiki page and KB all
agree. ChunkRefs are deduplicated, restricted to enabled text chunks in that
same KB/tenant, ordered shortest-first (all candidates are already direct Wiki
citations), and capped at four chunks / 14,000 runes. A single sufficiently
substantive direct chunk is allowed because many real Concept pages currently
have exactly one ChunkRef; empty or less than 80 runes returns
`cannot_generate` rather than inviting the model to improvise.

`source_hash` is SHA-256 over the selected chunk IDs, content revisions and
trimmed contents. When that snapshot changes, the previous active bank is
marked stale in the same transaction that writes the replacement bank.

## Real prompt summary

Prompt version: `knowledge-mri-mcq-v1`

System instructions require the model to:

- use only the supplied chunks and no outside facts as correct answers;
- return one JSON object only;
- generate exactly the requested number of questions;
- produce exactly four distinct options and one zero-based correct index;
- explain why the answer follows from the cited chunks; and
- copy every `source_chunk_ids` value from the supplied chunk IDs.

The user prompt includes the requested count, Concept title and summary, then
bounded blocks in this form:

```text
<SOURCE_CHUNK id="<uuid>">
<real chunk text>
</SOURCE_CHUNK>
```

Generation uses the KB summary model with temperature `0.2`, thinking disabled,
a JSON response format/schema, and at most 1,800 completion tokens. A rejected
response is retried with the deterministic rejection reason, up to three total
attempts.

## Model output and persistence schema

Expected model JSON:

```json
{
  "items": [
    {
      "question": "string",
      "options": ["A", "B", "C", "D"],
      "correct_option": 0,
      "explanation": "string",
      "source_chunk_ids": ["input-chunk-uuid"],
      "difficulty": "easy|medium|hard"
    }
  ]
}
```

Stored fields include concept key, Wiki page ID, question, normalized question
hash, options, correct option, explanation, source chunk IDs, source snapshot
hash, prompt version, model ID/name/token usage/attempt, difficulty and
active/stale flags. Migration `000090` / SQLite `000014` adds the composite
unique key `(tenant_id, knowledge_base_id, concept_key, question_hash)`.

No item is written unless the complete generated batch passes:

- parseable JSON and exact requested item count;
- non-empty question and explanation;
- exactly four non-empty, normalized-distinct options;
- `correct_option` in `0..3`;
- non-empty source IDs, all a subset of prompt input chunks; and
- no normalized question duplicate against current/stale stored items or the
  same generated batch.

## Live sample run

Knowledge base: `测试wiki`
KB ID: `379a24f9-9443-4247-aae7-3d41b0bf3bb4`
Model: `deepseek-v4-pro` (`2f7b8504-7883-4c2c-b17a-cba91449b1a1`)

The following are the actual 12 rows persisted by the live Phase 3 endpoint.
Correct indexes are zero-based.

### RAG

1. **根据提供的资料，在四步构建 RAG 的过程中，可以用于实现第一个 RAG 应用的框架有哪些？**
   - Options: `只有 LangChain` / `只有 LlamaIndex` / `LangChain 和 LlamaIndex` / `TensorFlow 和 PyTorch`
   - Correct: `2 — LangChain 和 LlamaIndex`
   - Explanation: 资料明确说将使用 LangChain 和 LlamaIndex 完成第一个 RAG 应用。
   - Source `034704c5-268f-4d67-b2e0-8f325f198b3e`: “接下来将尝试使用 LangChain 和 LlamaIndex 框架完成第一个 RAG 应用的实现与运行。”

2. **在基于 LangChain 框架实现 RAG 应用时，构建最小可行系统的四个步骤是什么？**
   - Options: `数据收集、模型训练、模型评估、模型部署` / `数据准备、索引构建、检索优化、生成集成` / `文本加载、文本分割、文本嵌入、文本生成` / `数据清洗、特征提取、模型选择、结果输出`
   - Correct: `1 — 数据准备、索引构建、检索优化、生成集成`
   - Explanation: 原文直接列出这四步。
   - Source `80ab7ad9-7fa0-42d2-ad2c-b217645b794c`: “四步构建最小可行系统分别是数据准备、索引构建、检索优化和生成集成。”

3. **关于 LlamaIndex 在 RAG 方面的特点，以下哪项描述最符合资料内容？**
   - Options: `更少封装好的 API，增加上手门槛` / `更多封装好的 API，降低上手门槛` / `不提供封装 API` / `只适用于低代码且无法完成复杂应用`
   - Correct: `1 — 更多封装好的 API，降低上手门槛`
   - Explanation: 与原文表述完全一致。
   - Source `ca98842e-f33f-4695-bf61-4e7d766f9353`: “LlamaIndex 提供了更多封装好的 API 接口，这无疑降低了上手门槛。”

4. **根据资料，在四步构建 RAG 的过程中，启动虚拟环境时创建的环境名称是什么？**
   - Options: `all-in-rag` / `rag-environment` / `langchain-env` / `llamaindex-env`
   - Correct: `0 — all-in-rag`
   - Explanation: 原文明确给出虚拟环境名。
   - Source `034704c5-268f-4d67-b2e0-8f325f198b3e`: “创建了名为 `all-in-rag` 的虚拟环境。”

### 嵌入模型

5. **在构建向量存储时，嵌入模型的作用是什么？**
   - Options: `将分割后的文本块转换为数值向量表示` / `直接回答查询` / `不转换地存储原文` / `生成摘要替代原文`
   - Correct: `0 — 将分割后的文本块转换为数值向量表示`
   - Explanation: 原文说明文本块先经嵌入模型转换为向量。
   - Source `6f49d1e0-cf2b-4173-b824-fb60e85dfc2f`: “将分割后的文本块（texts）通过初始化好的嵌入模型转换为向量表示。”

6. **在初始化中文嵌入模型 BAAI/bge-small-zh-v1.5 时，启用归一化的参数是哪个？**
   - Options: `normalize_embeddings: True` / `device: cpu` / `model_name: BAAI/bge-small-zh-v1.5` / `is_chat_model: True`
   - Correct: `0 — normalize_embeddings: True`
   - Explanation: 代码中的 `encode_kwargs` 明确启用了该参数。
   - Source `05437d18-ac90-4a1f-8d3b-f34e90376d58`: “启用嵌入归一化 (`normalize_embeddings: True`)。”

7. **使用 InMemoryVectorStore 构建向量索引时，以下哪项描述是正确的？**
   - Options: `先初始化嵌入模型，再转换文本并加入向量存储` / `向量存储自动下载嵌入模型` / `不能存储对应原文` / `添加文档前无需分割`
   - Correct: `0 — 先初始化嵌入模型，再转换文本并加入向量存储`
   - Explanation: 与原文描述的构建顺序一致。
   - Source `6f49d1e0-cf2b-4173-b824-fb60e85dfc2f`: “使用 InMemoryVectorStore 将这些向量及其对应的原始文本内容添加进去。”

8. **在 RAG 系统中，以下哪个代码片段正确设置了嵌入模型？**
   - Options: `Settings.embed_model = HuggingFaceEmbedding("BAAI/bge-small-zh-v1.5")` / `Settings.llm = HuggingFaceEmbedding(...)` / `Settings.embed_model = OpenAILike(...)` / `Settings.embed_model = SimpleDirectoryReader(...)`
   - Correct: `0 — Settings.embed_model = HuggingFaceEmbedding(...)`
   - Explanation: 只有该选项与原始代码一致。
   - Source `3abeb67e-0423-4883-b4d5-21a327fa2a75`: `Settings.embed_model = HuggingFaceEmbedding("BAAI/bge-small-zh-v1.5")`。

### 文本分块

9. **使用 RecursiveCharacterTextSplitter 时，默认情况下分隔符是否会被保留在分割后的文本块中？**
   - Options: `不会，默认删除` / `会，默认保留` / `仅换行符保留` / `仅空格保留`
   - Correct: `1 — 会，默认保留`
   - Explanation: 原文给出默认值 `keep_separator=True`。
   - Source `f4043793-9ffc-4319-a5c4-51064e7851fb`: “默认情况下 (`keep_separator=True`)，分隔符本身会被保留。”

10. **RecursiveCharacterTextSplitter 默认的分隔符尝试顺序是什么？**
    - Options: `字符、空格、行、段落` / `段落、行、空格、字符` / `只用段落` / `只用空格`
    - Correct: `1 — 段落、行、空格、字符`
    - Explanation: 原文直接列出 `\n\n`、`\n`、空格、空字符串的顺序。
    - Source `f4043793-9ffc-4319-a5c4-51064e7851fb`: “按顺序尝试……段落、行、空格、字符。”

11. **不指定参数初始化 RecursiveCharacterTextSplitter 时，默认的 chunk_size 和 chunk_overlap 分别是多少？**
    - Options: `1000/100` / `2000/200` / `4000/200` / `4000/400`
    - Correct: `2 — 4000/200`
    - Explanation: 基类默认参数在原文中明确给出。
    - Sources: `a2440106-068a-462d-91b5-35dece88fbfe` 与 `f4043793-9ffc-4319-a5c4-51064e7851fb`: “`chunk_size=4000` 和 `chunk_overlap=200`。”

12. **递归字符分割策略依次尝试段落、行、空格、字符分隔符的目的是什么？**
    - Options: `尽可能减小块大小` / `尽可能保持段落、句子和单词完整以保留语义结构` / `删除标点` / `转换为单字符`
    - Correct: `1 — 尽可能保持语义结构`
    - Explanation: 原文说明这些是语义上最相关的文本单元。
    - Source `f4043793-9ffc-4319-a5c4-51064e7851fb`: “尽可能保持段落、句子和单词的完整性……直到文本块达到目标大小。”

## Failure and retry statistics

Live run:

| Concept | Prompt chunks | Prompt tokens | Completion tokens | Attempts | Saved |
|---|---:|---:|---:|---:|---:|
| RAG | 3 | 681 | 720 | 1 | 4 |
| 嵌入模型 | 3 | 887 | 798 | 1 | 4 |
| 文本分块 | 3 | 814 | 777 | 1 | 4 |
| **Total** | **9 prompt chunk selections** | **2,382** | **2,295** | **3** | **12** |

- Live validation failures: 0
- Live retries: 0
- Dirty rows after failed generation: 0 (automated malformed-output test)
- Repeating the RAG generation endpoint returned the existing four items with
  `generated=0`, `attempt_count=0`; no second model call was needed.

Automated retry coverage proves malformed JSON is retried three times and
writes zero rows, while malformed-then-correct output succeeds on attempt two.

## Human quality conclusion

- Grounded in supplied source: **12/12**
- One unambiguous correct option: **12/12**
- Material outside the supplied chunks needed to answer: **0/12**
- Explanation traceable to cited chunk: **12/12**
- Distinct normalized question hashes: **12/12**

The bank is acceptable for the Phase 3 MVP. One RAG question asks for the
literal virtual-environment name `all-in-rag`; it is grounded and unambiguous
but has lower pedagogical value than the conceptual questions. A later item
ranking/curation phase should down-rank configuration trivia, but no difficulty
engine or quality-ranking feature belongs in this phase.

## Verification

Automated tests cover:

- valid model JSON and four persisted MCQs;
- malformed JSON with bounded retries and zero dirty writes;
- malformed then corrected output;
- three options, duplicate options and invalid correct index;
- source ID outside prompt input;
- duplicate normalized question;
- missing ChunkRefs;
- concept from another KB;
- non-Web principals;
- source snapshot change and stale-bank replacement; and
- pre-answer DTO omission of answers/explanations.

Live checks additionally covered a permitted owner request, lazy reuse, and an
actual unauthorized second user receiving 403 from the RBAC guard.

Final verification on the complete worktree:

- `go test ./... -count=1` — PASS
- `go test -race ./internal/application/service/learning ./internal/application/repository ./internal/handler -count=1` — PASS
- targeted `go vet` — PASS
- `git diff --check` — PASS

## Scope stop

Phase 3 intentionally stops here. It does not implement quiz submission,
attempt records, mastery updates, MRI scan orchestration or frontend changes.
