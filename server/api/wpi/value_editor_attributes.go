package wpi

import "slices"

// Checks of the value-editor element code module (KnowledgeStore
// doc-repo/code_modules/interpreter-value-editor.md). One module draws every
// value editor of devdoc 2026102701 §1.3.9: each class fixes the one it draws in
// editor, from the IDs the class editor offers (classValueEditorIDs).

var (
	valueEditorAttributes = []string{"data_binding", "editor_options"}
	// pendingValueEditors have no code yet, so only a draft class may name one.
	pendingValueEditors = []string{"date-picker", "time-picker", "button-type-picker", "database-picker", "schema-picker"}
	drawnValueEditors   = slices.DeleteFunc(slices.Clone(classValueEditorIDs), func(id string) bool {
		return slices.Contains(pendingValueEditors, id)
	})
)

// valueEditorAttributes checks the values of a value-editor instance or active
// class row; the label and binding are checked like every element's.
func (v *validator) valueEditorAttributes(e map[string]any, path string) {
	v.text(e, "label", path)
	v.binding(e, path)
	v.oneOf(e, "binding_mode", path, bindingModes)
	v.oneOf(e, "editor", path, drawnValueEditors)
	if val, ok := e["editor_options"]; ok {
		v.object(val, path+"/editor_options")
	}
}

// valueEditorClassDefaults checks the defaults of a value-editor class row in
// the authoring format: a draft may name an editor that is not built yet.
func (v *validator) valueEditorClassDefaults(defaults map[string]any, draft bool) {
	allowed := drawnValueEditors
	if draft {
		allowed = classValueEditorIDs
	}
	v.oneOf(defaults, "editor", "", allowed)
	if val, ok := defaults["editor_options"]; ok {
		v.object(val, "/editor_options")
	}
}
