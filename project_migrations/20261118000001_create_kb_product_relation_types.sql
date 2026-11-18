-- kb.product_relation_types is the vocabulary of kb.products.relation_type values.
-- code is the short code the model returns (see prompt-extract-products-merged-v*.md);
-- value is what kb.products.relation_type stores. An unknown code returned by the model is
-- inserted as 'proposed' (value = code) and the product row is accepted; an admin approves
-- it on System Admin -> Database Maintenance -> Product Relation Types, which requires both
-- descriptions. weight feeds product-review scoring. See openspec change
-- product-relation-types.

-- +goose Up
CREATE TABLE IF NOT EXISTS kb.product_relation_types (
    code             TEXT          PRIMARY KEY,
    value            TEXT          NOT NULL,
    status           TEXT          NOT NULL DEFAULT 'proposed',
    description_en   TEXT          NOT NULL DEFAULT '',
    description_zh   TEXT          NOT NULL DEFAULT '',
    weight           NUMERIC(4,3)  NOT NULL DEFAULT 0.5,
    first_record_id  BIGINT,
    first_product_ref TEXT         NOT NULL DEFAULT '',
    seen_count       BIGINT        NOT NULL DEFAULT 0,
    extra_info       JSONB         NOT NULL DEFAULT '{}'::jsonb,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    approved_at      TIMESTAMPTZ,
    approved_by      TEXT,
    CONSTRAINT chk_product_relation_types_status CHECK (status IN ('approved', 'proposed')),
    CONSTRAINT chk_product_relation_types_weight CHECK (weight >= 0 AND weight <= 1),
    CONSTRAINT chk_product_relation_types_described CHECK (
        status <> 'approved' OR (btrim(description_en) <> '' AND btrim(description_zh) <> '')),
    CONSTRAINT uq_product_relation_types_value UNIQUE (value)
);

-- Seed: the 20 codes the prompt defines plus oth. Weights copy
-- [scoring.relation_type_weights] in product-review.local.toml; the five relations it never
-- listed get 0.3, the fallback product review used, so scores do not move.
INSERT INTO kb.product_relation_types (code, value, status, description_en, description_zh, weight, approved_at, approved_by) VALUES
 ('sc','scope','approved','The product is within the scope of the document, clause, rule or requirement.','产品属于文件、条款、规则或要求的适用范围。',1.0,NOW(),'migration'),
 ('reg','regulated_object','approved','The product is subject to legal, regulatory or compliance control.','产品受法律、法规或合规管控。',1.0,NOW(),'migration'),
 ('req','requirement_target','approved','The product must satisfy a requirement.','产品必须满足某项要求。',1.0,NOW(),'migration'),
 ('perf','performance_requirement','approved','The product must meet performance, quality, safety, reliability or functional criteria.','产品必须满足性能、质量、安全、可靠性或功能指标。',0.7,NOW(),'migration'),
 ('des','design_requirement','approved','The product must be designed, configured, structured or constructed in a certain way.','产品必须按特定方式设计、配置、构造或制造。',0.7,NOW(),'migration'),
 ('mat','material_requirement','approved','The product must use, avoid or limit certain materials.','产品必须使用、避免或限制某些材料。',0.7,NOW(),'migration'),
 ('test','testing_requirement','approved','The product must be tested, inspected, calibrated, verified or validated.','产品必须经过检验、检测、校准、验证或确认。',0.6,NOW(),'migration'),
 ('cert','certification_requirement','approved','The product requires certification, approval, registration, labeling or conformity assessment.','产品需要认证、批准、注册、标识或合格评定。',0.6,NOW(),'migration'),
 ('use','usage_condition','approved','The product may or must be used under specified conditions.','产品可以或必须在规定条件下使用。',0.6,NOW(),'migration'),
 ('inst','installation_requirement','approved','The product must be installed, mounted, connected or commissioned in a certain way.','产品必须按特定方式安装、固定、连接或调试。',0.6,NOW(),'migration'),
 ('maint','maintenance_requirement','approved','The product must be maintained, serviced, repaired or periodically checked.','产品必须维护、保养、维修或定期检查。',0.6,NOW(),'migration'),
 ('stor','storage_requirement','approved','The product must be stored, transported, preserved or handled under specified conditions.','产品必须在规定条件下贮存、运输、保存或处置。',0.6,NOW(),'migration'),
 ('ban','prohibited_product','approved','The product is prohibited, restricted, banned or not allowed.','产品被禁止、限制或不允许。',0.3,NOW(),'migration'),
 ('ex','exempted_product','approved','The product is exempted or conditionally excluded.','产品被豁免或有条件排除。',0.3,NOW(),'migration'),
 ('part','component_of','approved','The product is a component, part, accessory or subsystem of another product.','产品是另一产品的组成部分、零件、附件或子系统。',0.5,NOW(),'migration'),
 ('has','contains_product','approved','The product or system contains another product, component, material or subsystem.','产品或系统包含另一产品、部件、材料或子系统。',0.5,NOW(),'migration'),
 ('compat','compatible_with','approved','The product must or may be compatible or interoperable with another product or system.','产品必须或可以与另一产品或系统兼容、互操作。',0.3,NOW(),'migration'),
 ('alt','replacement_or_alternative','approved','The product may replace or substitute another product.','产品可以替代另一产品。',0.3,NOW(),'migration'),
 ('meas','measurement_object','approved','The product is measured, monitored, detected or observed.','产品是被测量、监测、检测或观察的对象。',0.3,NOW(),'migration'),
 ('risk','risk_source','approved','The product creates, contributes to or controls a risk or hazard.','产品产生、助长或控制风险或危害。',0.5,NOW(),'migration'),
 ('oth','other','approved','Use only when no other relation code fits.','仅在没有其他关系代码适用时使用。',0.3,NOW(),'migration')
ON CONFLICT (code) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS kb.product_relation_types;
