## ADDED Requirements

### Requirement: Category tree storage
The system SHALL store category trees in `kb.category_trees`, their current nodes in
`kb.category_nodes`, the change log in `kb.category_tree_changes`, named revisions in
`kb.category_tree_revisions`, and the artifact-type binding in `kb.category_tree_types`.
Each node SHALL have an opaque UUID, a canonical name unique among the tree's non-retired
nodes, language-keyed preferred labels, alternative labels, hidden labels, definitions and
scope notes, a source, a list of external matches, and exactly one parent except the root.
A deleted node SHALL be kept with `retired_at` set and its ID never reused.

#### Scenario: Create a tree
- **WHEN** a user posts a tree with a canonical name, namespace, labels, definition and a nested list of initial nodes
- **THEN** the tree is stored, every initial node and every generated "others" leaf is recorded as the tree's first change set, and the tree's current sequence number equals the number of log entries

#### Scenario: Duplicate tree name
- **WHEN** a user creates a tree whose canonical name already exists
- **THEN** the request is rejected and nothing is stored

### Requirement: Tree validation
After every change set the tree SHALL pass these checks, or the change set SHALL be rejected
with all its effects rolled back: exactly one root, which is an inner node; one preferred label
in each of `en` and `zh` per node; definitions present in `en` and `zh`; canonical names unique;
preferred labels unique per language among ordinary (non-"others") nodes; no label both a
preferred and an alternative label of the same node; no alternative label shared by two nodes or
equal to another node's preferred label; every non-root node has one existing, non-retired
parent and there are no cycles; every inner node has exactly one "others" child and leaves have
none; "others" nodes have no children; no `exact` match between two nodes of the same tree.

#### Scenario: Missing Chinese label
- **WHEN** a change set adds a node without a `zh` preferred label
- **THEN** the change set is rejected with an error naming the node and the missing language, and the log and nodes are unchanged

#### Scenario: Duplicate canonical name
- **WHEN** a change set renames a node to another node's canonical name
- **THEN** the change set is rejected

### Requirement: Change sets and the change log
The system SHALL accept change sets of `add`, `delete`, `move` and `update` operations, apply
them atomically, and append one log entry per affected node with the change ID, change-set ID,
a sequence number increasing within the tree, timestamp, author or job, operation, node ID,
before and after node values, and reason. Log entries SHALL never be edited or removed.
Deleting a node SHALL retire its whole subtree. The root SHALL NOT be moved or deleted, and a
node SHALL NOT be moved under itself, a descendant, or an "others" leaf.

#### Scenario: Atomic change set
- **WHEN** a change set contains a valid add followed by an invalid move
- **THEN** neither operation takes effect and no log entry is written

#### Scenario: Delete an inner node
- **WHEN** a change set deletes an inner node with two leaf children
- **THEN** the node, its children and its "others" leaf are retired and four delete entries are logged

### Requirement: Reserved "others" leaves
The system SHALL maintain one reserved "others" leaf under every inner node, inside the same
change set as the change that requires it, with canonical name `<parent>-others`, labels
"Others" / "其他", and a definition generated from the parent in `en` and `zh`. The "others"
leaf SHALL follow its parent's renames, and SHALL be retired when its parent has no ordinary
children left. Callers SHALL NOT add, change, move or delete "others" leaves directly.

#### Scenario: Leaf gets its first child
- **WHEN** a change set adds a child to a leaf L
- **THEN** the same change set also adds `L-others` under L

#### Scenario: Parent renamed
- **WHEN** a change set renames node P to Q
- **THEN** the same change set renames `P-others` to `Q-others`

### Requirement: Revisions and replay
Users SHALL be able to create a named revision pointing at the tree's current sequence number;
revision names SHALL be unique within a tree and never moved. Users SHALL be able to view the
tree at any revision or sequence number, computed by replaying the log from an empty tree; the
replayed tree SHALL pass validation.

#### Scenario: View an earlier revision
- **WHEN** revision `v1.0.1` is created, a node is then renamed, and the user views `v1.0.1`
- **THEN** the returned tree shows the node's old canonical name, while the current tree shows the new one

#### Scenario: Duplicate revision name
- **WHEN** a user creates revision `v1.0.1` a second time
- **THEN** the request is rejected

### Requirement: Artifact type binding
Each artifact type SHALL be bound to at most one tree in `kb.category_tree_types`, set by a user.

#### Scenario: Bind metrics to a tree
- **WHEN** a user binds `metrics` to tree `metric-categories`
- **THEN** later classification of metrics uses that tree

### Requirement: Stale marking on tree changes
Within the change-set transaction the system SHALL mark existing bindings stale, recording the
log entry that caused it: bindings on any node in the subtree of a node whose definitions or
scope notes change; bindings on a leaf that gains its first ordinary child; bindings on an
inner node's "others" leaf when that node gains an ordinary child; and bindings on deleted
nodes. Label, source, canonical-name, match-only updates and moves SHALL NOT mark bindings stale.

#### Scenario: Definition change
- **WHEN** the definition of inner node P changes and artifact A is bound to a leaf under P
- **THEN** A's binding is marked stale with the change's ID and stays published

#### Scenario: Label change
- **WHEN** only the English preferred label of a bound leaf changes
- **THEN** no binding is marked stale
