# WPI Editor Keyboard Delete Design

## Goal

Allow keyboard deletion of one or more selected WPI Editor canvas nodes and edges with
either Backspace or Delete.

## Design

Configure Svelte Flow's existing `deleteKey` option with both `Backspace` and `Delete`.
Svelte Flow already deletes every selected node and selected edge, ignores key presses
from editable inputs, and calls the editor's `onbeforedelete` hook. That hook records the
existing undo snapshot before removal, so the editor does not need a second deletion
handler or new selection state. Keep the existing empty-selection guard in loaded-page
mode unchanged. When one node is selected, the guard retains its tracked ID after
deletion, allowing undo to restore the Properties panel selection. Multi-selection has
no single tracked node ID, so undo restores the selected nodes without choosing one for
the Properties panel.

## Documentation

Update the WPI Editor devdoc's keyboard shortcut and undo-history descriptions to name
both keys and state that keyboard deletion removes selected nodes and edges. The
implementation adds no visible interface text, so Paraglide messages do not change.

## Scope

Only the WPI Editor shortcut configuration and its existing devdoc shortcut and
undo-history descriptions change. No tests are added or run for this request.
