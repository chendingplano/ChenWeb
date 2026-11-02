package wpi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"

	"github.com/labstack/echo/v4"
)

// Class editing reads the stored definition, never an instance's frozen copy.
func editableClassKind(kind Kind) bool {
	return slices.Contains([]Kind{KindElement, KindAction, KindPageLayouter, KindElementLayouter}, kind)
}

func (s store) class(ctx context.Context, kind Kind, id string) (row ClassRow, found bool, err error) {
	if kind == KindElement {
		return s.elementClass(ctx, id)
	}
	var raw []byte
	err = s.db.QueryRowContext(ctx, `SELECT version, status, body FROM kb.wpi_descriptors
		WHERE kind = $1 AND descriptor_id = $2 AND status IN ('draft', 'active') ORDER BY version DESC LIMIT 1`, string(kind), id).
		Scan(&row.Version, &row.Status, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return ClassRow{}, false, nil
	}
	row.Class = raw
	return row, err == nil, err
}

func (s store) updateClassKind(ctx context.Context, kind Kind, id string, current ClassRow, raw []byte, actor string) (int, error) {
	if kind == KindElement {
		return s.updateClass(ctx, id, current, raw, actor)
	}
	var version int
	err := s.db.QueryRowContext(ctx, `UPDATE kb.wpi_descriptors
		SET schema_version = $3::text, body = $4::jsonb, updated_by = $5::text, updated_at = NOW()
		WHERE kind = $1::text AND descriptor_id = $2::text AND version = $6 AND status = $7::text
		RETURNING version`, string(kind), id, schemaVersionOf(kind), string(raw), actor, current.Version, current.Status).Scan(&version)
	return version, err
}

// GetClass serves GET /wpi/classes/:kind/:classId to authenticated authors.
func (h *Handler) GetClass(c echo.Context) error {
	user := h.User(c)
	if user == nil {
		return errorJSON(c, http.StatusUnauthorized, "Authentication required")
	}
	kind, id := Kind(c.Param("kind")), c.Param("classId")
	if !editableClassKind(kind) || !idPattern.MatchString(id) {
		return errorJSON(c, http.StatusNotFound, "Class not found")
	}
	row, found, err := (store{h.DB}).class(c.Request().Context(), kind, id)
	if err != nil {
		handlerLog.Error("WPI class read failed", "kind", kind, "id", id, "user", actorName(user), "error", err)
		return errorJSON(c, http.StatusInternalServerError, "Unable to read class")
	}
	if !found {
		return errorJSON(c, http.StatusNotFound, "Class not found")
	}
	handlerLog.Info("WPI class read", "kind", kind, "id", id, "version", row.Version, "status", row.Status, "user", actorName(user))
	return c.JSON(http.StatusOK, row)
}

// UpdateClass edits one existing class record in place, preserving its status.
func (h *Handler) UpdateClass(c echo.Context) error {
	user := h.User(c)
	if user == nil {
		return errorJSON(c, http.StatusUnauthorized, "Authentication required")
	}
	if !isAdmin(user) {
		return errorJSON(c, http.StatusForbidden, "Admin access required")
	}
	kind, id := Kind(c.Param("kind")), c.Param("classId")
	if !editableClassKind(kind) || !idPattern.MatchString(id) {
		return errorJSON(c, http.StatusNotFound, "Class not found")
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request().Body, maxBodyBytes+1))
	if err != nil || len(raw) > maxBodyBytes {
		return errorJSON(c, http.StatusBadRequest, "Invalid request body")
	}
	body, err := decodeBody(raw)
	if err != nil {
		return errorJSON(c, http.StatusBadRequest, "Body must be a JSON object")
	}
	if str(body["descriptor_id"]) != id {
		return errorJSON(c, http.StatusBadRequest, "descriptor_id does not match class ID")
	}
	st := store{h.DB}
	ctx := c.Request().Context()
	current, found, err := st.class(ctx, kind, id)
	if err != nil {
		handlerLog.Error("WPI class read failed", "kind", kind, "id", id, "user", actorName(user), "error", err)
		return errorJSON(c, http.StatusInternalServerError, "Unable to read class")
	}
	if !found {
		return errorJSON(c, http.StatusNotFound, "Class not found")
	}
	var issues []Issue
	if kind == KindElement {
		issues = validateElementClass(body, current.Status == "draft")
	} else {
		issues = ValidateDescriptor(kind, id, body)
	}
	if len(issues) > 0 {
		return c.JSON(http.StatusBadRequest, map[string]any{"errors": issues})
	}
	if kind != KindAction {
		if refused, err := checkModule(c, st, kind, id, body, user); refused {
			return err
		}
	}
	version, err := st.updateClassKind(ctx, kind, id, current, raw, actorName(user))
	if errors.Is(err, sql.ErrNoRows) {
		handlerLog.Warn("WPI class missing during update", "kind", kind, "id", id, "version", current.Version, "user", actorName(user))
		return errorJSON(c, http.StatusNotFound, "Class not found")
	}
	if err != nil {
		handlerLog.Error("WPI class update failed", "kind", kind, "id", id, "user", actorName(user), "error", err)
		return errorJSON(c, http.StatusInternalServerError, "Unable to store class")
	}
	var warnings []Issue
	if kind == KindElement {
		warnings = classWarnings(id, body)
	}
	handlerLog.Info("WPI class updated", "kind", kind, "id", id, "version", version, "status", current.Status, "user", actorName(user))
	return c.JSON(http.StatusOK, map[string]any{"descriptor_id": id, "version": version, "status": current.Status, "warnings": nonNil(warnings)})
}

var classValueEditorIDs = []string{
	"text", "multiline-text", "localized-text", "number", "switch", "value-picker", "ordered-multi-picker", "string-list",
	"date-picker", "time-picker", "color", "length", "font-picker", "alignment-picker", "icon-picker", "button-type-picker",
	"database-picker", "schema-picker", "table-picker", "field-picker", "data-source", "column-list", "sort-order-editor",
	"validation", "action-picker", "layouter-picker", "read-only", "json",
}

// attributeDefinitions checks authoring metadata independently of runtime defaults.
func (v *validator) attributeDefinitions(body map[string]any) {
	value, present := body["attribute_definitions"]
	if !present {
		return
	}
	definitions, ok := v.object(value, "/attribute_definitions")
	if !ok {
		return
	}
	for _, pointer := range slices.Sorted(maps.Keys(definitions)) {
		value := definitions[pointer]
		path := "/attribute_definitions/" + strings.ReplaceAll(strings.ReplaceAll(pointer, "~", "~0"), "/", "~1")
		if !validAttributePointer(pointer) {
			v.errf(path, "must name an attribute with a JSON pointer")
		}
		definition, ok := v.object(value, path)
		if !ok {
			continue
		}
		v.keys(definition, path, nil, "description", "scope", "value_editor")
		v.text(definition, "description", path)
		v.oneOf(definition, "scope", path, []string{"class", "instance"})
		v.elementDefinitionScope(body, pointer, definition, path)
		if value, present := definition["value_editor"]; present {
			editor, ok := v.object(value, path+"/value_editor")
			if !ok {
				continue
			}
			v.keys(editor, path+"/value_editor", []string{"id"}, "options")
			v.oneOf(editor, "id", path+"/value_editor", classValueEditorIDs)
			if value, present := editor["options"]; present {
				options, ok := v.object(value, path+"/value_editor/options")
				if ok {
					v.editorOptions(options, path+"/value_editor/options")
				}
			}
		}
	}
}

// Top-level element scope controls whether instances may supply their own value.
// Nested, action and layouter scopes remain authoring classifications.
func (v *validator) elementDefinitionScope(body map[string]any, pointer string, definition map[string]any, path string) {
	if v.kind != KindElement || !validAttributePointer(pointer) || strings.Contains(pointer[1:], "/") {
		return
	}
	scope, hasScope := definition["scope"].(string)
	if !hasScope || (scope != "class" && scope != "instance") {
		return
	}
	name := strings.ReplaceAll(strings.ReplaceAll(pointer[1:], "~1", "/"), "~0", "~")
	if slices.Contains(classAttrs, name) {
		if scope != "class" {
			v.errf(path+"/scope", "%q is a class attribute", name)
		}
		return
	}
	attribute, exists := body[name]
	if !exists {
		return // Keep metadata when an optional value or subtree has been removed.
	}
	attr, ok := attribute.(map[string]any)
	if !ok {
		return // The attribute's structural validation reports this.
	}
	fixed := attr["instance_modifiable"] == false
	if scope == "class" && (!fixed || attr["default_value"] == nil) {
		v.errf(path+"/scope", "a class attribute must be fixed with a non-null default_value")
	}
	if scope == "instance" && fixed {
		v.errf(path+"/scope", "a fixed attribute cannot have instance scope")
	}
}

func validAttributePointer(pointer string) bool {
	if !strings.HasPrefix(pointer, "/") || pointer == "/" {
		return false
	}
	for _, segment := range strings.Split(pointer[1:], "/") {
		for i := 0; i < len(segment); i++ {
			if segment[i] == '~' {
				if i+1 == len(segment) || (segment[i+1] != '0' && segment[i+1] != '1') {
					return false
				}
				i++
			}
		}
	}
	return true
}

func (v *validator) editorOptions(options map[string]any, path string) {
	for _, key := range slices.Sorted(maps.Keys(options)) {
		value := options[key]
		p := path + "/" + key
		switch key {
		case "integer", "theme", "numberRules", "commitOnBlur", "range":
			v.boolean(options, key, path)
		case "min", "max", "step", "minItems", "rows":
			switch value.(type) {
			case json.Number, float64:
			default:
				v.errf(p, "must be a number")
			}
		case "choices", "units", "keywords":
			values, ok := v.array(value, p, false)
			if ok {
				for _, item := range values {
					if _, ok := item.(string); !ok {
						v.errf(p, "must contain strings")
					}
				}
			}
		case "format", "database", "schema", "table":
			v.str(options, key, path)
		}
	}
}
