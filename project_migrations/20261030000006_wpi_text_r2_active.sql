-- Activate a new version of the human-approved text R2 contract.
-- Stored draft/active versions and all page class copies remain unchanged.
-- +goose Up
-- +goose StatementBegin
DO $migration$
BEGIN
  LOCK TABLE kb.wpi_descriptors IN SHARE ROW EXCLUSIVE MODE;
  IF NOT EXISTS (SELECT 1 FROM kb.code_modules WHERE code_module_id = 'text' AND module_type = 'element' AND status = 'active') THEN
    RAISE EXCEPTION 'Active text code module is required';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM kb.wpi_descriptors WHERE kind='element' AND descriptor_id='text' AND status IN ('active','draft')) THEN
    RAISE EXCEPTION 'Existing text descriptor is required';
  END IF;
  INSERT INTO kb.wpi_descriptors
    (kind,descriptor_id,version,schema_version,body,status,created_by,updated_by)
  SELECT 'element','text',
    (SELECT COALESCE(MAX(version),0)+1 FROM kb.wpi_descriptors WHERE kind='element' AND descriptor_id='text'),
    '0.4', previous.body || $descriptor${
  "descriptor_id": "text",
  "schema_version": "0.4",
  "code_module_id": "text",
  "description": {
    "en": "Static or bound paragraph with optional caption, icon and tooltip",
    "zh-cn": "支持可选标题、图标和提示的静态或绑定段落"
  },
  "implement_notes": {
    "en": "Approved R2: localized read-only paragraph, optional caption/icon/tooltip and configurable appearance.",
    "zh-cn": "已批准的 R2：本地化只读段落、可选标题/图标/提示及可配置外观。"
  },
  "text": {
    "help_tip": {
      "en": "Literal content; set text or data_binding, not both.",
      "zh-cn": "静态内容；文本和数据绑定只能设置其一。"
    }
  },
  "data_binding": {
    "help_tip": {
      "en": "Input doc path; set text or data_binding, not both.",
      "zh-cn": "输入文档路径；文本和数据绑定只能设置其一。"
    }
  },
  "label": {
    "help_tip": {
      "en": "Optional caption; omit when no caption is needed.",
      "zh-cn": "可选标题；无需标题时请省略。"
    }
  },
  "help_tip": {
    "help_tip": {
      "en": "Optional tooltip; empty means no tooltip.",
      "zh-cn": "可选提示；空字符串表示不显示提示。"
    },
    "default_value": ""
  },
  "icon": {
    "help_tip": {
      "en": "Decorative icon identifier; empty means no icon.",
      "zh-cn": "装饰图标标识；空字符串表示不显示图标。"
    },
    "default_value": ""
  },
  "label_placement": {
    "help_tip": {
      "en": "Caption position relative to the content row.",
      "zh-cn": "标题相对于内容行的位置。"
    },
    "default_value": "top"
  },
  "label_horizontal_gap": {
    "help_tip": {
      "en": "Caption gap for left/right placement.",
      "zh-cn": "标题位于左右时的间距。"
    },
    "default_value": "8px"
  },
  "label_vertical_gap": {
    "help_tip": {
      "en": "Caption gap for top/bottom placement.",
      "zh-cn": "标题位于上下时的间距。"
    },
    "default_value": "4px"
  },
  "label_horizontal_align": {
    "help_tip": {
      "en": "Caption text alignment.",
      "zh-cn": "标题文字对齐方式。"
    },
    "default_value": "start"
  },
  "label_vertical_align": {
    "help_tip": {
      "en": "Caption alignment beside the content row.",
      "zh-cn": "标题在内容行旁的垂直对齐方式。"
    },
    "default_value": "top"
  },
  "label_max_width": {
    "help_tip": {
      "en": "Maximum caption width; all caption text remains available.",
      "zh-cn": "标题最大宽度；保留完整标题文字。"
    },
    "default_value": "160px"
  },
  "label_font_weight": {
    "help_tip": {
      "en": "Caption font weight.",
      "zh-cn": "标题字重。"
    },
    "default_value": 500
  },
  "icon_placement": {
    "help_tip": {
      "en": "Icon position relative to paragraph text.",
      "zh-cn": "图标相对于段落文字的位置。"
    },
    "default_value": "start"
  },
  "icon_size": {
    "help_tip": {
      "en": "Decorative icon size.",
      "zh-cn": "装饰图标尺寸。"
    },
    "default_value": "16px"
  },
  "icon_gap": {
    "help_tip": {
      "en": "Gap between icon and paragraph.",
      "zh-cn": "图标与段落的间距。"
    },
    "default_value": "8px"
  },
  "icon_color": {
    "help_tip": {
      "en": "Icon color; inherit follows content foreground.",
      "zh-cn": "图标颜色；inherit 跟随内容前景色。"
    },
    "default_value": "inherit"
  },
  "font_family": {
    "help_tip": {
      "en": "Content font family; inherit uses surrounding font.",
      "zh-cn": "内容字体；inherit 使用周围字体。"
    },
    "default_value": "inherit"
  },
  "font_size": {
    "help_tip": {
      "en": "Content font size.",
      "zh-cn": "内容字号。"
    },
    "default_value": "0.875rem"
  },
  "font_weight": {
    "help_tip": {
      "en": "Content font weight; inherit uses surrounding weight.",
      "zh-cn": "内容字重；inherit 使用周围字重。"
    },
    "default_value": "inherit"
  },
  "font_style": {
    "help_tip": {
      "en": "Content font style.",
      "zh-cn": "内容字体样式。"
    },
    "default_value": "normal"
  },
  "line_height": {
    "help_tip": {
      "en": "Line height; normal preserves module small-text line height.",
      "zh-cn": "行高；normal 保留模块的小号文本行高。"
    },
    "default_value": "normal"
  },
  "color": {
    "help_tip": {
      "en": "Content foreground; inherit follows page/theme.",
      "zh-cn": "内容前景色；inherit 跟随页面或主题。"
    },
    "default_value": "inherit"
  },
  "background_color": {
    "help_tip": {
      "en": "Whole element background.",
      "zh-cn": "整个元素的背景颜色。"
    },
    "default_value": "transparent"
  },
  "text_align": {
    "help_tip": {
      "en": "Paragraph alignment within its box.",
      "zh-cn": "段落在自身区域中的对齐方式。"
    },
    "default_value": "start"
  },
  "white_space": {
    "help_tip": {
      "en": "Whitespace handling; pre-wrap preserves line breaks.",
      "zh-cn": "空白处理；pre-wrap 保留换行。"
    },
    "default_value": "normal"
  },
  "padding": {
    "help_tip": {
      "en": "Whole element padding.",
      "zh-cn": "整个元素的内边距。"
    },
    "default_value": "0.25rem 0px"
  },
  "width": {
    "help_tip": {
      "en": "Whole element width.",
      "zh-cn": "整个元素的宽度。"
    },
    "default_value": "auto"
  },
  "height": {
    "help_tip": {
      "en": "Whole element height; prefer auto for prose.",
      "zh-cn": "整个元素的高度；段落建议使用 auto。"
    },
    "default_value": "auto"
  },
  "min_width": {
    "help_tip": {
      "en": "Minimum whole element width.",
      "zh-cn": "整个元素的最小宽度。"
    },
    "default_value": "0px"
  },
  "max_width": {
    "help_tip": {
      "en": "Optional maximum width; omit for no author-imposed cap.",
      "zh-cn": "可选最大宽度；省略表示不设置作者指定的上限。"
    }
  }
}$descriptor$::jsonb,
    'active','seed:wpi-text-r2-active','seed:wpi-text-r2-active'
  FROM (
    SELECT body FROM kb.wpi_descriptors WHERE kind='element' AND descriptor_id='text' AND status IN ('active','draft')
    ORDER BY version DESC LIMIT 1
  ) AS previous
  WHERE NOT EXISTS (SELECT 1 FROM kb.wpi_descriptors WHERE kind='element' AND descriptor_id='text' AND created_by='seed:wpi-text-r2-active');
  UPDATE kb.code_modules SET module_doc='interpreter-text.md',updated_at=NOW() WHERE code_module_id='text';
END;
$migration$;
-- +goose StatementEnd

-- +goose Down
DELETE FROM kb.wpi_descriptors WHERE kind='element' AND descriptor_id='text' AND created_by='seed:wpi-text-r2-active';
-- The canonical module document path is also correct for the prior implementation.
