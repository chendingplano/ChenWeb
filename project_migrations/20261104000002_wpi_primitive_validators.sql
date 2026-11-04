-- The primitive validators of the validator requirements §3.1 (KnowledgeStore
-- doc-repo/validators/validator-requirements.md; OpenSpec wpi-primitive-validators;
-- contract in KnowledgeStore doc-repo/code_modules/validator-primitive.md):
-- kb.wpi_descriptors gains kind 'validator' and kb.code_modules module type
-- 'validator', and one active class and one code module are stored for each of
-- string, integer, decimal, number, boolean, enum, null, binary and bigint.
-- Generated from server/api/wpi/validators/modules.json; seed_test.go checks that
-- the rows still state what the code accepts.
-- +goose Up
-- +goose StatementBegin
ALTER TABLE kb.wpi_descriptors DROP CONSTRAINT IF EXISTS wpi_descriptors_kind_check;
ALTER TABLE kb.wpi_descriptors
    ADD CONSTRAINT wpi_descriptors_kind_check CHECK (kind IN ('app', 'element', 'action', 'page-layouter', 'element-layouter', 'validator'));
COMMENT ON TABLE kb.wpi_descriptors IS
    'Web Page Interpreter class descriptors (app, element, action, page-layouter, element-layouter, '
    'validator), one row per version. Versions are append-only; the latest active version is the one in use.';
ALTER TABLE kb.code_modules DROP CONSTRAINT IF EXISTS code_modules_module_type_check;
ALTER TABLE kb.code_modules
    ADD CONSTRAINT code_modules_module_type_check CHECK (module_type IN ('element', 'action', 'page', 'page-layouter', 'element-layouter', 'validator'));
COMMENT ON COLUMN kb.code_modules.module_type IS
    'What the module implements: element, action, page, page-layouter, element-layouter or validator.';
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO kb.code_modules (code_module_id, module_name, description, module_type, module_doc, notes, creator) VALUES
    ('string', 'String Validator',
     'Text, with length, pattern, trimming and normalization',
     'validator', 'validator-primitive.md',
     'Primitive validator (requirements §3.1); checks run in the browser and the backend.',
     'migration:wpi-primitive-validators'),
    ('integer', 'Integer Validator',
     'A whole number that JavaScript holds exactly, with limits, sign and step',
     'validator', 'validator-primitive.md',
     'Primitive validator (requirements §3.1); checks run in the browser and the backend.',
     'migration:wpi-primitive-validators'),
    ('decimal', 'Decimal Validator',
     'An exact decimal number with precision, scale and rounding',
     'validator', 'validator-primitive.md',
     'Primitive validator (requirements §3.1); checks run in the browser and the backend.',
     'migration:wpi-primitive-validators'),
    ('number', 'Number Validator',
     'A number, optionally whole and finite, with limits and step',
     'validator', 'validator-primitive.md',
     'Primitive validator (requirements §3.1); checks run in the browser and the backend.',
     'migration:wpi-primitive-validators'),
    ('boolean', 'Boolean Validator',
     'True or false, optionally one of them',
     'validator', 'validator-primitive.md',
     'Primitive validator (requirements §3.1); checks run in the browser and the backend.',
     'migration:wpi-primitive-validators'),
    ('enum', 'Enum Validator',
     'One of the allowed values, and none of the excluded ones',
     'validator', 'validator-primitive.md',
     'Primitive validator (requirements §3.1); checks run in the browser and the backend.',
     'migration:wpi-primitive-validators'),
    ('null', 'Null Validator',
     'Null, or not null',
     'validator', 'validator-primitive.md',
     'Primitive validator (requirements §3.1); checks run in the browser and the backend.',
     'migration:wpi-primitive-validators'),
    ('binary', 'Binary Validator',
     'Bytes written as base64, base64url or hex, with size limits',
     'validator', 'validator-primitive.md',
     'Primitive validator (requirements §3.1); checks run in the browser and the backend.',
     'migration:wpi-primitive-validators'),
    ('bigint', 'Bigint Validator',
     'A whole number of any size, as digits, with limits and digit count',
     'validator', 'validator-primitive.md',
     'Primitive validator (requirements §3.1); checks run in the browser and the backend.',
     'migration:wpi-primitive-validators')
ON CONFLICT (code_module_id) DO NOTHING;
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO kb.wpi_descriptors (kind, descriptor_id, version, schema_version, body, status, created_by, updated_by)
SELECT 'validator', v.id, 1, '0.1', v.body, 'active', 'seed:wpi-primitive-validators', 'seed:wpi-primitive-validators'
FROM (VALUES
('string', $wpi${
  "descriptor_id": "string",
  "schema_version": "0.1",
  "code_module_id": "string",
  "category": "primitive",
  "description": {
    "en": "Text, with length, pattern, trimming and normalization",
    "zh-cn": "文本：长度、模式、去空白与规范化"
  },
  "implement_notes": {
    "en": "KnowledgeStore/doc-repo/code_modules/validator-primitive.md"
  },
  "value_types": [
    "string"
  ],
  "empty_values": [
    "absent",
    "null",
    "empty_string"
  ],
  "execution": "sync",
  "runs_in": "browser_and_backend",
  "error_codes": {
    "string.type": {
      "params": []
    },
    "string.empty": {
      "params": []
    },
    "string.min_length": {
      "params": [
        "min_length",
        "length_unit",
        "actual"
      ]
    },
    "string.max_length": {
      "params": [
        "max_length",
        "length_unit",
        "actual"
      ]
    },
    "string.blank": {
      "params": []
    },
    "string.line_break": {
      "params": []
    },
    "string.pattern": {
      "params": [
        "pattern"
      ]
    },
    "string.pattern_limit": {
      "params": [
        "limit"
      ]
    },
    "string.prefix": {
      "params": [
        "prefix"
      ]
    },
    "string.suffix": {
      "params": [
        "suffix"
      ]
    },
    "string.contains": {
      "params": [
        "contains"
      ]
    }
  },
  "constraints": {
    "empty": {
      "type": "choice",
      "help_tip": {
        "en": "What an absent, null or empty value does: pass accepts it with no other check; fail rejects it; check reads it as empty text and applies the other constraints (the default).",
        "zh-cn": "缺失、null 或空值的处理：pass 直接接受，不做其他检查；fail 拒绝；check 视为空文本并应用其他约束（默认）。"
      },
      "choices": [
        "pass",
        "fail",
        "check"
      ],
      "default_value": "check"
    },
    "trim": {
      "type": "choice",
      "help_tip": {
        "en": "Removes whitespace at the start, the end or both before the other checks.",
        "zh-cn": "在其他检查之前，去除开头、结尾或两端的空白。"
      },
      "choices": [
        "none",
        "both",
        "start",
        "end"
      ],
      "default_value": "none"
    },
    "normalization": {
      "type": "choice",
      "help_tip": {
        "en": "Unicode normalization applied before the other checks, such as NFC.",
        "zh-cn": "在其他检查之前应用的 Unicode 规范化，例如 NFC。"
      },
      "choices": [
        "none",
        "nfc",
        "nfd",
        "nfkc",
        "nfkd"
      ],
      "default_value": "none"
    },
    "length_unit": {
      "type": "choice",
      "help_tip": {
        "en": "What min_length and max_length count: code points (the default), UTF-16 code units or UTF-8 bytes.",
        "zh-cn": "min_length 和 max_length 计数的单位：码位（默认）、UTF-16 码元或 UTF-8 字节。"
      },
      "choices": [
        "code_points",
        "utf16_units",
        "utf8_bytes"
      ],
      "default_value": "code_points"
    },
    "min_length": {
      "type": "non_negative_integer",
      "help_tip": {
        "en": "The shortest length allowed.",
        "zh-cn": "允许的最短长度。"
      }
    },
    "max_length": {
      "type": "non_negative_integer",
      "help_tip": {
        "en": "The longest length allowed.",
        "zh-cn": "允许的最长长度。"
      }
    },
    "non_blank": {
      "type": "boolean",
      "help_tip": {
        "en": "Rejects text that is only whitespace, empty text included.",
        "zh-cn": "拒绝仅由空白组成的文本（包括空文本）。"
      },
      "default_value": false
    },
    "single_line": {
      "type": "boolean",
      "help_tip": {
        "en": "Rejects line breaks.",
        "zh-cn": "拒绝换行。"
      },
      "default_value": false
    },
    "pattern": {
      "type": "pattern",
      "help_tip": {
        "en": "A regular expression the whole value must match, in syntax Go and JavaScript read the same way. Values over 10,000 characters fail.",
        "zh-cn": "整个值必须匹配的正则表达式，须使用 Go 与 JavaScript 解释一致的语法。超过 10,000 个字符的值不予通过。"
      }
    },
    "prefix": {
      "type": "string",
      "help_tip": {
        "en": "Text the value must start with.",
        "zh-cn": "值必须以此文本开头。"
      }
    },
    "suffix": {
      "type": "string",
      "help_tip": {
        "en": "Text the value must end with.",
        "zh-cn": "值必须以此文本结尾。"
      }
    },
    "contains": {
      "type": "string",
      "help_tip": {
        "en": "Text the value must contain.",
        "zh-cn": "值必须包含此文本。"
      }
    }
  },
  "empty_constraint": "empty"
}$wpi$::jsonb),
('integer', $wpi${
  "descriptor_id": "integer",
  "schema_version": "0.1",
  "code_module_id": "integer",
  "category": "primitive",
  "description": {
    "en": "A whole number that JavaScript holds exactly, with limits, sign and step",
    "zh-cn": "JavaScript 可精确表示的整数：范围、符号与步长"
  },
  "implement_notes": {
    "en": "KnowledgeStore/doc-repo/code_modules/validator-primitive.md"
  },
  "value_types": [
    "number",
    "string"
  ],
  "empty_values": [
    "absent",
    "null",
    "empty_string"
  ],
  "execution": "sync",
  "runs_in": "browser_and_backend",
  "error_codes": {
    "integer.type": {
      "params": []
    },
    "integer.empty": {
      "params": []
    },
    "integer.unsafe": {
      "params": []
    },
    "integer.minimum": {
      "params": [
        "minimum"
      ]
    },
    "integer.maximum": {
      "params": [
        "maximum"
      ]
    },
    "integer.exclusive_minimum": {
      "params": [
        "exclusive_minimum"
      ]
    },
    "integer.exclusive_maximum": {
      "params": [
        "exclusive_maximum"
      ]
    },
    "integer.sign": {
      "params": [
        "sign"
      ]
    },
    "integer.multiple_of": {
      "params": [
        "multiple_of"
      ]
    },
    "integer.range": {
      "params": [
        "representation",
        "lowest",
        "highest"
      ]
    }
  },
  "constraints": {
    "empty": {
      "type": "choice",
      "help_tip": {
        "en": "What an absent, null or empty value does: pass accepts it with no other check (the default); fail rejects it, which makes the value required.",
        "zh-cn": "缺失、null 或空值的处理：pass 直接接受，不做其他检查（默认）；fail 拒绝，即该值必填。"
      },
      "choices": [
        "pass",
        "fail"
      ],
      "default_value": "pass"
    },
    "accept_strings": {
      "type": "boolean",
      "help_tip": {
        "en": "Accepts text such as \" 42 \", read after removing spaces.",
        "zh-cn": "接受文本（例如 \" 42 \"），去除空格后读取。"
      },
      "default_value": true
    },
    "minimum": {
      "type": "integer",
      "help_tip": {
        "en": "The smallest value allowed.",
        "zh-cn": "允许的最小值。"
      }
    },
    "maximum": {
      "type": "integer",
      "help_tip": {
        "en": "The largest value allowed.",
        "zh-cn": "允许的最大值。"
      }
    },
    "exclusive_minimum": {
      "type": "integer",
      "help_tip": {
        "en": "The value must be greater than this.",
        "zh-cn": "值必须大于此数。"
      }
    },
    "exclusive_maximum": {
      "type": "integer",
      "help_tip": {
        "en": "The value must be less than this.",
        "zh-cn": "值必须小于此数。"
      }
    },
    "sign": {
      "type": "choice",
      "help_tip": {
        "en": "Whether the value must be positive, negative, non-negative, non-positive or non-zero.",
        "zh-cn": "值是否必须为正、负、非负、非正或非零。"
      },
      "choices": [
        "any",
        "positive",
        "negative",
        "non_negative",
        "non_positive",
        "non_zero"
      ],
      "default_value": "any"
    },
    "multiple_of": {
      "type": "positive_integer",
      "help_tip": {
        "en": "The value must be an exact multiple of this.",
        "zh-cn": "值必须是此数的整数倍。"
      }
    },
    "representation": {
      "type": "choice",
      "help_tip": {
        "en": "A bounded integer type the value must fit, such as int32.",
        "zh-cn": "值必须能放入的有界整数类型，例如 int32。"
      },
      "choices": [
        "any",
        "int8",
        "int16",
        "int32",
        "uint8",
        "uint16",
        "uint32"
      ],
      "default_value": "any"
    }
  },
  "empty_constraint": "empty"
}$wpi$::jsonb),
('decimal', $wpi${
  "descriptor_id": "decimal",
  "schema_version": "0.1",
  "code_module_id": "decimal",
  "category": "primitive",
  "description": {
    "en": "An exact decimal number with precision, scale and rounding",
    "zh-cn": "精确十进制数：精度、小数位数与舍入"
  },
  "implement_notes": {
    "en": "KnowledgeStore/doc-repo/code_modules/validator-primitive.md"
  },
  "value_types": [
    "string",
    "number"
  ],
  "empty_values": [
    "absent",
    "null",
    "empty_string"
  ],
  "execution": "sync",
  "runs_in": "browser_and_backend",
  "error_codes": {
    "decimal.type": {
      "params": []
    },
    "decimal.empty": {
      "params": []
    },
    "decimal.scale": {
      "params": [
        "scale",
        "actual"
      ]
    },
    "decimal.precision": {
      "params": [
        "precision",
        "actual"
      ]
    },
    "decimal.integer_digits": {
      "params": [
        "integer_digits",
        "actual"
      ]
    },
    "decimal.minimum": {
      "params": [
        "minimum"
      ]
    },
    "decimal.maximum": {
      "params": [
        "maximum"
      ]
    },
    "decimal.exclusive_minimum": {
      "params": [
        "exclusive_minimum"
      ]
    },
    "decimal.exclusive_maximum": {
      "params": [
        "exclusive_maximum"
      ]
    },
    "decimal.sign": {
      "params": [
        "sign"
      ]
    },
    "decimal.multiple_of": {
      "params": [
        "multiple_of"
      ]
    }
  },
  "constraints": {
    "empty": {
      "type": "choice",
      "help_tip": {
        "en": "What an absent, null or empty value does: pass accepts it with no other check (the default); fail rejects it, which makes the value required.",
        "zh-cn": "缺失、null 或空值的处理：pass 直接接受，不做其他检查（默认）；fail 拒绝，即该值必填。"
      },
      "choices": [
        "pass",
        "fail"
      ],
      "default_value": "pass"
    },
    "accept_numbers": {
      "type": "boolean",
      "help_tip": {
        "en": "Accepts JSON numbers, read by their shortest form; send text to keep every digit.",
        "zh-cn": "接受 JSON 数字（按最短形式读取）；需保留全部位数时请用文本。"
      },
      "default_value": true
    },
    "scale": {
      "type": "non_negative_integer",
      "help_tip": {
        "en": "The most digits after the point.",
        "zh-cn": "小数点后的最多位数。"
      }
    },
    "rounding": {
      "type": "choice",
      "help_tip": {
        "en": "With scale: reject extra digits, or round them half up, half even, down, up, toward minus infinity (floor) or plus infinity (ceiling).",
        "zh-cn": "与 scale 同用：拒绝多余位数，或按四舍五入、银行家舍入、截断、进位、向负无穷（floor）或向正无穷（ceiling）取整。"
      },
      "choices": [
        "reject",
        "half_up",
        "half_even",
        "down",
        "up",
        "floor",
        "ceiling"
      ],
      "default_value": "reject"
    },
    "precision": {
      "type": "positive_integer",
      "help_tip": {
        "en": "The most digits in all; with scale, at most precision − scale digits before the point, as SQL NUMERIC.",
        "zh-cn": "总位数上限；与 scale 同用时，小数点前至多 precision − scale 位，同 SQL NUMERIC。"
      }
    },
    "minimum": {
      "type": "decimal",
      "help_tip": {
        "en": "The smallest value allowed.",
        "zh-cn": "允许的最小值。"
      }
    },
    "maximum": {
      "type": "decimal",
      "help_tip": {
        "en": "The largest value allowed.",
        "zh-cn": "允许的最大值。"
      }
    },
    "exclusive_minimum": {
      "type": "decimal",
      "help_tip": {
        "en": "The value must be greater than this.",
        "zh-cn": "值必须大于此数。"
      }
    },
    "exclusive_maximum": {
      "type": "decimal",
      "help_tip": {
        "en": "The value must be less than this.",
        "zh-cn": "值必须小于此数。"
      }
    },
    "sign": {
      "type": "choice",
      "help_tip": {
        "en": "Whether the value must be positive, negative, non-negative, non-positive or non-zero.",
        "zh-cn": "值是否必须为正、负、非负、非正或非零。"
      },
      "choices": [
        "any",
        "positive",
        "negative",
        "non_negative",
        "non_positive",
        "non_zero"
      ],
      "default_value": "any"
    },
    "multiple_of": {
      "type": "positive_decimal",
      "help_tip": {
        "en": "The value must be an exact multiple of this.",
        "zh-cn": "值必须是此数的整数倍。"
      }
    }
  },
  "empty_constraint": "empty"
}$wpi$::jsonb),
('number', $wpi${
  "descriptor_id": "number",
  "schema_version": "0.1",
  "code_module_id": "number",
  "category": "primitive",
  "description": {
    "en": "A number, optionally whole and finite, with limits and step",
    "zh-cn": "数字：可限定为整数与有限值，含范围与步长"
  },
  "implement_notes": {
    "en": "KnowledgeStore/doc-repo/code_modules/validator-primitive.md"
  },
  "value_types": [
    "number",
    "string"
  ],
  "empty_values": [
    "absent",
    "null",
    "empty_string"
  ],
  "execution": "sync",
  "runs_in": "browser_and_backend",
  "error_codes": {
    "number.type": {
      "params": []
    },
    "number.empty": {
      "params": []
    },
    "number.finite": {
      "params": []
    },
    "number.integer": {
      "params": []
    },
    "number.minimum": {
      "params": [
        "minimum"
      ]
    },
    "number.maximum": {
      "params": [
        "maximum"
      ]
    },
    "number.exclusive_minimum": {
      "params": [
        "exclusive_minimum"
      ]
    },
    "number.exclusive_maximum": {
      "params": [
        "exclusive_maximum"
      ]
    },
    "number.sign": {
      "params": [
        "sign"
      ]
    },
    "number.multiple_of": {
      "params": [
        "multiple_of"
      ]
    }
  },
  "constraints": {
    "empty": {
      "type": "choice",
      "help_tip": {
        "en": "What an absent, null or empty value does: pass accepts it with no other check (the default); fail rejects it, which makes the value required.",
        "zh-cn": "缺失、null 或空值的处理：pass 直接接受，不做其他检查（默认）；fail 拒绝，即该值必填。"
      },
      "choices": [
        "pass",
        "fail"
      ],
      "default_value": "pass"
    },
    "accept_strings": {
      "type": "boolean",
      "help_tip": {
        "en": "Accepts text such as \"-2.5e3\", read after removing spaces.",
        "zh-cn": "接受文本（例如 \"-2.5e3\"），去除空格后读取。"
      },
      "default_value": true
    },
    "allow_non_finite": {
      "type": "boolean",
      "help_tip": {
        "en": "Accepts the text Infinity, -Infinity and NaN; the other constraints do not apply to them.",
        "zh-cn": "接受文本 Infinity、-Infinity 和 NaN；其他约束不适用于这些值。"
      },
      "default_value": false
    },
    "integer_only": {
      "type": "boolean",
      "help_tip": {
        "en": "Rejects numbers with a fraction.",
        "zh-cn": "拒绝带小数部分的数字。"
      },
      "default_value": false
    },
    "minimum": {
      "type": "number",
      "help_tip": {
        "en": "The smallest value allowed.",
        "zh-cn": "允许的最小值。"
      }
    },
    "maximum": {
      "type": "number",
      "help_tip": {
        "en": "The largest value allowed.",
        "zh-cn": "允许的最大值。"
      }
    },
    "exclusive_minimum": {
      "type": "number",
      "help_tip": {
        "en": "The value must be greater than this.",
        "zh-cn": "值必须大于此数。"
      }
    },
    "exclusive_maximum": {
      "type": "number",
      "help_tip": {
        "en": "The value must be less than this.",
        "zh-cn": "值必须小于此数。"
      }
    },
    "sign": {
      "type": "choice",
      "help_tip": {
        "en": "Whether the value must be positive, negative, non-negative, non-positive or non-zero.",
        "zh-cn": "值是否必须为正、负、非负、非正或非零。"
      },
      "choices": [
        "any",
        "positive",
        "negative",
        "non_negative",
        "non_positive",
        "non_zero"
      ],
      "default_value": "any"
    },
    "multiple_of": {
      "type": "positive_number",
      "help_tip": {
        "en": "The value must be an exact multiple of this.",
        "zh-cn": "值必须是此数的整数倍。"
      }
    }
  },
  "empty_constraint": "empty"
}$wpi$::jsonb),
('boolean', $wpi${
  "descriptor_id": "boolean",
  "schema_version": "0.1",
  "code_module_id": "boolean",
  "category": "primitive",
  "description": {
    "en": "True or false, optionally one of them",
    "zh-cn": "真或假，可限定其一"
  },
  "implement_notes": {
    "en": "KnowledgeStore/doc-repo/code_modules/validator-primitive.md"
  },
  "value_types": [
    "boolean",
    "string",
    "number"
  ],
  "empty_values": [
    "absent",
    "null",
    "empty_string"
  ],
  "execution": "sync",
  "runs_in": "browser_and_backend",
  "error_codes": {
    "boolean.type": {
      "params": []
    },
    "boolean.empty": {
      "params": []
    },
    "boolean.must_be": {
      "params": [
        "must_be"
      ]
    }
  },
  "constraints": {
    "empty": {
      "type": "choice",
      "help_tip": {
        "en": "What an absent, null or empty value does: pass accepts it with no other check (the default); fail rejects it; check reads it as false, so must_be true rejects it.",
        "zh-cn": "缺失、null 或空值的处理：pass 直接接受，不做其他检查（默认）；fail 拒绝；check 视为 false，因此 must_be 为 true 时会被拒绝。"
      },
      "choices": [
        "pass",
        "fail",
        "check"
      ],
      "default_value": "pass"
    },
    "accept_strings": {
      "type": "boolean",
      "help_tip": {
        "en": "Accepts the text true or false, in any case.",
        "zh-cn": "接受任意大小写的文本 true 或 false。"
      },
      "default_value": false
    },
    "accept_numbers": {
      "type": "boolean",
      "help_tip": {
        "en": "Accepts 1 for true and 0 for false.",
        "zh-cn": "接受 1 表示真、0 表示假。"
      },
      "default_value": false
    },
    "must_be": {
      "type": "choice",
      "help_tip": {
        "en": "Whether the value must be true, false, or either.",
        "zh-cn": "值必须为真、假，或两者皆可。"
      },
      "choices": [
        "either",
        "true",
        "false"
      ],
      "default_value": "either"
    }
  },
  "empty_constraint": "empty"
}$wpi$::jsonb),
('enum', $wpi${
  "descriptor_id": "enum",
  "schema_version": "0.1",
  "code_module_id": "enum",
  "category": "primitive",
  "description": {
    "en": "One of the allowed values, and none of the excluded ones",
    "zh-cn": "允许值之一，且不属于排除值"
  },
  "implement_notes": {
    "en": "KnowledgeStore/doc-repo/code_modules/validator-primitive.md"
  },
  "value_types": [
    "string",
    "number",
    "boolean",
    "null",
    "object",
    "array"
  ],
  "empty_values": [
    "absent",
    "null",
    "empty_string"
  ],
  "execution": "sync",
  "runs_in": "browser_and_backend",
  "error_codes": {
    "enum.empty": {
      "params": []
    },
    "enum.not_allowed": {
      "params": [
        "allowed_values"
      ]
    },
    "enum.excluded": {
      "params": [
        "excluded_values"
      ]
    }
  },
  "constraints": {
    "empty": {
      "type": "choice",
      "help_tip": {
        "en": "What an absent, null or empty value does: pass accepts it with no other check (the default); fail rejects it; check compares it with the allowed and excluded values (absent as null).",
        "zh-cn": "缺失、null 或空值的处理：pass 直接接受，不做其他检查（默认）；fail 拒绝；check 将其与允许值和排除值比较（缺失视为 null）。"
      },
      "choices": [
        "pass",
        "fail",
        "check"
      ],
      "default_value": "pass"
    },
    "allowed_values": {
      "type": "json_array",
      "help_tip": {
        "en": "The values allowed, as a JSON array; every instance must set it.",
        "zh-cn": "允许的值（JSON 数组）；每个实例都必须设置。"
      },
      "default_value": null
    },
    "excluded_values": {
      "type": "json_array",
      "help_tip": {
        "en": "Values that are not allowed, as a JSON array.",
        "zh-cn": "不允许的值（JSON 数组）。"
      }
    },
    "case_sensitive": {
      "type": "boolean",
      "help_tip": {
        "en": "Whether text compares with its case.",
        "zh-cn": "比较文本时是否区分大小写。"
      },
      "default_value": true
    },
    "type_sensitive": {
      "type": "boolean",
      "help_tip": {
        "en": "When off, 1 equals \"1\" and true equals \"true\".",
        "zh-cn": "关闭时，1 等于 \"1\"，true 等于 \"true\"。"
      },
      "default_value": true
    }
  },
  "empty_constraint": "empty"
}$wpi$::jsonb),
('null', $wpi${
  "descriptor_id": "null",
  "schema_version": "0.1",
  "code_module_id": "null",
  "category": "primitive",
  "description": {
    "en": "Null, or not null",
    "zh-cn": "为 null 或不为 null"
  },
  "implement_notes": {
    "en": "KnowledgeStore/doc-repo/code_modules/validator-primitive.md"
  },
  "value_types": [
    "string",
    "number",
    "boolean",
    "null",
    "object",
    "array"
  ],
  "empty_values": [],
  "execution": "sync",
  "runs_in": "browser_and_backend",
  "error_codes": {
    "null.must_be_null": {
      "params": []
    },
    "null.must_not_be_null": {
      "params": []
    }
  },
  "constraints": {
    "mode": {
      "type": "choice",
      "help_tip": {
        "en": "Whether the value must be null or must not be null.",
        "zh-cn": "值必须为 null 还是不得为 null。"
      },
      "choices": [
        "must_be_null",
        "must_not_be_null"
      ],
      "default_value": "must_be_null"
    },
    "absent_is_null": {
      "type": "boolean",
      "help_tip": {
        "en": "Counts an absent value as null.",
        "zh-cn": "将缺失的值视为 null。"
      },
      "default_value": true
    },
    "empty_string_is_null": {
      "type": "boolean",
      "help_tip": {
        "en": "Counts empty text as null.",
        "zh-cn": "将空文本视为 null。"
      },
      "default_value": false
    }
  }
}$wpi$::jsonb),
('binary', $wpi${
  "descriptor_id": "binary",
  "schema_version": "0.1",
  "code_module_id": "binary",
  "category": "primitive",
  "description": {
    "en": "Bytes written as base64, base64url or hex, with size limits",
    "zh-cn": "以 base64、base64url 或 hex 编码的字节，含大小限制"
  },
  "implement_notes": {
    "en": "KnowledgeStore/doc-repo/code_modules/validator-primitive.md"
  },
  "value_types": [
    "string"
  ],
  "empty_values": [
    "absent",
    "null",
    "empty_string"
  ],
  "execution": "sync",
  "runs_in": "browser_and_backend",
  "error_codes": {
    "binary.type": {
      "params": []
    },
    "binary.empty": {
      "params": []
    },
    "binary.encoding": {
      "params": [
        "encoding"
      ]
    },
    "binary.padding": {
      "params": [
        "padding"
      ]
    },
    "binary.case": {
      "params": [
        "hex_case"
      ]
    },
    "binary.min_bytes": {
      "params": [
        "min_bytes",
        "actual"
      ]
    },
    "binary.max_bytes": {
      "params": [
        "max_bytes",
        "actual"
      ]
    }
  },
  "constraints": {
    "empty": {
      "type": "choice",
      "help_tip": {
        "en": "What an absent, null or empty value does: pass accepts it with no other check (the default); fail rejects it, which makes the value required.",
        "zh-cn": "缺失、null 或空值的处理：pass 直接接受，不做其他检查（默认）；fail 拒绝，即该值必填。"
      },
      "choices": [
        "pass",
        "fail"
      ],
      "default_value": "pass"
    },
    "encoding": {
      "type": "choice",
      "help_tip": {
        "en": "How the bytes are written: base64, base64url or hex.",
        "zh-cn": "字节的编码方式：base64、base64url 或 hex。"
      },
      "choices": [
        "base64",
        "base64url",
        "hex"
      ],
      "default_value": "base64"
    },
    "ignore_whitespace": {
      "type": "boolean",
      "help_tip": {
        "en": "Ignores spaces and line breaks inside the text.",
        "zh-cn": "忽略文本中的空格和换行。"
      },
      "default_value": false
    },
    "padding": {
      "type": "choice",
      "help_tip": {
        "en": "The = padding of base64: optional, required or forbidden.",
        "zh-cn": "base64 的 = 填充：可选、必需或禁止。"
      },
      "choices": [
        "optional",
        "required",
        "forbidden"
      ],
      "default_value": "optional"
    },
    "hex_case": {
      "type": "choice",
      "help_tip": {
        "en": "Hex digits in any case, lowercase only or uppercase only.",
        "zh-cn": "十六进制数字：任意大小写、仅小写或仅大写。"
      },
      "choices": [
        "any",
        "lower",
        "upper"
      ],
      "default_value": "any"
    },
    "min_bytes": {
      "type": "non_negative_integer",
      "help_tip": {
        "en": "The fewest bytes allowed, once decoded.",
        "zh-cn": "解码后允许的最少字节数。"
      }
    },
    "max_bytes": {
      "type": "non_negative_integer",
      "help_tip": {
        "en": "The most bytes allowed, once decoded.",
        "zh-cn": "解码后允许的最多字节数。"
      }
    }
  },
  "empty_constraint": "empty"
}$wpi$::jsonb),
('bigint', $wpi${
  "descriptor_id": "bigint",
  "schema_version": "0.1",
  "code_module_id": "bigint",
  "category": "primitive",
  "description": {
    "en": "A whole number of any size, as digits, with limits and digit count",
    "zh-cn": "任意大小的整数（数字文本）：范围与位数"
  },
  "implement_notes": {
    "en": "KnowledgeStore/doc-repo/code_modules/validator-primitive.md"
  },
  "value_types": [
    "string",
    "number"
  ],
  "empty_values": [
    "absent",
    "null",
    "empty_string"
  ],
  "execution": "sync",
  "runs_in": "browser_and_backend",
  "error_codes": {
    "bigint.type": {
      "params": []
    },
    "bigint.empty": {
      "params": []
    },
    "bigint.unsafe_number": {
      "params": []
    },
    "bigint.leading_zeros": {
      "params": []
    },
    "bigint.min_digits": {
      "params": [
        "min_digits",
        "actual"
      ]
    },
    "bigint.max_digits": {
      "params": [
        "max_digits",
        "actual"
      ]
    },
    "bigint.minimum": {
      "params": [
        "minimum"
      ]
    },
    "bigint.maximum": {
      "params": [
        "maximum"
      ]
    },
    "bigint.exclusive_minimum": {
      "params": [
        "exclusive_minimum"
      ]
    },
    "bigint.exclusive_maximum": {
      "params": [
        "exclusive_maximum"
      ]
    },
    "bigint.sign": {
      "params": [
        "sign"
      ]
    },
    "bigint.multiple_of": {
      "params": [
        "multiple_of"
      ]
    },
    "bigint.range": {
      "params": [
        "representation",
        "lowest",
        "highest"
      ]
    }
  },
  "constraints": {
    "empty": {
      "type": "choice",
      "help_tip": {
        "en": "What an absent, null or empty value does: pass accepts it with no other check (the default); fail rejects it, which makes the value required.",
        "zh-cn": "缺失、null 或空值的处理：pass 直接接受，不做其他检查（默认）；fail 拒绝，即该值必填。"
      },
      "choices": [
        "pass",
        "fail"
      ],
      "default_value": "pass"
    },
    "accept_numbers": {
      "type": "boolean",
      "help_tip": {
        "en": "Accepts JSON numbers that JSON holds exactly, up to 2^53 − 1.",
        "zh-cn": "接受 JSON 能精确表示的数字，最大 2^53 − 1。"
      },
      "default_value": true
    },
    "allow_leading_zeros": {
      "type": "boolean",
      "help_tip": {
        "en": "Accepts text such as 007.",
        "zh-cn": "接受 007 这样的文本。"
      },
      "default_value": true
    },
    "min_digits": {
      "type": "positive_integer",
      "help_tip": {
        "en": "The fewest digits allowed; the sign and leading zeros do not count.",
        "zh-cn": "允许的最少位数，不计符号和前导零。"
      }
    },
    "max_digits": {
      "type": "positive_integer",
      "help_tip": {
        "en": "The most digits allowed; the sign and leading zeros do not count.",
        "zh-cn": "允许的最多位数，不计符号和前导零。"
      }
    },
    "minimum": {
      "type": "bigint",
      "help_tip": {
        "en": "The smallest value allowed.",
        "zh-cn": "允许的最小值。"
      }
    },
    "maximum": {
      "type": "bigint",
      "help_tip": {
        "en": "The largest value allowed.",
        "zh-cn": "允许的最大值。"
      }
    },
    "exclusive_minimum": {
      "type": "bigint",
      "help_tip": {
        "en": "The value must be greater than this.",
        "zh-cn": "值必须大于此数。"
      }
    },
    "exclusive_maximum": {
      "type": "bigint",
      "help_tip": {
        "en": "The value must be less than this.",
        "zh-cn": "值必须小于此数。"
      }
    },
    "sign": {
      "type": "choice",
      "help_tip": {
        "en": "Whether the value must be positive, negative, non-negative, non-positive or non-zero.",
        "zh-cn": "值是否必须为正、负、非负、非正或非零。"
      },
      "choices": [
        "any",
        "positive",
        "negative",
        "non_negative",
        "non_positive",
        "non_zero"
      ],
      "default_value": "any"
    },
    "multiple_of": {
      "type": "positive_bigint",
      "help_tip": {
        "en": "The value must be an exact multiple of this.",
        "zh-cn": "值必须是此数的整数倍。"
      }
    },
    "representation": {
      "type": "choice",
      "help_tip": {
        "en": "A bounded integer type the value must fit, such as int64 or uint128.",
        "zh-cn": "值必须能放入的有界整数类型，例如 int64 或 uint128。"
      },
      "choices": [
        "unbounded",
        "int64",
        "uint64",
        "int128",
        "uint128"
      ],
      "default_value": "unbounded"
    }
  },
  "empty_constraint": "empty"
}$wpi$::jsonb)
) AS v(id, body)
WHERE NOT EXISTS (SELECT 1 FROM kb.wpi_descriptors d WHERE d.kind = 'validator' AND d.descriptor_id = v.id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM kb.wpi_descriptors WHERE kind = 'validator';
DELETE FROM kb.code_modules WHERE module_type = 'validator';
ALTER TABLE kb.code_modules DROP CONSTRAINT IF EXISTS code_modules_module_type_check;
ALTER TABLE kb.code_modules
    ADD CONSTRAINT code_modules_module_type_check CHECK (module_type IN ('element', 'action', 'page', 'page-layouter', 'element-layouter'));
COMMENT ON COLUMN kb.code_modules.module_type IS
    'What the module implements: element, action, page, page-layouter or element-layouter.';
ALTER TABLE kb.wpi_descriptors DROP CONSTRAINT IF EXISTS wpi_descriptors_kind_check;
ALTER TABLE kb.wpi_descriptors
    ADD CONSTRAINT wpi_descriptors_kind_check CHECK (kind IN ('app', 'element', 'action', 'page-layouter', 'element-layouter'));
-- +goose StatementEnd
