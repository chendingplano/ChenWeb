面向智能体的 **DocGraph**：一种面向**LLM 文档分析**的图原生架构，结合了：

* 用于机器可读文档导航的 **DOCMAP.md** 层，
* 用于多步证据链的**推理图**，
* 以及覆盖分块、条款、实体、引文和文档结构的**混合检索**。

基于图的检索尤其适合**多跳问题**、长文档语料，以及像标准、规范和法规这类具有显式交叉引用的领域。[arXiv][1]

## DocGraph

```text
                                  ┌───────────────────────────┐
                                  │        USER QUERY         │
                                  │ “如果系统 Y 使用 Z，哪些     │
                                  │  条款要求满足 X？”          │
                                  └─────────────┬─────────────┘
                                                │
                                                ▼
                     ┌──────────────────────────────────────────────┐
                     │         QUERY PLANNER / ROUTER               │
                     │ intent, scope, entities, doc families, hops  │
                     └───────┬───────────────────────┬──────────────┘
                             │                       │
                             │                       │
                             ▼                       ▼
              ┌────────────────────────┐   ┌───────────────────────────┐
              │      DOCMAP.md         │   │     VECTOR / BM25 INDEX   │
              │ machine-readable map   │   │ chunks, headings, tables  │
              │ of corpus structure    │   │ semantic + lexical search │
              └──────────┬─────────────┘   └─────────────┬─────────────┘
                         │                               │
                         ▼                               ▼
      ┌────────────────────────────────────────────────────────────────────┐
      │                            DOCGRAPH                                │
      │                                                                    │
      │  Document nodes      Section nodes      Clause nodes               │
      │  Standard nodes      Table/Figure nodes Term/Definition nodes      │
      │  Entity nodes        Citation nodes     Requirement nodes          │
      │                                                                    │
      │  Edge types:                                                       │
      │  CONTAINS • DEFINES • CITES • AMENDS • REFERENCES • OVERRIDES      │
      │  APPLIES_TO • EXCEPTION_TO • SATISFIES • CONFLICTS_WITH            │
      │  DERIVED_FROM • VERSION_OF • NEARBY • SAME_TOPIC                   │
      └──────────┬───────────────────────────────────────────────┬─────────┘
                 │                                               │
                 ▼                                               ▼
   ┌──────────────────────────────┐                 ┌───────────────────────────┐
   │     REASONING GRAPH          │                 │    EVIDENCE ASSEMBLER     │
   │ query-specific working graph │                 │ ranks passages + paths +  │
   │ hypotheses, hops, subclaims  │                 │ citations + provenance    │
   └──────────────┬───────────────┘                 └─────────────┬─────────────┘
                  │                                               │
                  └──────────────────────┬────────────────────────┘
                                         ▼
                          ┌────────────────────────────────┐
                          │   LLM ANSWER + CITED TRACE     │
                          │ answer, uncertainty, evidence  │
                          │ path, clause-level grounding   │
                          └────────────────────────────────┘
```

## 各层分别做什么

### 1) DOCMAP.md

这是语料库面向智能体的**控制平面**。

**DOCMAP.md 之于文档，就像 SKILL.md 之于技能。** 它会告诉模型：

* 存在哪些文档族，
* 权威版本位于哪里，
* 整个语料库是如何组织的，
* 哪些文件是规范性的，哪些是解释性的，
* 哪些章节属于定义、要求、例外、附录、变更日志和对照关系。

最近面向智能体的文档生态已经开始收敛到这种模式：在根目录放置一个映射文件，引导智能体进入正确的文档子集，而不是让它盲目扫描全部内容。[Simon Willison’s Weblog][2]

关于文档映射，请参考 `DOCMAP.md`。

### 2) DocGraph

这是**持久化的语料图**。

它同时存储：

* **结构关系**：文档 → 章节 → 条款 → 表格，
* **语义关系**：术语 A 定义 B，条款 C 引用 D，章节 E 覆盖 F。

对于标准、规范和法规，最重要的节点类型通常是：

* **Document（文档）**
* **Section / subsection / clause（章节 / 小节 / 条款）**
* **Definition（定义）**
* **Requirement（要求）**
* **Exception（例外）**
* **Citation / cross-reference（引文 / 交叉引用）**
* **Term / controlled vocabulary（术语 / 受控词汇）**
* **Version / amendment（版本 / 修订）**
* **Entity / system / component / actor（实体 / 系统 / 组件 / 参与方）**

这类大型技术资料库最能从这里受益，因为这些语料并不只是文本集合；它们本质上是**义务、定义、范围限制和引用关系构成的网络**。关于图增强 RAG 的论文一再指出，这种结构能够提升复杂问题和多跳问题的检索效果。[arXiv][3]

### 3) 推理图

这是**查询时临时构建的图**。

与持久化的 DocGraph 不同，推理图是按问题逐次构建的。它会跟踪：

* 子问题，
* 检索到的证据，
* 假设，
* 尚未解决的歧义，
* 支持 / 反驳关系，
* 最终证据路径。

监管类问题示例：

```text
问题：“对于海上系统中的远程停机，条款 8 是否要求冗余？”

推理图：
  H1: “远程停机”在规范 A 中有定义
  H2: 海上系统属于范围 B
  H3: 条款 8 施加了 SHALL 要求
  H4: 附录 C 提供了一个例外
  H5: 较新的修订修改了附录 C

证据路径：
  DOCMAP 路由
    -> 规范 A
    -> 范围 B
    -> 条款 8
    -> 附录 C
    -> 修订 2025-2
```

这种分离很有用：

* **DocGraph** = 持久的语料记忆
* **推理图** = 面向单次查询的工作记忆

### 4) 检索

一个强健的 DocGraph 系统应当使用**混合检索**，而不是纯图检索。

最佳模式：

* **DOCMAP 路由** 用来缩小搜索空间
* **vector/BM25 检索** 用来找候选分块
* **图遍历** 沿着引用、定义、例外和版本链接继续扩展
* **重排序** 将段落分数与图路径分数一起综合评估

这与当前 GraphRAG 的实践一致：混合方法通常优于纯向量检索或纯图遍历，尤其是在问题需要跨多个条款或多个文档拼接证据时更是如此。[ScienceDirect][4]

---

## 为什么它特别适合标准、规范和法规

这类资料库通常都具备让“扁平分块”表现不佳的特征：

* 层级结构强，
* 交叉引用密集，
* 定义明确，
* 存在版本和修订，
* 有例外与豁免条款，
* 很多问题天然就是多跳问题。

DocGraph 有助于回答这样的问题：

* “哪些条款定义、约束或覆盖了这项要求？”
* “2023 版与当前版本之间发生了什么变化？”
* “哪些表格和附录适用于这个组件？”
* “这项要求是规范性的、说明性的，还是已经被替代？”
* “哪一条条款链支持这个结论？”

这正是图增强检索被报告为最有帮助的典型场景。[MDPI][5]

## 一个好的心智模型

可以把整个栈理解为：

```text
DOCMAP.md      = 如何在语料库中导航
DocGraph       = 语料库在结构与语义上“知道什么”
推理图         = 这个具体问题是如何被求解的
检索           = 证据如何进入系统
LLM            = 基于证据的答案如何被综合生成
```

## 推荐的节点与边模式

一个紧凑可实现的模式如下：

```text
Nodes:
  Document(id, title, type, version, authority)
  Section(id, heading, level)
  Clause(id, text, modality)          # SHALL / SHOULD / MAY / MUST NOT
  Definition(id, term, definition)
  Requirement(id, requirement_type)
  Exception(id)
  Citation(id, target_ref)
  Table(id)
  Figure(id)
  Entity(id, name, type)
  Topic(id, label)

Edges:
  CONTAINS
  NEXT
  PARENT_OF
  DEFINES
  REFERENCES
  CITES
  AMENDS
  SUPERSEDES
  APPLIES_TO
  EXCEPTION_TO
  SATISFIES
  CONFLICTS_WITH
  MENTIONS
  SAME_TOPIC
  DERIVED_FROM
```

## 如果你想要一个尽可能简单的流水线

```text
1. 将语料解析为文档 / 章节 / 条款
2. 基于语料元数据构建 DOCMAP.md
3. 在分块上建立向量索引
4. 基于结构 + 引用 + 定义 + 版本构建 DocGraph
5. 在查询时：
   a. 通过 DOCMAP 路由
   b. 检索 top chunks
   c. 通过图邻居扩展
   d. 构建推理图
   e. 生成带条款级引用的答案
```

## 结论

当你希望模型将文档资料库视为一个**可导航的证据系统**，而不只是一个分块集合时，**DocGraph** 最有价值。

用一句话概括：

> **DOCMAP.md 决定去哪里找，DocGraph 建模哪些内容彼此相连，推理图跟踪答案如何推导出来，而检索负责带入精确证据。**

如果你愿意，我还可以把它进一步整理成一个**正式规范**，包含：

* 一个 `DOCMAP.md` 模板，
* 一个用于 DocGraph 节点 / 边的 JSON Schema，
* 以及一个面向标准 / 法规语料的端到端查询流程。

[1]: https://arxiv.org/abs/2404.16130?utm_source=chatgpt.com "A Graph RAG Approach to Query-Focused Summarization"
[2]: https://simonwillison.net/2025/Oct/24/claude-code-docs-map/?utm_source=chatgpt.com "claude_code_docs_map.md"
[3]: https://arxiv.org/pdf/2501.13958?utm_source=chatgpt.com "A Survey of Graph Retrieval-Augmented Generation for ..."
[4]: https://www.sciencedirect.com/science/article/pii/S092658052600035X?utm_source=chatgpt.com "Bridging dual knowledge graphs for multi-hop question ..."
[5]: https://www.mdpi.com/2079-9292/14/11/2102?utm_source=chatgpt.com "Document GraphRAG: Knowledge Graph Enhanced ..."
