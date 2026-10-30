-- Expand the input class without modifying saved page class copies.
-- +goose Up
INSERT INTO kb.wpi_descriptors (kind, descriptor_id, version, schema_version, body, status, created_by, updated_by)
SELECT kind, descriptor_id, (SELECT COALESCE(max(version), 0) + 1 FROM kb.wpi_descriptors WHERE kind = 'element' AND descriptor_id = 'input'), schema_version, body || $attrs${
  "input_type": {
    "help_tip": {
      "en": "Input type: text, password, email, number, tel, url or search.",
      "zh-cn": "输入类型：text、password、email、number、tel、url 或 search。"
    },
    "default_value": "text"
  },
  "label_placement": {
    "help_tip": {
      "en": "Caption position relative to the input.",
      "zh-cn": "标题相对于输入框的位置。"
    },
    "default_value": "left"
  },
  "label_horizontal_gap": {
    "help_tip": {
      "en": "Horizontal caption gap in pixels.",
      "zh-cn": "标题的水平间距（像素）。"
    },
    "default_value": "10px"
  },
  "label_vertical_gap": {
    "help_tip": {
      "en": "Vertical caption gap in pixels.",
      "zh-cn": "标题的垂直间距（像素）。"
    },
    "default_value": "10px"
  },
  "label_horizontal_align": {
    "help_tip": {
      "en": "Caption text horizontal alignment.",
      "zh-cn": "标题文字的水平对齐方式。"
    },
    "default_value": "right"
  },
  "label_vertical_align": {
    "help_tip": {
      "en": "Caption vertical alignment next to the control.",
      "zh-cn": "标题相对于输入框的垂直对齐方式。"
    },
    "default_value": "center"
  },
  "label_max_width": {
    "help_tip": {
      "en": "Maximum caption width.",
      "zh-cn": "标题最大宽度。"
    },
    "default_value": "160px"
  },
  "label_max_lines": {
    "help_tip": {
      "en": "Maximum number of caption lines.",
      "zh-cn": "标题最多显示的行数。"
    },
    "default_value": 1
  },
  "label_font_size": {
    "help_tip": {
      "en": "Caption font size in pixels.",
      "zh-cn": "标题字号（像素）。"
    },
    "default_value": "14px"
  },
  "label_font_weight": {
    "help_tip": {
      "en": "Caption font weight.",
      "zh-cn": "标题字重。"
    },
    "default_value": "500"
  },
  "label_font_style": {
    "help_tip": {
      "en": "Caption font style.",
      "zh-cn": "标题字体样式。"
    },
    "default_value": "normal"
  },
  "required": {
    "help_tip": {
      "en": "Whether an editable value is required.",
      "zh-cn": "是否必须填写可编辑的值。"
    },
    "default_value": false
  },
  "label_font_family": {
    "help_tip": {
      "en": "Caption font family.",
      "zh-cn": "标题字体。"
    }
  },
  "label_color": {
    "help_tip": {
      "en": "Caption color.",
      "zh-cn": "标题颜色。"
    }
  },
  "placeholder": {
    "help_tip": {
      "en": "Prompt shown in an empty control.",
      "zh-cn": "空输入框中显示的提示。"
    }
  },
  "help_text": {
    "help_tip": {
      "en": "Help shown below the control.",
      "zh-cn": "输入框下方显示的帮助。"
    }
  },
  "help_tip": {
    "help_tip": {
      "en": "Additional help available on request.",
      "zh-cn": "按需显示的补充帮助。"
    }
  },
  "autocomplete": {
    "help_tip": {
      "en": "Browser autocomplete token.",
      "zh-cn": "浏览器自动填充标记。"
    }
  },
  "max_length": {
    "help_tip": {
      "en": "Maximum Unicode code points.",
      "zh-cn": "最多允许的 Unicode 码点数。"
    }
  },
  "validation": {
    "help_tip": {
      "en": "Additional validation rules.",
      "zh-cn": "其他验证规则。"
    }
  },
  "height": {
    "help_tip": {
      "en": "Height of the complete field.",
      "zh-cn": "整个字段的高度。"
    }
  }
}$attrs$::jsonb, 'active', 'seed:wpi-input-attributes', 'seed:wpi-input-attributes'
FROM kb.wpi_descriptors
WHERE kind = 'element' AND descriptor_id = 'input' AND status = 'active'
  AND version = (SELECT max(version) FROM kb.wpi_descriptors WHERE kind = 'element' AND descriptor_id = 'input' AND status = 'active')
  AND NOT EXISTS (SELECT 1 FROM kb.wpi_descriptors WHERE kind = 'element' AND descriptor_id = 'input' AND created_by = 'seed:wpi-input-attributes');

-- +goose Down
DELETE FROM kb.wpi_descriptors WHERE kind = 'element' AND descriptor_id = 'input' AND created_by = 'seed:wpi-input-attributes';
