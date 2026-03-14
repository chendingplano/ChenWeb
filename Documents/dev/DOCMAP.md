# DOCMAP

Version: 1.0
Purpose: Machine-readable map of documentation for AI agents.

---

# 1. Metadata
Metadata includes the following data items:
- doc-name: <doc name>
- doc-no: <doc no>
- publish date: <publish date>
- release date: <release date>
- abolish date: <abolish date>
- current state: <state>
- replace: [doc-no, ...]
- created: <YYYY-MM-DD>
- last_updated: <YYYY-MM-DD>
- parser: <parser-version>

# 2. Contributors
Contributors include organizations and individuals.
  - main organizations: [<org-name>, <org-name>,...]
  - participating organizations: [<org-name>, <org-name>,...]
  - main authors: [<person-name>, <person-name>,...]
  - participating authors: [<person-name>, ...]

---

# 3. Catagory and Classification
Catagories:
    - Catagory I
    - Catagory II
    - Catagory III
    - Catagory IV
    - Catagory V

CCS: <ccs-code>

# 4. Agent Info
This section contains information specific for agents:
    - chunking strategy
    - chunk size
    - human reviewed: {yes|no}
    - accuracy level: [0, 1]

# 5. Documentation Scope

included_topics:
  - Doc Structure
  - API Reference
  - Main Data Reference
  - Machine Readability Reference
  - Machine Executable Reference
  - Developer guides
  - Operations
  - Metric Index
  - Compliance Rule Index
  - Relevant Standard Reference

excluded_topics:
  - experimental notes
  - internal drafts

---

# 6. Executive Summary

<3–6 sentences describing the system and documentation>

Example:

This documentation describes a platform that validates technical documents
against industry standards. The system extracts standards, identifies
requirements, and verifies compliance within technical documentation.
The repository contains architecture descriptions, API references,
developer guides, and operational procedures.

---

# 7. Keywords

keywords:
  - AI
  - RAG
  - document analysis
  - compliance
  - standards validation

---

# 8. Documentation Structure

root: docs/

tree:

  overview:
    description: High-level introduction to the system
    documents:
      - docs/overview/system-overview.md

  architecture:
    description: System design and components
    documents:
      - docs/architecture/architecture.md
      - docs/architecture/components.md

  guides:
    description: User and developer guides
    documents:
      - docs/guides/installation.md
      - docs/guides/usage.md

  api:
    description: API documentation
    documents:
      - docs/api/api-reference.md

  operations:
    description: Deployment and operational procedures
    documents:
      - docs/operations/deployment.md
      - docs/operations/monitoring.md

---

# 9. Document Index

documents:

  - path: docs/overview/system-overview.md
    summary: Overview of the system and its purpose
    keywords: [overview, introduction]

  - path: docs/architecture/architecture.md
    summary: Detailed architecture description
    keywords: [architecture, system design]

  - path: docs/guides/installation.md
    summary: Installation instructions
    keywords: [installation, setup]

  - path: docs/api/api-reference.md
    summary: API endpoint reference
    keywords: [api, reference]

---

# 10. Key Concepts

concepts:

  - name: Standard
    description: Industry specification used for compliance
    related_docs:
      - docs/standards/standards-list.md

  - name: Requirement
    description: Specific rule defined by a standard
    related_docs:
      - docs/standards/standards-list.md

  - name: Validation
    description: Process of checking a document against requirements
    related_docs:
      - docs/architecture/architecture.md

---

# 11. Topics
Concepts are fundamental ideas or entities. Topics are the areas of discussion about ideas, concepts, entities., etc.
    - Topic Reference
    - Topic and Content Relation

# 12. Logical Relationships
Logical relationships are mainly used for agent to reason.

# 11. Concept Relationships

relationships:

  - Standard -> contains -> Requirement
  - Document -> references -> Standard
  - Validator -> checks -> Requirement

---

# 12. Task-Oriented Navigation

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

# 13. External References

external_standards:

  - name: ISO 9001
    description: Quality management standard

  - name: IEEE 829
    description: Software test documentation standard

---

# 14. Retrieval Hints (For RAG Systems)

query_examples:

  - system architecture
  - API usage
  - installation steps
  - deployment configuration
  - standards validation

recommended_docs:

  architecture:
    - docs/architecture/architecture.md

  installation:
    - docs/guides/installation.md

  api:
    - docs/api/api-reference.md

---

# 15. Update Policy

DOCMAP.md must be updated when:

- new documentation directories are added
- documents are renamed or removed
- architecture or major system components change

---

# 16. Short Summary

DOCMAP provides a machine-readable map of the documentation,
including structure, document index, key concepts, and task-based
navigation to help AI agents efficiently locate relevant information.

## Logical Relationship and Relation
The terms **“relations”** and **“logical relationships”** overlap but are used with **different emphasis and abstraction levels**, especially in **knowledge graphs, ontologies, and reasoning systems for agents**.

**Relations**

> explicit factual connections between entities.

**Logical relationships**

> reasoning-based connections derived from rules, constraints, or interpretations.

For AI agents, the distinction between relations and logical relationship is important.

Agents operate in two phases:

### Knowledge retrieval

Uses **relations**.

Example:

```
Find all Requirements contained in ISO9001
```

---

### 1. Relations (Graph / Knowledge Representation)

A **relation** is a **formal connection between entities (nodes)**.

Typical form:

```
(subject) --relation--> (object)
```

Example:

```
Standard --contains--> Requirement
Document --references--> Standard
Validator --checks--> Requirement
```

Characteristics:

* **objective**
* **structural**
* **machine-readable**
* **explicitly defined**
* used in **graphs, databases, and ontologies**

Relations are usually **facts or assertions** about the world.

Example knowledge graph triple:

```
(Document123, references, ISO9001)
```

This is **data**, not reasoning.

---

### 2. Logical Relationships

A **logical relationship** is a **relationship defined by reasoning or logic**, rather than just a stored edge.

Logical relationships often represent:

* **implication**
* **dependency**
* **causality**
* **constraints**
* **derivations**

Examples:

```
If Document references Standard
AND Standard contains Requirement
THEN Document must satisfy Requirement
```

This relationship is **not explicitly stored** but **derived logically**.

Logical relationships are often expressed as:

* rules
* implications
* constraints

Example rule:

```
references(Document, Standard)
contains(Standard, Requirement)
→ must_satisfy(Document, Requirement)
```

This is **reasoning**, not just data.

---

### 3. Key Difference

|                | Relations          | Logical Relationships              |
| -------------- | ------------------ | ---------------------------------- |
| Nature         | Structural link    | Reasoning link                     |
| Role           | Data               | Logic                              |
| Representation | Graph edge         | Rule / inference                   |
| Objectivity    | Mostly objective   | Can include interpretation         |
| Use            | Knowledge graphs   | Reasoning engines                  |
| Example        | A → references → B | If A references B → A depends on B |

---

### 4. Another Way to View It

### Relations = **What is true**

Example:

```
Standard --contains--> Requirement
```

This is a **fact**.

---

### Logical relationships = **What follows logically**

Example:

```
If Standard contains Requirement
AND Document claims compliance with Standard
THEN Document must satisfy Requirement
```

This is **reasoning**.

---

### 5. In Graph Systems

In graph databases (Neo4j, RDF, etc.), **relations are edges**.

Example:

```
(Document) --references--> (Standard)
```

But logical relationships may be:

* inferred edges
* rule-based relationships
* temporary reasoning outputs

Example inferred relation:

```
(Document) --must_satisfy--> (Requirement)
```

derived from:

```
references + contains
```

---

### 6. Subjectivity Aspect

Your observation about **subjectivity** is insightful.

### Relations

Usually objective:

```
Document --references--> ISO9001
```

This is a factual statement.

---

### Logical relationships

Can include interpretation.

Example:

```
Document --implies--> compliance
```

or

```
Section A --contradicts--> Section B
```

These may involve **interpretation**.

Especially when generated by:

* humans
* LLM reasoning
* rule systems

---

### 7. Another Example

Consider this statement:

```
Requirement R1 is stricter than Requirement R2
```

This is **not always a pure fact**.

It may depend on interpretation.

So it is better classified as a **logical relationship** rather than a simple relation.

---

### 8. Layered View (Very Useful)

You can think of three layers.

#### Layer 1 — Entities

```
Standard
Requirement
Document
Section
```

---

#### Layer 2 — Relations (Facts)

```
Standard --contains--> Requirement
Document --references--> Standard
Requirement --located_in--> Section
```

---

#### Layer 3 — Logical Relationships (Reasoning)

```
references + contains → must_satisfy

must_satisfy + violation → non_compliant
```

These are **rules**.

---

### 9. In Agent Systems

---

### Reasoning

Uses **logical relationships**.

Example:

```
If requirement is violated → compliance failure
```

---

### 10. Practical Example

In the **document compliance checker**:

#### Relations

```
Document --references--> Standard
Standard --contains--> Requirement
Requirement --located_in--> Section
```

---

#### Logical relationships

```
references + contains → must_satisfy

must_satisfy + missing_requirement → violation

violation → non_compliance
```

---

### 11. Why This Distinction Matters

Especially for **LLM-based reasoning systems**.

Relations help with:

* retrieval
* indexing
* graph navigation

Logical relationships help with:

* inference
* validation
* explanation

---

### 12. Simple Mental Model

```
Relations = edges
Logical relationships = rules
```

or

```
Relations = data
Logical relationships = logic
```
