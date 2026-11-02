-- Store the Class editor's scope and value editor beside each element attribute.
-- Existing defaults, help tips, record versions/status and frozen page copies stay intact.
-- Metadata for built-in class fields and nested values remains in attribute_definitions.
-- +goose Up
-- +goose StatementBegin
DO $migration$
DECLARE
  editor_defaults jsonb := $defaults${"text":{"text":{"id":"localized-text","options":{"rows":2}},"data_binding":{"id":"text","options":{}},"label":{"id":"localized-text","options":{}},"help_tip":{"id":"localized-text","options":{"rows":2}},"label_placement":{"id":"value-picker","options":{"choices":["top","left","right","bottom"]}},"label_horizontal_gap":{"id":"length","options":{"units":["px"]}},"label_vertical_gap":{"id":"length","options":{"units":["px"]}},"label_horizontal_align":{"id":"value-picker","options":{"choices":["start","center","end"]}},"label_vertical_align":{"id":"value-picker","options":{"choices":["top","center","bottom"]}},"label_max_width":{"id":"length","options":{"units":["px"]}},"label_font_weight":{"id":"value-picker","options":{"choices":["inherit","normal","bold","100","200","300","400","500","600","700","800","900"]}},"icon":{"id":"value-picker","options":{"choices":["none","info","circle-help","circle-check","triangle-alert","file-text","user"]}},"icon_placement":{"id":"value-picker","options":{"choices":["start","end"]}},"icon_size":{"id":"length","options":{"units":["px"]}},"icon_gap":{"id":"length","options":{"units":["px"]}},"icon_color":{"id":"color","options":{}},"font_family":{"id":"text","options":{}},"font_size":{"id":"length","options":{"units":["px","rem","em"]}},"font_weight":{"id":"value-picker","options":{"choices":["inherit","normal","bold","100","200","300","400","500","600","700","800","900"]}},"font_style":{"id":"value-picker","options":{"choices":["normal","italic","oblique"]}},"line_height":{"id":"json","options":{}},"color":{"id":"color","options":{}},"background_color":{"id":"color","options":{}},"text_align":{"id":"value-picker","options":{"choices":["start","center","end","justify"]}},"white_space":{"id":"value-picker","options":{"choices":["normal","pre-wrap"]}},"padding":{"id":"text","options":{}},"width":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}},"height":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}},"min_width":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}},"max_width":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}}},"input":{"label":{"id":"localized-text","options":{}},"data_binding":{"id":"text","options":{}},"binding_mode":{"id":"value-picker","options":{"choices":["read-only","read-write"]}},"input_type":{"id":"value-picker","options":{"choices":["text","password","email","number","tel","url","search"]}},"label_placement":{"id":"value-picker","options":{"choices":["left","top","right","bottom"]}},"label_horizontal_gap":{"id":"length","options":{"units":["px"]}},"label_vertical_gap":{"id":"length","options":{"units":["px"]}},"label_horizontal_align":{"id":"value-picker","options":{"choices":["left","center","right"]}},"label_vertical_align":{"id":"value-picker","options":{"choices":["top","center","bottom"]}},"label_max_width":{"id":"length","options":{"units":["px"]}},"label_max_lines":{"id":"number","options":{"min":1,"integer":true}},"label_font_family":{"id":"text","options":{}},"label_font_size":{"id":"length","options":{"units":["px"]}},"label_color":{"id":"color","options":{}},"label_font_weight":{"id":"value-picker","options":{"choices":["normal","bold","100","200","300","400","500","600","700","800","900"]}},"label_font_style":{"id":"value-picker","options":{"choices":["normal","italic","oblique"]}},"placeholder":{"id":"localized-text","options":{}},"help_text":{"id":"localized-text","options":{}},"help_tip":{"id":"localized-text","options":{}},"autocomplete":{"id":"text","options":{}},"required":{"id":"switch","options":{}},"max_length":{"id":"number","options":{"min":0,"integer":true}},"validation":{"id":"validation","options":{}},"width":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}},"height":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}},"min_width":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}},"max_width":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}}},"select":{"label":{"id":"localized-text","options":{}},"data_binding":{"id":"text","options":{}},"binding_mode":{"id":"value-picker","options":{"choices":["read-only","read-write"]}},"options":{"id":"json","options":{"rows":3}},"width":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}},"height":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}},"min_width":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}},"max_width":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}}},"button":{"label":{"id":"localized-text","options":{}},"on_click":{"id":"value-picker","options":{}},"width":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}},"height":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}},"min_width":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}},"max_width":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}}},"list":{"source":{"id":"data-source","options":{}},"columns":{"id":"column-list","options":{}},"presentation":{"id":"value-picker","options":{"choices":["rows"]}},"pagination":{"id":"value-picker","options":{"choices":["yes","no"]}},"on_row_click":{"id":"json","options":{}},"width":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}},"height":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}},"min_width":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}},"max_width":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}}},"menu":{"items":{"id":"json","options":{"rows":4}},"initial_item":{"id":"value-picker","options":{}},"width":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}},"height":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}},"min_width":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}},"max_width":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}}},"chat-composer":{"placeholder":{"id":"localized-text","options":{}},"rows":{"id":"number","options":{"min":1,"max":20,"integer":true}},"slash_commands":{"id":"switch","options":{}},"toolbar":{"id":"ordered-multi-picker","options":{"choices":["agent","model","prompt","text_editor","new_session"],"minItems":0}},"agents":{"id":"ordered-multi-picker","options":{"choices":["Claude Code","Codex","DeepSeek Harness","pi"],"minItems":1}},"default_agent":{"id":"value-picker","options":{}},"default_model":{"id":"text","options":{}},"width":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}},"height":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}},"min_width":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}},"max_width":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}}},"chat-panel":{"tabs":{"id":"ordered-multi-picker","options":{"choices":["chat","dialog"],"minItems":1}},"initial_tab":{"id":"value-picker","options":{}},"width":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}},"height":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}},"min_width":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}},"max_width":{"id":"length","options":{"keywords":["auto","elastic"],"units":["px","rem","em","%"]}}}}$defaults$::jsonb;
  descriptor record;
  attribute record;
  name text;
  pointer text;
  scope text;
  editor jsonb;
  definition jsonb;
  settings jsonb;
  metadata jsonb;
  next_body jsonb;
BEGIN
  LOCK TABLE kb.wpi_descriptors IN SHARE ROW EXCLUSIVE MODE;
  FOR descriptor IN
    SELECT DISTINCT ON (descriptor_id) descriptor_id, version, body
    FROM kb.wpi_descriptors
    WHERE kind = 'element' AND status IN ('draft', 'active') AND schema_version = '0.4'
    ORDER BY descriptor_id, version DESC
  LOOP
    next_body := descriptor.body;
    metadata := COALESCE(descriptor.body->'attribute_definitions', '{}'::jsonb);
    FOR attribute IN SELECT key, value FROM jsonb_each(descriptor.body)
    LOOP
      name := attribute.key;
      pointer := '/' || replace(replace(name, '~', '~0'), '/', '~1');
      definition := COALESCE(metadata->pointer, '{}'::jsonb);
      IF name = ANY(ARRAY['descriptor_id','schema_version','code_module_id','description','implement_notes']) THEN
        editor := COALESCE(definition->'value_editor', jsonb_build_object(
          'id', CASE WHEN name IN ('description','implement_notes') THEN 'localized-text' ELSE 'text' END,
          'options', '{}'::jsonb));
        metadata := jsonb_set(metadata, ARRAY[pointer], definition || jsonb_build_object('scope','class','value_editor',editor), true);
      ELSIF name <> 'attribute_definitions' AND jsonb_typeof(attribute.value) = 'object' THEN
        settings := attribute.value;
        scope := COALESCE(settings->>'scope', CASE WHEN settings->'instance_modifiable' = 'false'::jsonb THEN 'class' ELSE 'instance' END);
        editor := COALESCE(settings->'value_editor', definition->'value_editor',
          editor_defaults #> ARRAY[descriptor.body->>'code_module_id', name],
          jsonb_build_object('id', CASE
            WHEN name IN ('label','text','description','placeholder','help_text','help_tip') THEN 'localized-text'
            WHEN name IN ('width','height','min_width','max_width','min_height','max_height','padding') OR name ~ '_(gap|margin|size|thickness)$' THEN 'length'
            WHEN name ~ 'color$' THEN 'color'
            WHEN jsonb_typeof(settings->'default_value') = 'boolean' THEN 'switch'
            WHEN jsonb_typeof(settings->'default_value') = 'number' THEN 'number'
            WHEN jsonb_typeof(settings->'default_value') IN ('object','array') THEN 'json'
            ELSE 'text' END, 'options','{}'::jsonb));
        IF NOT (editor ? 'options') THEN editor := editor || '{"options":{}}'::jsonb; END IF;
        settings := settings || jsonb_build_object('scope',scope,'value_editor',editor);
        IF scope = 'class' THEN settings := settings || '{"instance_modifiable":false}'::jsonb;
        ELSIF settings ? 'instance_modifiable' THEN settings := settings || '{"instance_modifiable":true}'::jsonb;
        END IF;
        IF definition ? 'description' THEN settings := settings || jsonb_build_object('help_tip',definition->'description'); END IF;
        next_body := jsonb_set(next_body, ARRAY[name], settings, true);
        metadata := metadata - pointer;
      END IF;
    END LOOP;
    IF metadata = '{}'::jsonb THEN next_body := next_body - 'attribute_definitions';
    ELSE next_body := jsonb_set(next_body, '{attribute_definitions}', metadata, true);
    END IF;
    UPDATE kb.wpi_descriptors SET body = next_body,
      updated_by = 'migration:wpi-inline-attribute-definitions', updated_at = NOW()
    WHERE kind = 'element' AND descriptor_id = descriptor.descriptor_id AND version = descriptor.version
      AND body IS DISTINCT FROM next_body;
  END LOOP;
END
$migration$;
-- +goose StatementEnd

-- +goose Down
-- Reverse the representation while retaining author changes: move inline settings
-- back to pointer metadata; leave actual defaults and help tips untouched.
-- +goose StatementBegin
DO $migration$
DECLARE
  descriptor record;
  attribute record;
  pointer text;
  metadata jsonb;
  next_body jsonb;
BEGIN
  LOCK TABLE kb.wpi_descriptors IN SHARE ROW EXCLUSIVE MODE;
  FOR descriptor IN SELECT descriptor_id, version, body FROM kb.wpi_descriptors
    WHERE kind = 'element' AND status IN ('draft','active') AND schema_version = '0.4'
  LOOP
    next_body := descriptor.body;
    metadata := COALESCE(descriptor.body->'attribute_definitions','{}'::jsonb);
    FOR attribute IN SELECT key,value FROM jsonb_each(descriptor.body)
    LOOP
      IF attribute.key <> 'attribute_definitions' AND jsonb_typeof(attribute.value) = 'object'
         AND attribute.value ? 'scope' AND attribute.value ? 'value_editor' THEN
        pointer := '/' || replace(replace(attribute.key,'~','~0'),'/','~1');
        metadata := jsonb_set(metadata, ARRAY[pointer], COALESCE(metadata->pointer,'{}'::jsonb)
          || jsonb_build_object('scope',attribute.value->'scope','value_editor',attribute.value->'value_editor')
          || CASE WHEN attribute.value ? 'help_tip' THEN jsonb_build_object('description',attribute.value->'help_tip') ELSE '{}'::jsonb END, true);
        next_body := jsonb_set(next_body, ARRAY[attribute.key], attribute.value - 'scope' - 'value_editor', true);
      END IF;
    END LOOP;
    next_body := jsonb_set(next_body,'{attribute_definitions}',metadata,true);
    UPDATE kb.wpi_descriptors SET body = next_body,
      updated_by = 'migration:wpi-inline-attribute-definitions-down', updated_at = NOW()
    WHERE kind = 'element' AND descriptor_id = descriptor.descriptor_id AND version = descriptor.version
      AND body IS DISTINCT FROM next_body;
  END LOOP;
END
$migration$;
-- +goose StatementEnd
