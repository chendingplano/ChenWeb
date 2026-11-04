package wpi

import (
	"golang.org/x/text/language"
	"maps"
	"slices"
)

// Checks of the value-editor element code module (KnowledgeStore
// doc-repo/code_modules/interpreter-value-editor.md). One module draws every
// value editor of devdoc 2026102701 §1.3.9: each class fixes the one it draws in
// editor, from the IDs the class editor offers (classValueEditorIDs).

var (
	valueEditorAttributes = []string{"data_binding", "editor_options", "label_options"}
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
		if opts, valid := v.object(val, path+"/editor_options"); valid && str(e["editor"]) == "string-editor" {
			v.stringEditorOptions(opts, path+"/editor_options")
		}
	}
	if val, ok := e["label_options"]; ok {
		if opts, valid := v.object(val, path+"/label_options"); valid {
			v.labelOptions(opts, path+"/label_options")
		}
	}
}

// labelOptions checks the List choices of the label, the only key label_options
// holds. The label is a string, so the choices are checked as a string editor's.
func (v *validator) labelOptions(opts map[string]any, path string) {
	choices := map[string]any{}
	for _, key := range slices.Sorted(maps.Keys(opts)) {
		if key != "choices" {
			v.errf(path+"/"+key, "unknown attribute %q", key)
		} else {
			choices[key] = opts[key]
		}
	}
	v.stringEditorOptions(choices, path)
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
		if opts, valid := v.object(val, "/editor_options"); valid && str(defaults["editor"]) == "string-editor" {
			v.stringEditorOptions(opts, "/editor_options")
		}
	}
}

func (v *validator) stringEditorOptions(opts map[string]any, path string) {
	v.oneOf(opts, "mode", path, []string{"single", "multilingual", "list"})
	v.boolean(opts, "emptyAsNull", path)
	v.boolean(opts, "commitOnBlur", path)
	for _, key := range []string{"languages", "choices"} {
		val, present := opts[key]
		if !present {
			continue
		}
		items, ok := v.array(val, path+"/"+key, key == "languages")
		if !ok {
			continue
		}
		seen := map[string]bool{}
		for _, item := range items {
			s, ok := item.(string)
			if !ok || (key == "languages" && s == "") || seen[s] {
				v.errf(path+"/"+key, "must contain distinct strings")
				break
			}
			if key == "languages" {
				if _, err := language.Parse(s); err != nil {
					v.errf(path+"/"+key, "must contain valid language tags")
					break
				}
			}
			seen[s] = true
		}
	}
}
