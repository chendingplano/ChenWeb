# DOCMAP

版本：1.0  
用途：供 AI 智能体使用的机器可读文档地图。

---

# 1. 元数据
元数据包括以下数据项：
- doc-name: <文档名称>
- doc-no: <文档编号>
- publish date: <发布日期>
- release date: <实施日期>
- abolish date: <废止日期>
- current state: <状态>
- replace: [doc-no, ...]
- created: <YYYY-MM-DD>
- last_updated: <YYYY-MM-DD>
- parser: <解析器版本>

# 2. 贡献者
贡献者包括组织和个人。
  - main organizations: [<组织名称>, <组织名称>, ...]
  - participating organizations: [<组织名称>, <组织名称>, ...]
  - main authors: [<作者姓名>, <作者姓名>, ...]
  - participating authors: [<作者姓名>, ...]

---

# 3. 类别与分类
类别：
    - 类别 I
    - 类别 II
    - 类别 III
    - 类别 IV
    - 类别 V

CCS: <ccs-code>

# 4. 智能体信息
本节包含面向智能体的特定信息：
    - chunking strategy
    - chunk size
    - human reviewed: {yes|no}
    - accuracy level: [0, 1]

# 5. 文档范围

included_topics:
  - 文档结构
  - API 参考
  - 主要数据参考
  - 机器可读性参考
  - 机器可执行参考
  - 开发者指南
  - 运维
  - 指标索引
  - 合规规则索引
  - 相关标准参考

excluded_topics:
  - 实验性说明
  - 内部草稿

---

# 6. 执行摘要

<用 3–6 句话描述系统和文档>

示例：

本文件描述了一个用于根据行业标准校验技术文档的平台。
该系统能够提取标准、识别要求，并验证技术文档中的合规性。
该仓库包含架构说明、API 参考、
开发者指南以及运维流程。

---

# 7. 关键词

keywords:
  - AI
  - RAG
  - 文档分析
  - 合规
  - 标准校验

---

# 8. 文档结构

root: docs/

tree:

  overview:
    description: 系统的高层介绍
    documents:
      - docs/overview/system-overview.md

  architecture:
    description: 系统设计与组件
    documents:
      - docs/architecture/architecture.md
      - docs/architecture/components.md

  guides:
    description: 用户与开发者指南
    documents:
      - docs/guides/installation.md
      - docs/guides/usage.md

  api:
    description: API 文档
    documents:
      - docs/api/api-reference.md

  operations:
    description: 部署与运维流程
    documents:
      - docs/operations/deployment.md
      - docs/operations/monitoring.md

---

# 9. 文档索引

documents:

  - path: docs/overview/system-overview.md
    summary: 系统及其用途概览
    keywords: [概览, 介绍]

  - path: docs/architecture/architecture.md
    summary: 详细架构说明
    keywords: [架构, 系统设计]

  - path: docs/guides/installation.md
    summary: 安装说明
    keywords: [安装, 设置]

  - path: docs/api/api-reference.md
    summary: API 端点参考
    keywords: [api, 参考]

---

# 10. 关键概念

concepts:

  - name: Standard
    description: 用于合规校验的行业规范
    related_docs:
      - docs/standards/standards-list.md

  - name: Requirement
    description: 标准定义的具体规则
    related_docs:
      - docs/standards/standards-list.md

  - name: Validation
    description: 根据要求检查文档的过程
    related_docs:
      - docs/architecture/architecture.md

---

# 11. 主题
概念是基础性的思想或实体。主题是围绕这些思想、概念、实体等展开讨论的领域。
    - 主题参考
    - 主题与内容关系

# 12. 逻辑关系
逻辑关系主要用于帮助智能体进行推理。

# 11. 概念关系

relationships:

  - Standard -> contains -> Requirement
  - Document -> references -> Standard
  - Validator -> checks -> Requirement

---

# 12. 面向任务的导航

tasks:

  understand_system:
    steps:
      - docs/overview/system-overview.md
      - docs/architecture/architecture.md

  install_system:
    steps:
      - docs/guides/installation.md

  use_api:
    steps:
      - docs/api/api-reference.md

  deploy_system:
    steps:
      - docs/operations/deployment.md

---

# 13. 外部参考

external_standards:

  - name: ISO 9001
    description: 质量管理标准

  - name: IEEE 829
    description: 软件测试文档标准

---

# 14. 检索提示（用于 RAG 系统）

query_examples:

  - 系统架构
  - API 使用
  - 安装步骤
  - 部署配置
  - 标准校验

recommended_docs:

  architecture:
    - docs/architecture/architecture.md

  installation:
    - docs/guides/installation.md

  api:
    - docs/api/api-reference.md

---

# 15. 更新策略

在以下情况发生时，必须更新 DOCMAP.md：

- 新增文档目录
- 文档被重命名或删除
- 架构或主要系统组件发生变化

---

# 16. 简短总结

DOCMAP 提供了一个机器可读的文档地图，
其中包含结构、文档索引、关键概念以及基于任务的
